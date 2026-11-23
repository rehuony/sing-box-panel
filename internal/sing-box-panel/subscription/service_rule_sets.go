// SPDX-License-Identifier: GPL-3.0-or-later

package subscription

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/rehuony/sing-box-panel/internal/database"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/store"
)

// SubscriptionRuleSet is a rule set as the management API shows it.
type SubscriptionRuleSet struct {
	ID     string                   `json:"id"`
	Client store.SubscriptionFormat `json:"client"`
	Name   string                   `json:"name"`
	Rules  []SubscriptionRule       `json:"rules"`
	// Channels are the channels whose exit rules route by it, in name order.
	Channels  []SubscriptionRuleSetChannel `json:"channels"`
	CreatedAt time.Time                    `json:"created_at"`
	UpdatedAt time.Time                    `json:"updated_at"`
}

type SubscriptionRuleSetChannel struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type SubscriptionRuleSetList struct {
	Items []SubscriptionRuleSet `json:"items"`
}

type CreateSubscriptionRuleSetRequest struct {
	Client store.SubscriptionFormat `json:"client"`
	Name   string                   `json:"name"`
	Rules  []SubscriptionRule       `json:"rules"`
}

// UpdateSubscriptionRuleSetRequest replaces a rule set's name and rules; its
// client never changes.
type UpdateSubscriptionRuleSetRequest struct {
	Name  string             `json:"name"`
	Rules []SubscriptionRule `json:"rules"`
}

func (application *Service) CreateSubscriptionRuleSet(ctx context.Context, request CreateSubscriptionRuleSetRequest) (SubscriptionRuleSet, error) {
	client := RenderFormat(request.Client)
	if client != RenderFormatSingBox && client != RenderFormatMihomo && client != RenderFormatLoon {
		return SubscriptionRuleSet{}, policyError("client", "unsupported_client")
	}
	rules, err := validateRuleSet(client, request.Name, request.Rules)
	if err != nil {
		return SubscriptionRuleSet{}, err
	}
	id, err := database.NewID(application.random, "rule_set")
	if err != nil {
		return SubscriptionRuleSet{}, err
	}
	now := application.now().UTC()
	stored, err := application.database.CreateSubscriptionRuleSet(ctx, store.SubscriptionRuleSet{
		ID: id, Client: request.Client, Name: request.Name, Rules: rules, CreatedAt: now, UpdatedAt: now,
	})
	if err != nil {
		return SubscriptionRuleSet{}, ruleSetStoreError(err)
	}
	return applicationRuleSet(stored)
}

func (application *Service) SubscriptionRuleSet(ctx context.Context, ruleSetID string) (SubscriptionRuleSet, error) {
	stored, err := application.database.GetSubscriptionRuleSet(ctx, strings.TrimSpace(ruleSetID))
	if err != nil {
		return SubscriptionRuleSet{}, err
	}
	return applicationRuleSet(stored)
}

// ListSubscriptionRuleSets returns every rule set, or only client's when it
// is not empty, in name order.
func (application *Service) ListSubscriptionRuleSets(ctx context.Context, client store.SubscriptionFormat) (SubscriptionRuleSetList, error) {
	stored, err := application.database.ListSubscriptionRuleSets(ctx, client)
	if err != nil {
		return SubscriptionRuleSetList{}, err
	}
	list := SubscriptionRuleSetList{Items: make([]SubscriptionRuleSet, 0, len(stored))}
	for _, set := range stored {
		value, err := applicationRuleSet(set)
		if err != nil {
			return SubscriptionRuleSetList{}, err
		}
		list.Items = append(list.Items, value)
	}
	return list, nil
}

// UpdateSubscriptionRuleSet replaces a rule set's name and rules while its
// updated_at is still expected; every channel that routes by it publishes
// the new rules at once.
func (application *Service) UpdateSubscriptionRuleSet(ctx context.Context, ruleSetID string, request UpdateSubscriptionRuleSetRequest, expected time.Time) (SubscriptionRuleSet, error) {
	current, err := application.database.GetSubscriptionRuleSet(ctx, strings.TrimSpace(ruleSetID))
	if err != nil {
		return SubscriptionRuleSet{}, err
	}
	rules, err := validateRuleSet(RenderFormat(current.Client), request.Name, request.Rules)
	if err != nil {
		return SubscriptionRuleSet{}, err
	}
	stored, err := application.database.UpdateSubscriptionRuleSet(ctx, store.UpdateSubscriptionRuleSetInput{
		ID: current.ID, Name: request.Name, Rules: rules,
		ExpectedUpdatedAt: expected, UpdatedAt: application.nextSubscriptionUpdateTime(expected),
	})
	if err != nil {
		return SubscriptionRuleSet{}, ruleSetStoreError(err)
	}
	return applicationRuleSet(stored)
}

