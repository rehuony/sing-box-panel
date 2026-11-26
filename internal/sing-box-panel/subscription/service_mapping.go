// SPDX-License-Identifier: GPL-3.0-or-later

package subscription

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/store"
)

func (application *Service) newSubscriptionTokenSecret() (string, string, error) {
	raw := make([]byte, subscriptionTokenEntropyBytes)
	n, err := application.random(raw)
	if err != nil {
		return "", "", fmt.Errorf("generate subscription token: %w", err)
	}
	if n != len(raw) {
		return "", "", errors.New("generate subscription token: short random read")
	}
	plaintext := base64.RawURLEncoding.EncodeToString(raw)
	digest := sha256.Sum256([]byte(plaintext))
	return plaintext, hex.EncodeToString(digest[:]), nil
}

func (application *Service) nextSubscriptionUpdateTime(expected time.Time) time.Time {
	now := application.now().UTC()
	if !now.After(expected.UTC()) {
		return expected.UTC().Add(time.Nanosecond)
	}
	return now
}

func applicationSubscriptionChannel(value store.SubscriptionChannel) (SubscriptionChannel, error) {
	config, err := applicationChannelConfig(value.Config, value.Format)
	if err != nil {
		return SubscriptionChannel{}, err
	}
	return SubscriptionChannel{
		ID: value.ID, Name: value.Name, Format: value.Format, Config: config, Enabled: value.Enabled,
		CreatedAt: value.CreatedAt, UpdatedAt: value.UpdatedAt,
	}, nil
}

// applicationSubscriptionChannelSummary describes a channel by the nodes of
// pool it publishes, as rendering picks them, its exit rules and its final exit.
func applicationSubscriptionChannelSummary(value store.SubscriptionChannel, pool []ChannelNode) (SubscriptionChannelSummary, error) {
	config, err := applicationChannelConfig(value.Config, value.Format)
	if err != nil {
		return SubscriptionChannelSummary{}, err
	}
	published := publishedNodeIDs(expandGroupMatches(config, pool))
	final := ChannelFinalExit{Kind: config.DefaultExit.Kind}
	nodes := 0
	for _, node := range pool {
		if published[node.ID] && publishableOutbound(node.Protocol) {
			nodes++
		}
		if final.Kind == "node" && node.ID == config.DefaultExit.ID {
			final.Name = node.Name
		}
	}
	for _, group := range config.Groups {
		if final.Kind == "group" && group.ID == config.DefaultExit.ID {
			final.Name = group.Name
		}
	}
	return SubscriptionChannelSummary{
		ID: value.ID, Name: value.Name, Format: value.Format, Enabled: value.Enabled,
		NodeCount: nodes, RouteCount: len(config.Routes), FinalExit: final,
		CreatedAt: value.CreatedAt, UpdatedAt: value.UpdatedAt,
	}, nil
}

func applicationSubscriptionToken(value store.SubscriptionToken, at time.Time) SubscriptionToken {
	return SubscriptionToken{
		DownloadLimit: value.DownloadLimit,
		ID:            value.ID, UserID: value.UserID, Label: value.Label, Enabled: value.Enabled,
		ExpiresAt: store.CloneTime(value.ExpiresAt), RevokedAt: store.CloneTime(value.RevokedAt),
		SuccessfulRequestCount: value.SuccessfulRequestCount, BodyResponseCount: value.BodyResponseCount,
		BytesServed: value.BytesServed, LastUsedAt: store.CloneTime(value.LastUsedAt),
		CreatedAt: value.CreatedAt, Active: value.Active(at),
	}
}

func applicationSubscriptionUser(value store.SubscriptionUser) SubscriptionUser {
	return SubscriptionUser{
		ID: value.ID, Name: value.Name, Description: value.Description, Enabled: value.Enabled,
		CreatedAt: value.CreatedAt, UpdatedAt: value.UpdatedAt,
	}
}

func storeSubscriptionCursor(value *SubscriptionCursor) *store.CreatedAtCursor {
	if value == nil {
		return nil
	}
	return &store.CreatedAtCursor{CreatedAt: value.CreatedAt, ID: strings.TrimSpace(value.ID)}
}

func applicationSubscriptionCursor(value *store.CreatedAtCursor) *SubscriptionCursor {
	if value == nil {
		return nil
	}
	return &SubscriptionCursor{CreatedAt: value.CreatedAt, ID: value.ID}
}
