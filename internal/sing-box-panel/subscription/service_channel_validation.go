// SPDX-License-Identifier: GPL-3.0-or-later

package subscription

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/schema"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/store"
	jsonschema "github.com/santhosh-tekuri/jsonschema/v6"
)

// Modern sing-box channels use the reviewed native 1.14 schema, independently
// of the server's selected core. The subscriber still owns runtime validation.
const channelSingBoxSchemaVersion = "1.14.0"

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
	if config.Template != nil && format == store.SubscriptionFormatSingBox {
		if err := validateSubscriptionNativeJSON([]byte(config.Template.Content), "template.content"); err != nil {
			return ChannelConfig{}, err
		}
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

func validateSubscriptionNativeJSON(raw []byte, path string) error {
	err := schema.ValidateConfiguration(channelSingBoxSchemaVersion, raw)
	if err == nil {
		return nil
	}
	var issue *jsonschema.ValidationError
	if errors.As(err, &issue) {
		for len(issue.Causes) > 0 {
			issue = issue.Causes[0]
		}
		for _, part := range issue.InstanceLocation {
			// Return field locations, never invalid values or the validator's message,
			// which may include passwords, certificate material or complete documents.
			if len(part) > 64 || strings.ContainsAny(part, "\r\n\x00") {
				part = "*"
			}
			path += "/" + strings.ReplaceAll(strings.ReplaceAll(part, "~", "~0"), "/", "~1")
		}
	}
	return &PolicyError{Path: path, Code: "native_schema_violation"}
}
