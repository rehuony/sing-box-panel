// SPDX-License-Identifier: GPL-3.0-or-later

package subscription

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/store"
)

// validateSubscriptionChannelConfig decodes and checks a channel
// configuration for format.
func validateSubscriptionChannelConfig(format store.SubscriptionFormat, raw json.RawMessage) (ChannelConfig, error) {
	config, err := DecodeChannelConfig(raw)
	if err != nil {
		return ChannelConfig{}, err
	}
	if err := ValidateChannelConfig(config, RenderFormat(format)); err != nil {
		return ChannelConfig{}, err
	}
	return config, nil
}

// renderableChannelConfig decodes and checks a channel configuration for
// format, then renders it from the current pool and the rule sets it routes
// by, as a preview of it would, so a write never stores a channel its
// subscribers could not read; a render it refuses keeps the preview's
// problem. The render lands in the render cache, ready for the first
// subscriber.
func (application *Service) renderableChannelConfig(ctx context.Context, format store.SubscriptionFormat, raw json.RawMessage) (ChannelConfig, error) {
	config, err := validateSubscriptionChannelConfig(format, raw)
	if err != nil {
		return ChannelConfig{}, err
	}
	pool, err := application.database.SubscriptionPool(ctx)
	if err != nil {
		return ChannelConfig{}, err
	}
	available := make(map[string]bool, len(pool))
	for _, node := range pool {
		available[node.ID] = true
	}
	if err := validateChannelPoolReferences(config, available); err != nil {
		return ChannelConfig{}, err
	}
	ruleSets, err := application.readChannelRuleSets(ctx, config)
	if err != nil {
		return ChannelConfig{}, err
	}
	if _, _, err := application.renderSubscriptionState(store.PublicSubscriptionState{Pool: pool}, format, config, ruleSets); err != nil {
		return ChannelConfig{}, err
	}
	return config, nil
}

// validateChannelPoolReferences checks that every pool node config names is
// among available.
func validateChannelPoolReferences(config ChannelConfig, available map[string]bool) error {
	for groupIndex, group := range config.Groups {
		for index, member := range group.Members {
			if member.Kind == memberNode && !available[member.ID] {
				return policyError(fmt.Sprintf("groups[%d].members[%d]", groupIndex, index), "node_not_in_pool")
			}
		}
	}
	for index, route := range config.Routes {
		if route.Exit.Kind == "node" && !available[route.Exit.ID] {
			return policyError(fmt.Sprintf("routes[%d].exit", index), "node_not_in_pool")
		}
	}
	if config.DefaultExit.Kind == "node" && !available[config.DefaultExit.ID] {
		return policyError("default_exit", "node_not_in_pool")
	}
	return nil
}
