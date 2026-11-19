// SPDX-License-Identifier: GPL-3.0-or-later

package subscription

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/rehuony/sing-box-panel/internal/jsonstrict"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/store"
)

const maximumChannelConfigBytes = 640 << 10

// ChannelConfig is a channel's strategy groups, default exit and template.
// The channel publishes only the pool nodes its enabled groups name as
// members, rule exits or the default exit.
type ChannelConfig struct {
	IncompatibleNodes string          `json:"incompatible_nodes"`
	Groups            []RuleGroup     `json:"groups"`
	DefaultExit       RouteExit       `json:"default_exit"`
	Template          *NativeTemplate `json:"template,omitempty"`
}

type RouteExit struct {
	Kind string `json:"kind"`
	ID   string `json:"id,omitempty"`
}

// RuleGroup is a strategy group. Members are its candidates in order; the
// first is the initial manual selection.
type RuleGroup struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Enabled     bool              `json:"enabled"`
	Type        string            `json:"type"`
	HealthCheck *GroupHealthCheck `json:"health_check,omitempty"`
	Members     []GroupMember     `json:"members"`
	Rules       []ChannelRule     `json:"rules"`
}

// GroupMember is a candidate: a pool node, a built-in direct or reject exit,
// or another group of the channel.
type GroupMember struct {
	Kind string `json:"kind"`
	ID   string `json:"id"`
}

const (
	memberNode    = "node"
	memberBuiltin = "builtin"
	memberGroup   = "group"
)

type GroupHealthCheck struct {
	URL       string `json:"url"`
	Interval  int    `json:"interval"`
	Tolerance int    `json:"tolerance"`
}

func (g RuleGroup) healthCheck() GroupHealthCheck {
	if g.HealthCheck != nil {
		return *g.HealthCheck
	}
	return GroupHealthCheck{URL: "https://www.gstatic.com/generate_204", Interval: 300, Tolerance: 50}
}

// Rules and remote references share one ordered list. A disabled entry retains
// its metadata, but is not emitted. Nothing here fetches remote rule contents.
type ChannelRule struct {
	SortIndex int64          `json:"sort_index"`
	ID        string         `json:"id"`
	Enabled   bool           `json:"enabled"`
	Kind      string         `json:"kind"`
	Value     string         `json:"value,omitempty"`
	Remote    *RemoteRuleSet `json:"remote,omitempty"`
	Exit      RouteExit      `json:"exit"`
}

type RemoteRuleSet struct {
	Name        string `json:"name"`
	URL         string `json:"url"`
	Format      string `json:"format"`
	Behavior    string `json:"behavior,omitempty"`
	Accelerated bool   `json:"accelerated"`
}

type NativeTemplate struct {
	Format  RenderFormat `json:"format"`
	Content string       `json:"content"`
}

// DecodeChannelConfig strictly decodes a channel configuration: unknown and
// null fields are rejected, and every list is present.
func DecodeChannelConfig(raw json.RawMessage) (ChannelConfig, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return ChannelConfig{}, policyError("config", "object_required")
	}
	if len(raw) > maximumChannelConfigBytes {
		return ChannelConfig{}, policyError("config", "too_large")
	}
	if bytes.Contains(raw, []byte("null")) {
		var fields any
		if err := json.Unmarshal(raw, &fields); err == nil && containsNull(fields) {
			return ChannelConfig{}, policyError("config", "null_value")
		}
	}
	var config ChannelConfig
	if err := jsonstrict.Decode(raw, maximumChannelConfigBytes, &config); err != nil {
		return ChannelConfig{}, fmt.Errorf("subscription channel config: %w", err)
	}
	if config.Groups == nil {
		return ChannelConfig{}, policyError("groups", "required")
	}
	if config.DefaultExit.Kind == "" {
		return ChannelConfig{}, policyError("default_exit", "required")
	}
	if config.IncompatibleNodes == "" {
		config.IncompatibleNodes = "skip"
	}
	for index := range config.Groups {
		group := &config.Groups[index]
		if group.Members == nil {
			return ChannelConfig{}, policyError(fmt.Sprintf("groups[%d].members", index), "required")
		}
		if group.Rules == nil {
			return ChannelConfig{}, policyError(fmt.Sprintf("groups[%d].rules", index), "required")
		}
	}
	return config, nil
}

