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

// ChannelConfig is a channel's strategy groups, exit rules, final exit and
// template. The channel publishes only the pool nodes its enabled groups name
// as members or match, its enabled exit rules' exits or its final exit.
// Traffic follows the first enabled exit rule that matches, in order, and the
// final exit when none does.
type ChannelConfig struct {
	IncompatibleNodes string         `json:"incompatible_nodes"`
	Groups            []RuleGroup    `json:"groups"`
	Routes            []ChannelRoute `json:"routes"`
	// DefaultExit is the final exit.
	DefaultExit RouteExit       `json:"default_exit"`
	Template    *NativeTemplate `json:"template,omitempty"`
}

type RouteExit struct {
	Kind string `json:"kind"`
	ID   string `json:"id,omitempty"`
}

// RuleGroup is a strategy group. Members are its candidates in order; the
// first is the initial manual selection. Match takes further pool nodes
// after them.
type RuleGroup struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Enabled     bool              `json:"enabled"`
	Type        string            `json:"type"`
	Match       *GroupMatch       `json:"match,omitempty"`
	HealthCheck *GroupHealthCheck `json:"health_check,omitempty"`
	Members     []GroupMember     `json:"members"`
}

// GroupMatch selects pool nodes a group takes besides its members: nodes of
// one of Origins, when any are named, of one of Protocols, when any are
// named, whose display name Include and Exclude keep as a source filter
// does. A match that names nothing takes no nodes.
type GroupMatch struct {
	Include []string `json:"include"`
	Exclude []string `json:"exclude"`
	// Origins are "fleet", for the fleet's inbounds, or "source:<id>".
	Origins   []string `json:"origins"`
	Protocols []string `json:"protocols"`
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

// ChannelRoute is one exit rule: a rule set of the channel's client, whose
// rules all take Exit in their order, or one rule of the channel's own.
// Exactly one of RuleSetID and Rule is present. A disabled exit rule keeps
// its settings but is not emitted.
type ChannelRoute struct {
	ID        string            `json:"id"`
	Enabled   bool              `json:"enabled"`
	RuleSetID string            `json:"rule_set_id,omitempty"`
	Rule      *SubscriptionRule `json:"rule,omitempty"`
	Exit      RouteExit         `json:"exit"`
}

// SubscriptionRule matches a domain, part of one or an IP range, or names a
// remote rule set the client fetches. Nothing here fetches remote rules.
type SubscriptionRule struct {
	Kind   string         `json:"kind"`
	Value  string         `json:"value,omitempty"`
	Remote *RemoteRuleSet `json:"remote,omitempty"`
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
	if config.Routes == nil {
		return ChannelConfig{}, policyError("routes", "required")
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
		if match := group.Match; match != nil {
			for _, list := range []struct {
				name   string
				values []string
			}{{"include", match.Include}, {"exclude", match.Exclude}, {"origins", match.Origins}, {"protocols", match.Protocols}} {
				if list.values == nil {
					return ChannelConfig{}, policyError(fmt.Sprintf("groups[%d].match.%s", index, list.name), "required")
				}
			}
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
		Routes:            make([]store.SubscriptionChannelRoute, 0, len(config.Routes)),
	}
	if config.Template != nil {
		content := config.Template.Content
		stored.Template = &content
	}
	for _, group := range config.Groups {
		value := store.SubscriptionChannelGroup{
			ID: group.ID, Name: group.Name, Type: group.Type, Enabled: group.Enabled,
			Members: make([]store.SubscriptionChannelMember, 0, len(group.Members)),
		}
		if check := group.HealthCheck; check != nil {
			value.HealthCheck = &store.SubscriptionChannelHealthCheck{URL: check.URL, Interval: check.Interval, Tolerance: check.Tolerance}
		}
		if group.Match != nil {
			encoded, err := json.Marshal(group.Match)
			if err != nil {
				return store.SubscriptionChannelConfig{}, err
			}
			value.Match = encoded
		}
		for _, member := range group.Members {
			value.Members = append(value.Members, store.SubscriptionChannelMember{Kind: member.Kind, ID: member.ID})
		}
		stored.Groups = append(stored.Groups, value)
	}
	for _, route := range config.Routes {
		value := store.SubscriptionChannelRoute{
			ID: route.ID, Enabled: route.Enabled, RuleSetID: route.RuleSetID,
			Exit: store.SubscriptionChannelExit{Kind: route.Exit.Kind, ID: route.Exit.ID},
		}
		if rule := route.Rule; rule != nil {
			value.Kind, value.Value = rule.Kind, rule.Value
			if rule.Remote != nil {
				encoded, err := json.Marshal(rule.Remote)
				if err != nil {
					return store.SubscriptionChannelConfig{}, err
				}
				value.Remote = encoded
			}
		}
		stored.Routes = append(stored.Routes, value)
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
		Routes:            make([]ChannelRoute, 0, len(stored.Routes)),
	}
	if stored.Template != nil {
		config.Template = &NativeTemplate{Format: RenderFormat(format), Content: *stored.Template}
	}
	for _, group := range stored.Groups {
		value := RuleGroup{
			ID: group.ID, Name: group.Name, Type: group.Type, Enabled: group.Enabled,
			Members: make([]GroupMember, 0, len(group.Members)),
		}
		if check := group.HealthCheck; check != nil {
			value.HealthCheck = &GroupHealthCheck{URL: check.URL, Interval: check.Interval, Tolerance: check.Tolerance}
		}
		if len(group.Match) > 0 {
			value.Match = &GroupMatch{}
			if err := json.Unmarshal(group.Match, value.Match); err != nil {
				return ChannelConfig{}, fmt.Errorf("decode stored group match: %w", err)
			}
		}
		for _, member := range group.Members {
			value.Members = append(value.Members, GroupMember{Kind: member.Kind, ID: member.ID})
		}
		config.Groups = append(config.Groups, value)
	}
	for _, route := range stored.Routes {
		value := ChannelRoute{
			ID: route.ID, Enabled: route.Enabled, RuleSetID: route.RuleSetID,
			Exit: RouteExit{Kind: route.Exit.Kind, ID: route.Exit.ID},
		}
		if route.RuleSetID == "" {
			value.Rule = &SubscriptionRule{Kind: route.Kind, Value: route.Value}
			if len(route.Remote) > 0 {
				value.Rule.Remote = &RemoteRuleSet{}
				if err := json.Unmarshal(route.Remote, value.Rule.Remote); err != nil {
					return ChannelConfig{}, fmt.Errorf("decode stored remote rule set: %w", err)
				}
			}
		}
		config.Routes = append(config.Routes, value)
	}
	return config, nil
}
