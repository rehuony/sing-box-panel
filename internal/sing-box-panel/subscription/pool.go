// SPDX-License-Identifier: GPL-3.0-or-later

package subscription

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/store"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/telemetry"
)

// ErrInvalidNodeDisplayName rejects a display name that is empty, longer
// than 256 bytes, padded with spaces or holds a control character or comma.
var ErrInvalidNodeDisplayName = errors.New("invalid subscription node display name")

// Pool node origins.
const (
	originInbound = "inbound"
	originSource  = "source"
)

// poolNodeIDLength is the number of hex digits of a pool node ID's digest.
const poolNodeIDLength = 32

// poolNodeID derives a pool node's ID from its origin: the origin kind, the
// fleet node or source it comes from, and its key there.
func poolNodeID(kind, originID, originKey string) string {
	sum := sha256.Sum256([]byte(kind + "\x00" + originID + "\x00" + originKey))
	return "sn_" + hex.EncodeToString(sum[:])[:poolNodeIDLength]
}

// inboundOriginKey keys a fleet inbound by its tag and the identity of its
// credential.
func inboundOriginKey(node Node) string {
	return node.OriginTag + "#" + strings.TrimPrefix(node.Key, "local:")
}

// inboundTag returns the inbound tag of an inbound's origin key.
func inboundTag(originKey string) string {
	if index := strings.LastIndexByte(originKey, '#'); index >= 0 {
		return originKey[:index]
	}
	return originKey
}

// SyncPool makes the pool what its inputs yield: every fleet node's
// published inbounds and every node a source's filter keeps. Nodes that are
// gone are deleted with their channel memberships and grants, the others
// keep their display names and positions, and new nodes are appended in
// origin order. Synchronizations run one at a time.
func (application *Service) SyncPool(ctx context.Context) error {
	application.poolMu.Lock()
	defer application.poolMu.Unlock()
	for attempt := 0; ; attempt++ {
		inputs, err := application.database.LoadSubscriptionPoolInputs(ctx)
		if err != nil {
			return err
		}
		plan, err := application.planPool(inputs)
		if err != nil {
			return err
		}
		err = application.database.SynchronizeSubscriptionPool(ctx, plan)
		if !errors.Is(err, store.ErrSubscriptionPoolInputsChanged) || attempt == 2 {
			return err
		}
	}
}

// PoolInputsChanged synchronizes the pool after one of its inputs changed,
// such as a source's content or a fleet node's applied configuration. The
// change itself is already saved, so a failure is recorded instead of
// returned, and the next synchronization repairs the pool.
func (application *Service) PoolInputsChanged(ctx context.Context) {
	if err := application.SyncPool(ctx); err != nil && ctx.Err() == nil {
		application.telemetry.RecordOperation(ctx, "subscription.pool.sync", "Subscription node pool synchronization", err, telemetry.OperationLogContext{})
	}
}

func (application *Service) planPool(inputs store.SubscriptionPoolInputs) (store.SubscriptionPoolSynchronization, error) {
	plan := store.SubscriptionPoolSynchronization{
		Fingerprint: inputs.Fingerprint, KeptSources: map[string]bool{}, SourceErrors: map[string]string{},
		At: application.now().UTC(),
	}
	kept := 0
	locals := application.localNodes(inputs.Publications)
	for _, inbound := range locals.Inbounds {
		key := inboundOriginKey(inbound.Node)
		outbound, err := outboundWithoutTag(inbound.Node.Outbound)
		if err != nil {
			return store.SubscriptionPoolSynchronization{}, err
		}
		plan.Nodes = append(plan.Nodes, store.SubscriptionPoolNodeInput{
			ID: poolNodeID(originInbound, inbound.NodeID, key), NodeID: inbound.NodeID, OriginKey: key,
			OriginName: inbound.Node.OriginName, DisplayName: inbound.NodeName + " · " + inbound.Node.OriginName,
			Protocol: inbound.Node.Type, Outbound: outbound,
		})
	}
	for _, source := range inputs.Sources {
		reading, err := ReadSource(source.Content, SourceFormat(source.ContentFormat), sourceFilter(source.Filter))
		if err != nil {
			plan.KeptSources[source.ID] = true
			plan.SourceErrors[source.ID] = sourceErrorCode(err)
			kept += source.NodeCount
			continue
		}
		for _, item := range reading.Kept() {
			outbound, err := json.Marshal(item.Outbound)
			if err != nil {
				return store.SubscriptionPoolSynchronization{}, err
			}
			plan.Nodes = append(plan.Nodes, store.SubscriptionPoolNodeInput{
				ID: poolNodeID(originSource, source.ID, item.OriginKey), SourceID: source.ID, OriginKey: item.OriginKey,
				OriginName: item.Name, DisplayName: item.Name, Protocol: item.Protocol, Outbound: outbound,
			})
		}
	}
	if len(plan.Nodes)+kept > MaximumNodes {
		return store.SubscriptionPoolSynchronization{}, fmt.Errorf("%w: the pool would hold more than %d nodes", store.ErrSubscriptionLimitExceeded, MaximumNodes)
	}
	return plan, nil
}

