// SPDX-License-Identifier: GPL-3.0-or-later

package subscription

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"math"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/store"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/telemetry"
)

var (
	ErrPublicSubscriptionAccessDenied       = errors.New("public subscription access denied")
	ErrPublicSubscriptionChannelUnavailable = errors.New("public subscription channel is not in the applied bundle")
)

// PublicSubscriptionResult owns caller-safe response bytes rendered from one
// consistent read of the pool and live authorization. Diagnostics contain stable positions and codes only; neither the
// plaintext token nor source/configuration secrets are retained in this value.
type PublicSubscriptionResult struct {
	TokenID     string                    `json:"-"`
	Format      store.SubscriptionFormat  `json:"format"`
	MediaType   string                    `json:"media_type"`
	Body        []byte                    `json:"-"`
	ETag        string                    `json:"etag"`
	NodeCount   int                       `json:"node_count"`
	Diagnostics []RenderDiagnostic        `json:"diagnostics"`
	Traffic     PublicSubscriptionTraffic `json:"-"`
}

// PublicSubscriptionTraffic is what the Subscription-Userinfo header
// reports: the recorded usage of the nodes whose inbounds the subscription
// carries, including incomplete or stale totals, their quotas and the key's
// expiry.
type PublicSubscriptionTraffic struct {
	UploadBytes   int64
	DownloadBytes int64
	// TotalBytes is the sum of the nodes' quotas, 0 when any of them is
	// unlimited or the subscription carries no node.
	TotalBytes int64
	// ExpiresAt is when the key expires, nil when it never does.
	ExpiresAt *time.Time
}

// PublicSubscription authenticates the current token/user state and renders
// the current channel from the pool, only a user's granted nodes for a
// user-scoped key. The store supplies the key, channel and pool in one
// consistent read.
func (application *Service) PublicSubscription(
	ctx context.Context,
	plaintextToken string,
	channelID string,
) (PublicSubscriptionResult, error) {
	if plaintextToken == "" || len(plaintextToken) > 512 || !validPublicID(channelID) {
		return PublicSubscriptionResult{}, ErrPublicSubscriptionAccessDenied
	}
	digest := sha256.Sum256([]byte(plaintextToken))
	now := application.now().UTC()
	state, err := application.database.LoadPublicSubscriptionState(
		ctx,
		hex.EncodeToString(digest[:]),
		channelID,
		now,
	)
	if err != nil {
		switch {
		case errors.Is(err, store.ErrSubscriptionTokenNotFound),
			errors.Is(err, store.ErrSubscriptionTokenInactive):
			return PublicSubscriptionResult{}, ErrPublicSubscriptionAccessDenied
		case errors.Is(err, store.ErrSubscriptionChannelNotFound):
			return PublicSubscriptionResult{}, ErrPublicSubscriptionChannelUnavailable
		default:
			return PublicSubscriptionResult{}, err
		}
	}

	config, err := applicationChannelConfig(state.Channel.Config, state.Channel.Format)
	if err != nil {
		return PublicSubscriptionResult{}, err
	}
	ruleSets, err := channelRuleSets(state.RuleSets)
	if err != nil {
		return PublicSubscriptionResult{}, err
	}
	result, nodeIDs, err := application.renderSubscriptionState(state, state.Channel.Format, config, ruleSets)
	if err != nil {
		return PublicSubscriptionResult{}, err
	}
	traffic, err := application.publicSubscriptionTraffic(ctx, now, nodeIDs)
	if err != nil {
		return PublicSubscriptionResult{}, err
	}
	traffic.ExpiresAt = state.TokenExpiresAt
	bodyDigest := sha256.Sum256(result.Content)
	return PublicSubscriptionResult{
		TokenID: state.TokenID, Format: store.SubscriptionFormat(result.Format), MediaType: result.MediaType,
		Body: bytes.Clone(result.Content), ETag: hex.EncodeToString(bodyDigest[:]),
		NodeCount: result.NodeCount, Diagnostics: result.Diagnostics, Traffic: traffic,
	}, nil
}