func containsNull(value any) bool {
	switch value := value.(type) {
	case nil:
		return true
	case map[string]any:
		for _, child := range value {
			if containsNull(child) {
				return true
			}
		}
	case []any:
		for _, child := range value {
			if containsNull(child) {
				return true
			}
		}
	}
	return false
}

// storeChannelConfig converts config for storage.
func storeChannelConfig(config ChannelConfig) (store.SubscriptionChannelConfig, error) {
	stored := store.SubscriptionChannelConfig{
		IncompatibleNodes: config.IncompatibleNodes,
		DefaultExit:       store.SubscriptionChannelExit{Kind: config.DefaultExit.Kind, ID: config.DefaultExit.ID},
		Groups:            make([]store.SubscriptionChannelGroup, 0, len(config.Groups)),
	}
	if config.Template != nil {
		content := config.Template.Content
		stored.Template = &content
	}
	for _, group := range config.Groups {
		value := store.SubscriptionChannelGroup{
			ID: group.ID, Name: group.Name, Type: group.Type, Enabled: group.Enabled,
			Members: make([]store.SubscriptionChannelMember, 0, len(group.Members)),
			Rules:   make([]store.SubscriptionChannelRule, 0, len(group.Rules)),
		}
		if check := group.HealthCheck; check != nil {
			value.HealthCheck = &store.SubscriptionChannelHealthCheck{URL: check.URL, Interval: check.Interval, Tolerance: check.Tolerance}
		}
		for _, member := range group.Members {
			value.Members = append(value.Members, store.SubscriptionChannelMember{Kind: member.Kind, ID: member.ID})
		}
		for _, rule := range group.Rules {
			stored := store.SubscriptionChannelRule{
				ID: rule.ID, SortIndex: rule.SortIndex, Enabled: rule.Enabled, Kind: rule.Kind, Value: rule.Value,
				Exit: store.SubscriptionChannelExit{Kind: rule.Exit.Kind, ID: rule.Exit.ID},
			}
			if rule.Remote != nil {
				encoded, err := json.Marshal(rule.Remote)
				if err != nil {
					return store.SubscriptionChannelConfig{}, err
				}
				stored.Remote = encoded
			}
			value.Rules = append(value.Rules, stored)
		}
		stored.Groups = append(stored.Groups, value)
	}
	return stored, nil
}

// applicationChannelConfig converts a stored configuration for a channel of
// format.
func applicationChannelConfig(stored store.SubscriptionChannelConfig, format store.SubscriptionFormat) (ChannelConfig, error) {
	config := ChannelConfig{
		IncompatibleNodes: stored.IncompatibleNodes,
		DefaultExit:       RouteExit{Kind: stored.DefaultExit.Kind, ID: stored.DefaultExit.ID},
		Groups:            make([]RuleGroup, 0, len(stored.Groups)),
	}
	if stored.Template != nil {
		config.Template = &NativeTemplate{Format: RenderFormat(format), Content: *stored.Template}
	}
	for _, group := range stored.Groups {
		value := RuleGroup{
			ID: group.ID, Name: group.Name, Type: group.Type, Enabled: group.Enabled,
			Members: make([]GroupMember, 0, len(group.Members)), Rules: make([]ChannelRule, 0, len(group.Rules)),
		}
		if check := group.HealthCheck; check != nil {
			value.HealthCheck = &GroupHealthCheck{URL: check.URL, Interval: check.Interval, Tolerance: check.Tolerance}
		}
		for _, member := range group.Members {
			value.Members = append(value.Members, GroupMember{Kind: member.Kind, ID: member.ID})
		}
		for _, rule := range group.Rules {
			converted := ChannelRule{
				SortIndex: rule.SortIndex, ID: rule.ID, Enabled: rule.Enabled, Kind: rule.Kind, Value: rule.Value,
				Exit: RouteExit{Kind: rule.Exit.Kind, ID: rule.Exit.ID},
			}
			if len(rule.Remote) > 0 {
				converted.Remote = &RemoteRuleSet{}
				if err := json.Unmarshal(rule.Remote, converted.Remote); err != nil {
					return ChannelConfig{}, fmt.Errorf("decode stored remote rule set: %w", err)
				}
			}
			value.Rules = append(value.Rules, converted)
		}
		config.Groups = append(config.Groups, value)
	}
	return config, nil
}