// DeleteSubscriptionRuleSet deletes a rule set and the exit rules of every
// channel that routes by it; those channels take a new updated_at.
func (application *Service) DeleteSubscriptionRuleSet(ctx context.Context, ruleSetID string, expected time.Time) error {
	return application.database.DeleteSubscriptionRuleSet(ctx, strings.TrimSpace(ruleSetID), expected, application.now().UTC())
}

// validateRuleSet checks a rule set's name and rules for client and returns
// the rules encoded for storage.
func validateRuleSet(client RenderFormat, name string, rules []SubscriptionRule) (json.RawMessage, error) {
	if !validRuleSetName(name) {
		return nil, policyError("name", "invalid_or_duplicate_name")
	}
	if rules == nil {
		return nil, policyError("rules", "required")
	}
	if err := validateRuleSetRules(rules, client, "rules"); err != nil {
		return nil, err
	}
	encoded, err := json.Marshal(rules)
	if err != nil {
		return nil, err
	}
	if len(encoded) > store.MaximumRuleSetRulesBytes {
		return nil, policyError("rules", "too_large")
	}
	return encoded, nil
}

func validRuleSetName(name string) bool {
	return name != "" && len(name) <= 128 && strings.TrimSpace(name) == name && utf8.ValidString(name) &&
		strings.IndexFunc(name, unicode.IsControl) < 0
}

// ruleSetStoreError reports a name another rule set of the client already
// uses as a policy error on the name.
func ruleSetStoreError(err error) error {
	if errors.Is(err, store.ErrSubscriptionRuleSetExists) {
		return policyError("name", "invalid_or_duplicate_name")
	}
	return err
}

func applicationRuleSet(value store.SubscriptionRuleSet) (SubscriptionRuleSet, error) {
	set := SubscriptionRuleSet{
		ID: value.ID, Client: value.Client, Name: value.Name,
		Channels:  make([]SubscriptionRuleSetChannel, 0, len(value.Channels)),
		CreatedAt: value.CreatedAt, UpdatedAt: value.UpdatedAt,
	}
	if err := json.Unmarshal(value.Rules, &set.Rules); err != nil {
		return SubscriptionRuleSet{}, fmt.Errorf("decode stored rule set rules: %w", err)
	}
	if set.Rules == nil {
		set.Rules = []SubscriptionRule{}
	}
	for _, channel := range value.Channels {
		set.Channels = append(set.Channels, SubscriptionRuleSetChannel{ID: channel.ID, Name: channel.Name})
	}
	return set, nil
}

// channelRuleSets converts stored rule sets for rendering, by ID.
func channelRuleSets(stored []store.SubscriptionRuleSet) (map[string]ChannelRuleSet, error) {
	sets := make(map[string]ChannelRuleSet, len(stored))
	for _, value := range stored {
		set := ChannelRuleSet{Client: RenderFormat(value.Client)}
		if err := json.Unmarshal(value.Rules, &set.Rules); err != nil {
			return nil, fmt.Errorf("decode stored rule set rules: %w", err)
		}
		sets[value.ID] = set
	}
	return sets, nil
}

// readChannelRuleSets reads the rule sets config routes by that exist.
func (application *Service) readChannelRuleSets(ctx context.Context, config ChannelConfig) (map[string]ChannelRuleSet, error) {
	ids := []string{}
	for _, route := range config.Routes {
		if route.RuleSetID != "" {
			ids = append(ids, route.RuleSetID)
		}
	}
	if len(ids) == 0 {
		return map[string]ChannelRuleSet{}, nil
	}
	stored, err := application.database.SubscriptionRuleSetsByID(ctx, ids)
	if err != nil {
		return nil, err
	}
	return channelRuleSets(stored)
}