// channelNodes is the pool as channels render it, in pool order.
func channelNodes(pool []store.SubscriptionPoolNode) []ChannelNode {
	nodes := make([]ChannelNode, len(pool))
	for index, node := range pool {
		nodes[index] = ChannelNode{
			ID: node.ID, Name: node.DisplayName, Protocol: node.Protocol,
			SourceID: node.SourceID, OriginName: node.OriginName, Outbound: node.Outbound,
		}
	}
	return nodes
}

// publicSubscriptionTraffic is the usage the subscription header reports:
// the traffic of the nodes whose inbounds the subscription carries, each in
// its own current period, against the sum of their quotas.
func (application *Service) publicSubscriptionTraffic(ctx context.Context, at time.Time, nodeIDs []string) (PublicSubscriptionTraffic, error) {
	status, err := application.telemetry.TrafficStatus(ctx, at, nodeIDs)
	if err != nil {
		return PublicSubscriptionTraffic{}, err
	}
	return PublicSubscriptionTraffic{
		UploadBytes: status.UploadBytes, DownloadBytes: status.DownloadBytes,
		TotalBytes: quotaSum(status.Nodes),
	}, nil
}

// quotaSum is the sum of the nodes' quotas, capped at the largest int64, so
// what a client has left is what the nodes have left together. It is 0, which
// clients read as unlimited, when any node is unlimited or there is none.
func quotaSum(nodes []telemetry.NodeTrafficUsage) int64 {
	var total int64
	for _, node := range nodes {
		if node.QuotaBytes == nil {
			return 0
		}
		if *node.QuotaBytes > math.MaxInt64-total {
			total = math.MaxInt64
			continue
		}
		total += *node.QuotaBytes
	}
	return total
}

// renderSubscriptionState renders the channel of state, or config in its
// place, from the pool nodes state allows and the rule sets config routes
// by, and returns the fleet nodes whose inbounds the output delivers.
// renderSubscriptionState renders a channel and lists the fleet nodes it
// carries, from the render cache when nothing it reads changed.
func (application *Service) renderSubscriptionState(state store.PublicSubscriptionState, format store.SubscriptionFormat, config ChannelConfig, ruleSets map[string]ChannelRuleSet) (RenderResult, []string, error) {
	key, err := renderDigest(state.Pool, format, config, ruleSets)
	if err != nil {
		return RenderResult{}, nil, err
	}
	if cached, found := application.renders.get(key); found {
		return cached.result, cached.nodeIDs, nil
	}
	rendered, err := RenderChannelNodes(channelNodes(state.Pool), RenderFormat(format), config, ruleSets)
	if err != nil {
		return RenderResult{}, nil, err
	}
	fleetNodes := []string{}
	seen := map[string]bool{}
	for _, node := range state.Pool {
		if node.NodeID != "" && !seen[node.NodeID] && rendered.Carries(node.ID) {
			seen[node.NodeID] = true
			fleetNodes = append(fleetNodes, node.NodeID)
		}
	}
	application.renders.put(key, cachedRender{result: rendered, nodeIDs: fleetNodes})
	return rendered, fleetNodes, nil
}

func (application *Service) RecordPublicSubscriptionUse(
	ctx context.Context,
	tokenID string,
	bodyBytes int64,
) error {
	err := application.database.RecordSubscriptionTokenUse(ctx, tokenID, application.now().UTC(), bodyBytes)
	if errors.Is(err, store.ErrSubscriptionTokenInactive) {
		return ErrPublicSubscriptionAccessDenied
	}
	return err
}

func validPublicID(value string) bool {
	if value == "" || len(value) > 128 || !utf8.ValidString(value) {
		return false
	}
	for _, character := range value {
		if unicode.IsControl(character) || unicode.IsSpace(character) {
			return false
		}
	}
	return true
}
