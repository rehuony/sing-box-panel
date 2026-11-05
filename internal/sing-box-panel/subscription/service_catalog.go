// SPDX-License-Identifier: GPL-3.0-or-later

package subscription

import (
	"context"
	"encoding/json"

	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/store"
)

type subscriptionCatalogEntry struct {
	Summary      SubscriptionNodeSummary
	Node         Node
	ManualID     string
	CollectionID string
}

func (app *Service) SubscriptionNodeCatalog(ctx context.Context) (SubscriptionNodeCatalog, error) {
	catalog, _, err := app.subscriptionCatalog(ctx)
	return catalog, err
}

func (app *Service) subscriptionCatalog(ctx context.Context) (SubscriptionNodeCatalog, []subscriptionCatalogEntry, error) {
	state, err := app.database.LoadSubscriptionNodeCatalogState(ctx)
	if err != nil {
		return SubscriptionNodeCatalog{}, nil, err
	}
	locals := app.localNodes(state.Nodes, true)
	manual, err := manualPublicationNodes(state.ManualNodes)
	if err != nil {
		return SubscriptionNodeCatalog{}, nil, err
	}
	nodes := append(append([]Node(nil), locals.Nodes...), manual...)
	sourceNames := map[string]string{"local": "手动节点", "manual": "手动节点"}
	enabledSources := map[string]bool{"local": true, "manual": true}
	manualByKey := make(map[string]store.ManualSubscriptionNode)
	for index, node := range manual {
		manualByKey[node.Key] = state.ManualNodes[index]
	}
	for _, source := range state.Sources {
		values, err := DecodeNodes(source.NormalizedNodes)
		if err != nil {
			return SubscriptionNodeCatalog{}, nil, err
		}
		nodes = append(nodes, values...)
		sourceNames[source.SourceID], enabledSources[source.SourceID] = source.Name, source.Enabled
		if source.SourceKind == store.SubscriptionSourceLocal {
			sourceNames[source.SourceID] = "手动节点"
		}
	}
	if len(nodes) > MaximumNodes {
		return SubscriptionNodeCatalog{}, nil, store.ErrSubscriptionLimitExceeded
	}
	nodes = orderSubscriptionNodes(nodes, state.Sources, subscriptionNodeOrders(state.NodeOrders))
	result := SubscriptionNodeCatalog{
		NodePublications: locals.Summaries, Diagnostics: locals.Diagnostics,
		Nodes: make([]SubscriptionNodeSummary, 0, len(nodes)), NodeOrders: subscriptionNodeOrders(state.NodeOrders),
	}
	if result.NodePublications == nil {
		result.NodePublications = []NodePublicationSummary{}
	}
	if result.Diagnostics == nil {
		result.Diagnostics = []ConversionDiagnostic{}
	}
	entries := make([]subscriptionCatalogEntry, 0, len(nodes))
	for _, node := range nodes {
		id := PublicationID(node)
		visibility := state.Visibility[id]
		summary, err := summarizeSubscriptionNode(node)
		if err != nil {
			return SubscriptionNodeCatalog{}, nil, err
		}
		summary.SourceName = sourceNames[node.SourceID]
		summary.Hidden, summary.VisibilityRevision = visibility.Hidden, visibility.Revision
		summary.Available = enabledSources[node.SourceID]
		if node.SourceID == "local" {
			summary.Origin = "local"
			summary.Name = node.OriginTag
			if summary.Name == "" {
				summary.Name = node.Tag
			}
			summary.SourceName, summary.NodeID = node.NodeName, node.NodeID
			summary.Listener = locals.Listeners[node.Key]
			// A node without a public host keeps its inbounds visible as
			// unavailable, without the placeholder host.
			if locals.Hostless[node.NodeID] {
				summary.Server = ""
				summary.Available = false
				decoded, decodeErr := DecodeDocument(node.Outbound)
				if decodeErr != nil {
					return SubscriptionNodeCatalog{}, nil, decodeErr
				}
				outbound := decoded.(map[string]any)
				delete(outbound, "server")
				if tls, ok := outbound["tls"].(map[string]any); ok && tls["server_name"] == catalogHost {
					delete(tls, "server_name")
				}
				node.Outbound, err = json.Marshal(outbound)
				if err != nil {
					return SubscriptionNodeCatalog{}, nil, err
				}
				if summary.SNI == catalogHost {
					summary.SNI = ""
				}
			}
		} else if node.SourceID == "manual" {
			summary.Origin = "manual"
			summary.Revision = manualByKey[node.Key].Revision
		}
		entries = append(entries, subscriptionCatalogEntry{Summary: summary, Node: node, ManualID: manualByKey[node.Key].ID, CollectionID: nodeCollection(node.SourceID, state.Sources)})
		result.Nodes = append(result.Nodes, summary)
	}
	return result, entries, nil
}

func (app *Service) subscriptionNodeEntry(ctx context.Context, id string) (subscriptionCatalogEntry, error) {
	if !validPublicID(id) {
		return subscriptionCatalogEntry{}, store.ErrSubscriptionNodeNotFound
	}
	_, entries, err := app.subscriptionCatalog(ctx)
	if err != nil {
		return subscriptionCatalogEntry{}, err
	}
	for _, entry := range entries {
		if entry.Summary.ID == id {
			return entry, nil
		}
	}
	return subscriptionCatalogEntry{}, store.ErrSubscriptionNodeNotFound
}

func (app *Service) SubscriptionNode(ctx context.Context, id string) (SubscriptionNodeDetail, error) {
	entry, err := app.subscriptionNodeEntry(ctx, id)
	if err != nil {
		return SubscriptionNodeDetail{}, err
	}
	return SubscriptionNodeDetail{SubscriptionNodeSummary: entry.Summary, OutboundJSON: string(entry.Node.Outbound)}, nil
}

func summarizeSubscriptionNode(node Node) (SubscriptionNodeSummary, error) {
	var endpoint struct {
		Server      string          `json:"server"`
		Port        int             `json:"server_port"`
		ServerPorts json.RawMessage `json:"server_ports"`
		Realm       struct {
			ServerURL string `json:"server_url"`
		} `json:"realm"`
		TLS struct {
			Enabled    bool   `json:"enabled"`
			ServerName string `json:"server_name"`
			Reality    struct {
				Enabled bool `json:"enabled"`
			} `json:"reality"`
		} `json:"tls"`
	}
	if err := json.Unmarshal(node.Outbound, &endpoint); err != nil {
		return SubscriptionNodeSummary{}, err
	}
	var ports []string
	if len(endpoint.ServerPorts) > 0 {
		if err := json.Unmarshal(endpoint.ServerPorts, &ports); err != nil {
			var single string
			if err := json.Unmarshal(endpoint.ServerPorts, &single); err != nil {
				return SubscriptionNodeSummary{}, err
			}
			ports = []string{single}
		}
	}
	if node.Type == "ssh" && endpoint.Port == 0 {
		endpoint.Port = 22
	}
	return SubscriptionNodeSummary{
		ServerPorts: ports, RealmURL: endpoint.Realm.ServerURL,
		ID: PublicationID(node), Key: node.Key, SourceID: node.SourceID,
		Type: node.Type, Tag: node.Tag, Name: node.Tag, Credential: node.Credential, Origin: "source",
		Server: endpoint.Server, Port: endpoint.Port, TLS: endpoint.TLS.Enabled, Reality: endpoint.TLS.Reality.Enabled, SNI: endpoint.TLS.ServerName,
	}, nil
}
