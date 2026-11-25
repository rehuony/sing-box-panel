// SPDX-License-Identifier: GPL-3.0-or-later

package subscription

import (
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