func outboundWithoutTag(raw json.RawMessage) (json.RawMessage, error) {
	outbound, err := DecodeDocumentObject(raw)
	if err != nil {
		return nil, err
	}
	delete(outbound, "tag")
	return json.Marshal(outbound)
}

func sourceFilter(filter store.SubscriptionSourceFilter) SourceFilter {
	return SourceFilter{Include: append([]string{}, filter.Include...), Exclude: append([]string{}, filter.Exclude...)}
}

// sourceErrorCode is the code a source records for err.
func sourceErrorCode(err error) string {
	var parseErr *SourceParseError
	if errors.As(err, &parseErr) {
		return parseErr.Code
	}
	if code := FetchErrorCode(err); code != "" {
		return code
	}
	if errors.Is(err, store.ErrSubscriptionLimitExceeded) {
		return SourceErrorTooManyNodes
	}
	return "source_failed"
}

// SubscriptionNodeOrigin is where a pool node comes from: a fleet node's
// inbound or a source.
type SubscriptionNodeOrigin struct {
	Kind       string `json:"kind"`
	NodeID     string `json:"node_id,omitempty"`
	NodeName   string `json:"node_name,omitempty"`
	InboundTag string `json:"inbound_tag,omitempty"`
	SourceID   string `json:"source_id,omitempty"`
	SourceName string `json:"source_name,omitempty"`
}

// SubscriptionNode is a pool node as the management API shows it.
type SubscriptionNode struct {
	ID          string                 `json:"id"`
	DisplayName string                 `json:"display_name"`
	OriginName  string                 `json:"origin_name"`
	Protocol    string                 `json:"protocol"`
	Origin      SubscriptionNodeOrigin `json:"origin"`
	Server      string                 `json:"server,omitempty"`
	Port        int                    `json:"port,omitempty"`
	ServerPorts []string               `json:"server_ports,omitempty"`
	RealmURL    string                 `json:"realm_url,omitempty"`
	TLS         bool                   `json:"tls"`
	Reality     bool                   `json:"reality"`
	SNI         string                 `json:"sni,omitempty"`
	Transport   string                 `json:"transport,omitempty"`
	UpdatedAt   time.Time              `json:"updated_at"`
}

// SubscriptionNodeDetail adds the node's outbound, credentials included.
type SubscriptionNodeDetail struct {
	SubscriptionNode
	Outbound json.RawMessage `json:"outbound"`
}

// SubscriptionNodeList is the pool in its saved order with what the fleet
// publishes from.
type SubscriptionNodeList struct {
	Items            []SubscriptionNode       `json:"items"`
	NodePublications []NodePublicationSummary `json:"node_publications"`
	Diagnostics      []ConversionDiagnostic   `json:"diagnostics"`
}

func subscriptionNodeView(node store.SubscriptionPoolNode) (SubscriptionNode, error) {
	var endpoint struct {
		Server      string          `json:"server"`
		Port        int             `json:"server_port"`
		ServerPorts json.RawMessage `json:"server_ports"`
		Peers       []struct {
			Address string `json:"address"`
			Port    int    `json:"port"`
		} `json:"peers"`
		Realm struct {
			ServerURL string `json:"server_url"`
		} `json:"realm"`
		TLS struct {
			Enabled    bool   `json:"enabled"`
			ServerName string `json:"server_name"`
			Reality    struct {
				Enabled bool `json:"enabled"`
			} `json:"reality"`
		} `json:"tls"`
		Transport struct {
			Type string `json:"type"`
		} `json:"transport"`
	}
	if err := json.Unmarshal(node.Outbound, &endpoint); err != nil {
		return SubscriptionNode{}, fmt.Errorf("decode subscription node %s: %w", node.ID, err)
	}
	var ports []string
	if len(endpoint.ServerPorts) > 0 {
		if err := json.Unmarshal(endpoint.ServerPorts, &ports); err != nil {
			var single string
			if err := json.Unmarshal(endpoint.ServerPorts, &single); err != nil {
				return SubscriptionNode{}, err
			}
			ports = []string{single}
		}
	}
	if endpoint.Server == "" && len(endpoint.Peers) > 0 {
		endpoint.Server, endpoint.Port = endpoint.Peers[0].Address, endpoint.Peers[0].Port
	}
	if node.Protocol == "ssh" && endpoint.Port == 0 {
		endpoint.Port = 22
	}
	view := SubscriptionNode{
		ID: node.ID, DisplayName: node.DisplayName, OriginName: node.OriginName, Protocol: node.Protocol,
		Server: endpoint.Server, Port: endpoint.Port, ServerPorts: ports, RealmURL: endpoint.Realm.ServerURL,
		TLS: endpoint.TLS.Enabled, Reality: endpoint.TLS.Reality.Enabled, SNI: endpoint.TLS.ServerName,
		Transport: endpoint.Transport.Type, UpdatedAt: node.UpdatedAt,
	}
	if node.NodeID != "" {
		view.Origin = SubscriptionNodeOrigin{Kind: originInbound, NodeID: node.NodeID, NodeName: node.NodeName, InboundTag: inboundTag(node.OriginKey)}
	} else {
		view.Origin = SubscriptionNodeOrigin{Kind: originSource, SourceID: node.SourceID, SourceName: node.SourceName}
	}
	return view, nil
}

// SubscriptionNodes returns the pool in its saved order, with the nodes the
// fleet publishes from and the diagnostics of what it cannot publish.
func (application *Service) SubscriptionNodes(ctx context.Context) (SubscriptionNodeList, error) {
	pool, err := application.database.SubscriptionPool(ctx)
	if err != nil {
		return SubscriptionNodeList{}, err
	}
	publications, err := application.database.LoadNodePublications(ctx)
	if err != nil {
		return SubscriptionNodeList{}, err
	}
	locals := application.localNodes(publications)
	list := SubscriptionNodeList{Items: make([]SubscriptionNode, 0, len(pool)), NodePublications: locals.Summaries, Diagnostics: locals.Diagnostics}
	for _, node := range pool {
		view, err := subscriptionNodeView(node)
		if err != nil {
			return SubscriptionNodeList{}, err
		}
		list.Items = append(list.Items, view)
	}
	return list, nil
}

func (application *Service) SubscriptionNode(ctx context.Context, id string) (SubscriptionNodeDetail, error) {
	if !validPublicID(id) {
		return SubscriptionNodeDetail{}, store.ErrSubscriptionNodeNotFound
	}
	node, err := application.database.SubscriptionPoolNode(ctx, id)
	if err != nil {
		return SubscriptionNodeDetail{}, err
	}
	return subscriptionNodeDetail(node)
}

func subscriptionNodeDetail(node store.SubscriptionPoolNode) (SubscriptionNodeDetail, error) {
	view, err := subscriptionNodeView(node)
	if err != nil {
		return SubscriptionNodeDetail{}, err
	}
	return SubscriptionNodeDetail{SubscriptionNode: view, Outbound: append(json.RawMessage(nil), node.Outbound...)}, nil
}

// UpdateSubscriptionNode changes a node's display name while its updated_at
// is still expected. The name is unique in the pool, ignoring case.
func (application *Service) UpdateSubscriptionNode(ctx context.Context, id, displayName string, expected time.Time) (SubscriptionNodeDetail, error) {
	if !validPublicID(id) {
		return SubscriptionNodeDetail{}, store.ErrSubscriptionNodeNotFound
	}
	// Channels cannot publish a name with a comma.
	if err := store.ValidateNodeDisplayName(displayName); err != nil || strings.Contains(displayName, ",") {
		return SubscriptionNodeDetail{}, ErrInvalidNodeDisplayName
	}
	node, err := application.database.RenameSubscriptionPoolNode(ctx, id, displayName, expected, application.nextSubscriptionUpdateTime(expected))
	if err != nil {
		return SubscriptionNodeDetail{}, err
	}
	return subscriptionNodeDetail(node)
}

// SetSubscriptionNodeOrder saves the pool's order. ids must be exactly the
// pool's current IDs; otherwise the pool changed since it was read.
func (application *Service) SetSubscriptionNodeOrder(ctx context.Context, ids []string) (SubscriptionNodeList, error) {
	if len(ids) > MaximumNodes {
		return SubscriptionNodeList{}, store.ErrSubscriptionPoolChanged
	}
	if err := application.database.SetSubscriptionPoolOrder(ctx, ids); err != nil {
		return SubscriptionNodeList{}, err
	}
	return application.SubscriptionNodes(ctx)
}
