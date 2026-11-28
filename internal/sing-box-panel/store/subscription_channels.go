// SPDX-License-Identifier: GPL-3.0-or-later

package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

type SubscriptionFormat string

const (
	SubscriptionFormatSingBox SubscriptionFormat = "sing-box"
	SubscriptionFormatMihomo  SubscriptionFormat = "mihomo"
	SubscriptionFormatLoon    SubscriptionFormat = "loon"
)

// SubscriptionChannel is a channel with its strategy groups and exit rules.
// The subscription service validates the configuration; the store keeps it
// and checks that every pool node and rule set it names exists.
type SubscriptionChannel struct {
	ID        string
	Name      string
	Format    SubscriptionFormat
	Enabled   bool
	Config    SubscriptionChannelConfig
	CreatedAt time.Time
	UpdatedAt time.Time
}

type SubscriptionChannelConfig struct {
	IncompatibleNodes string
	// Template is the channel's base configuration; nil uses the shared default.
	Template *string
	// DefaultExit is the final exit, for traffic no exit rule matches.
	DefaultExit SubscriptionChannelExit
	Groups      []SubscriptionChannelGroup
	Routes      []SubscriptionChannelRoute
}

// SubscriptionChannelExit is where traffic goes: direct, reject, a pool node
// or a group of the channel.
type SubscriptionChannelExit struct {
	Kind string
	ID   string
}

type SubscriptionChannelGroup struct {
	ID          string
	Name        string
	Type        string
	Enabled     bool
	HealthCheck *SubscriptionChannelHealthCheck
	// Match selects the pool nodes the group takes besides its members; nil
	// takes none.
	Match   json.RawMessage
	Members []SubscriptionChannelMember
}

type SubscriptionChannelHealthCheck struct {
	URL       string
	Interval  int
	Tolerance int
}

// SubscriptionChannelMember is a group candidate: a pool node ("node"), a
// built-in exit ("builtin", direct or reject) or another group ("group").
type SubscriptionChannelMember struct {
	Kind string
	ID   string
}

// SubscriptionChannelRoute is one exit rule: a rule set of the channel's
// client, named by RuleSetID, or one rule of the channel's own.
type SubscriptionChannelRoute struct {
	ID        string
	Enabled   bool
	RuleSetID string
	Kind      string
	Value     string
	// Remote is the rule set's metadata, remote rules only.
	Remote json.RawMessage
	Exit   SubscriptionChannelExit
}

type UpdateSubscriptionChannelInput struct {
	Channel           SubscriptionChannel
	ExpectedUpdatedAt time.Time
}

const subscriptionChannelColumns = `
    id, name, format, enabled, incompatible_nodes, template, default_exit_kind,
    coalesce(default_exit_node_id, ''), coalesce(default_exit_group_id, ''), created_at, updated_at`

// CreateSubscriptionChannel stores a new channel at the end of the channels'
// order.
func (s *Store) CreateSubscriptionChannel(ctx context.Context, channel SubscriptionChannel) (SubscriptionChannel, error) {
	prepared, err := prepareSubscriptionChannel(channel)
	if err != nil {
		return SubscriptionChannel{}, err
	}
	prepared.UpdatedAt = prepared.CreatedAt
	var stored SubscriptionChannel
	err = s.WithTx(ctx, func(tx *sql.Tx) error {
		if err := ensureChannelIdentityAvailable(ctx, tx, prepared.ID, prepared.Name, ""); err != nil {
			return err
		}
		if err := ensureSubscriptionChannelLimits(ctx, tx, "", prepared.Enabled); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO subscription_channels(
				id, name, format, enabled, incompatible_nodes, template, created_at, updated_at, position
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, (SELECT coalesce(max(position) + 1, 0) FROM subscription_channels))`,
			prepared.ID, prepared.Name, string(prepared.Format), boolInt(prepared.Enabled), prepared.Config.IncompatibleNodes,
			prepared.Config.Template, formatTime(prepared.CreatedAt), formatTime(prepared.UpdatedAt)); err != nil {
			return fmt.Errorf("insert subscription channel: %w", err)
		}
		if err := saveSubscriptionChannelGroups(ctx, tx, prepared); err != nil {
			return err
		}
		stored, err = getSubscriptionChannel(ctx, tx, prepared.ID)
		return err
	})
	return stored, err
}

func (s *Store) GetSubscriptionChannel(ctx context.Context, channelID string) (SubscriptionChannel, error) {
	if err := validateSubscriptionID(channelID, "channel"); err != nil {
		return SubscriptionChannel{}, fmt.Errorf("%w: %s", ErrSubscriptionChannelNotFound, channelID)
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return SubscriptionChannel{}, err
	}
	defer func() { _ = tx.Rollback() }()
	channel, err := getSubscriptionChannel(ctx, tx, channelID)
	if err != nil {
		return SubscriptionChannel{}, err
	}
	return channel, tx.Commit()
}

// ListSubscriptionChannels returns every channel in its saved order, each
// with its groups, members and exit rules, all from one snapshot.
func (s *Store) ListSubscriptionChannels(ctx context.Context) ([]SubscriptionChannel, error) {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	rows, err := tx.QueryContext(ctx, `SELECT id FROM subscription_channels ORDER BY position, id`)
	if err != nil {
		return nil, fmt.Errorf("list subscription channels: %w", err)
	}
	ids := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("scan subscription channel: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate subscription channels: %w", err)
	}
	channels := make([]SubscriptionChannel, 0, len(ids))
	for _, id := range ids {
		channel, err := getSubscriptionChannel(ctx, tx, id)
		if err != nil {
			return nil, err
		}
		channels = append(channels, channel)
	}
	return channels, tx.Commit()
}

// SetSubscriptionChannelOrder saves the channels' order. ids must name every
// channel exactly once; otherwise they changed since they were read and it
// fails with ErrSubscriptionChannelsChanged.
func (s *Store) SetSubscriptionChannelOrder(ctx context.Context, ids []string) error {
	return s.saveSubscriptionOrder(ctx, "subscription_channels", ids, ErrSubscriptionChannelsChanged, "")
}

// UpdateSubscriptionChannel replaces a channel, its groups, members and exit
// rules together while its updated_at is still ExpectedUpdatedAt.
func (s *Store) UpdateSubscriptionChannel(ctx context.Context, input UpdateSubscriptionChannelInput) (SubscriptionChannel, error) {
	prepared, err := prepareSubscriptionChannel(input.Channel)
	if err != nil {
		return SubscriptionChannel{}, err
	}
	expected, err := requiredUTC(input.ExpectedUpdatedAt, "expected channel updated_at")
	if err != nil {
		return SubscriptionChannel{}, err
	}
	if prepared.UpdatedAt, err = nextSubscriptionUpdateTime(input.Channel.UpdatedAt, expected); err != nil {
		return SubscriptionChannel{}, err
	}
	var stored SubscriptionChannel
	err = s.WithTx(ctx, func(tx *sql.Tx) error {
		current, err := getSubscriptionChannel(ctx, tx, prepared.ID)
		if err != nil {
			return err
		}
		if !current.UpdatedAt.Equal(expected) {
			return subscriptionConflict("channel", prepared.ID, expected, current.UpdatedAt)
		}
		if err := ensureChannelIdentityAvailable(ctx, tx, prepared.ID, prepared.Name, prepared.ID); err != nil {
			return err
		}
		if err := ensureSubscriptionChannelLimits(ctx, tx, prepared.ID, prepared.Enabled); err != nil {
			return err
		}
		result, err := tx.ExecContext(ctx, `UPDATE subscription_channels
			SET name = ?, format = ?, enabled = ?, incompatible_nodes = ?, template = ?,
			    default_exit_kind = 'direct', default_exit_node_id = NULL, default_exit_group_id = NULL, updated_at = ?
			WHERE id = ? AND updated_at = ?`,
			prepared.Name, string(prepared.Format), boolInt(prepared.Enabled), prepared.Config.IncompatibleNodes,
			prepared.Config.Template, formatTime(prepared.UpdatedAt), prepared.ID, formatTime(expected))
		if err != nil {
			return fmt.Errorf("update subscription channel: %w", err)
		}
		if err := requireSingleSubscriptionWrite(result, "update subscription channel"); err != nil {
			return err
		}
		for _, table := range []string{"subscription_channel_routes", "subscription_channel_members", "subscription_channel_groups"} {
			if _, err := tx.ExecContext(ctx, `DELETE FROM `+table+` WHERE channel_id = ?`, prepared.ID); err != nil {
				return fmt.Errorf("replace subscription channel groups: %w", err)
			}
		}
		if err := saveSubscriptionChannelGroups(ctx, tx, prepared); err != nil {
			return err
		}
		stored, err = getSubscriptionChannel(ctx, tx, prepared.ID)
		return err
	})
	return stored, err
}

func (s *Store) DeleteSubscriptionChannel(ctx context.Context, channelID string, expectedUpdatedAt time.Time) error {
	if err := validateSubscriptionID(channelID, "channel"); err != nil {
		return fmt.Errorf("%w: %s", ErrSubscriptionChannelNotFound, channelID)
	}
	expected, err := requiredUTC(expectedUpdatedAt, "expected channel updated_at")
	if err != nil {
		return err
	}
	return s.WithTx(ctx, func(tx *sql.Tx) error {
		var updatedAt string
		err := tx.QueryRowContext(ctx, `SELECT updated_at FROM subscription_channels WHERE id = ?`, channelID).Scan(&updatedAt)
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("%w: %s", ErrSubscriptionChannelNotFound, channelID)
		}
		if err != nil {
			return err
		}
		current, err := parseTime(updatedAt)
		if err != nil {
			return err
		}
		if !current.Equal(expected) {
			return subscriptionConflict("channel", channelID, expected, current)
		}
		result, err := tx.ExecContext(ctx, `DELETE FROM subscription_channels WHERE id = ? AND updated_at = ?`, channelID, formatTime(expected))
		if err != nil {
			return fmt.Errorf("delete subscription channel: %w", err)
		}
		return requireSingleSubscriptionWrite(result, "delete subscription channel")
	})
}

func prepareSubscriptionChannel(channel SubscriptionChannel) (SubscriptionChannel, error) {
	if err := validateSubscriptionID(channel.ID, "channel"); err != nil {
		return SubscriptionChannel{}, err
	}
	if err := validateSubscriptionName(channel.Name); err != nil {
		return SubscriptionChannel{}, err
	}
	if !validSubscriptionFormat(channel.Format) {
		return SubscriptionChannel{}, fmt.Errorf("invalid subscription format %q", channel.Format)
	}
	if channel.Config.IncompatibleNodes == "" {
		channel.Config.IncompatibleNodes = "skip"
	}
	if channel.Config.DefaultExit.Kind == "" {
		channel.Config.DefaultExit.Kind = "direct"
	}
	if channel.CreatedAt.IsZero() {
		channel.CreatedAt = time.Now().UTC()
	}
	channel.CreatedAt = channel.CreatedAt.UTC()
	return channel, nil
}

// saveSubscriptionChannelGroups inserts the channel's groups, their members,
// its exit rules and its final exit. A reference to a node that is not in the
// pool, or to a rule set that does not exist or belongs to another client,
// fails with a SubscriptionReferenceError naming it, as do exit rules that
// together hold more than MaximumChannelRules rules.
func saveSubscriptionChannelGroups(ctx context.Context, tx *sql.Tx, channel SubscriptionChannel) error {
	nodeExists := func(id, path string) error {
		var exists bool
		if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM subscription_nodes WHERE id = ?)`, id).Scan(&exists); err != nil {
			return err
		}
		if !exists {
			return &SubscriptionReferenceError{Path: path, Code: "node_not_in_pool"}
		}
		return nil
	}
	for index, group := range channel.Config.Groups {
		var url, interval, tolerance, match any
		if check := group.HealthCheck; check != nil {
			url, interval, tolerance = check.URL, check.Interval, check.Tolerance
		}
		if len(group.Match) > 0 {
			match = string(group.Match)
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO subscription_channel_groups(
				channel_id, id, position, name, type, enabled, health_check_url, health_check_interval, health_check_tolerance, match_json
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			channel.ID, group.ID, index, group.Name, group.Type, boolInt(group.Enabled), url, interval, tolerance, match); err != nil {
			return fmt.Errorf("insert subscription channel group: %w", err)
		}
	}
	for groupIndex, group := range channel.Config.Groups {
		for position, member := range group.Members {
			var nodeID, groupID, builtin any
			switch member.Kind {
			case "node":
				if err := nodeExists(member.ID, fmt.Sprintf("groups[%d].members[%d]", groupIndex, position)); err != nil {
					return err
				}
				nodeID = member.ID
			case "group":
				groupID = member.ID
			case "builtin":
				builtin = member.ID
			default:
				return fmt.Errorf("invalid subscription channel member kind %q", member.Kind)
			}
			if _, err := tx.ExecContext(ctx, `INSERT INTO subscription_channel_members(
					channel_id, group_id, position, node_id, member_group_id, builtin
				) VALUES (?, ?, ?, ?, ?, ?)`, channel.ID, group.ID, position, nodeID, groupID, builtin); err != nil {
				return fmt.Errorf("insert subscription channel member: %w", err)
			}
		}
	}
	for position, route := range channel.Config.Routes {
		path := fmt.Sprintf("routes[%d]", position)
		exitNode, exitGroup, err := channelExitColumns(route.Exit)
		if err != nil {
			return err
		}
		if exitNode != nil {
			if err := nodeExists(route.Exit.ID, path+".exit"); err != nil {
				return err
			}
		}
		var ruleSet, kind, value, remote any
		if route.RuleSetID != "" {
			var client SubscriptionFormat
			err := tx.QueryRowContext(ctx, `SELECT client FROM subscription_rule_sets WHERE id = ?`, route.RuleSetID).Scan(&client)
			if errors.Is(err, sql.ErrNoRows) {
				return &SubscriptionReferenceError{Path: path + ".rule_set_id", Code: "rule_set_not_found"}
			}
			if err != nil {
				return fmt.Errorf("read subscription rule set client: %w", err)
			}
			if client != channel.Format {
				return &SubscriptionReferenceError{Path: path + ".rule_set_id", Code: "rule_set_client_mismatch"}
			}
			ruleSet = route.RuleSetID
		} else {
			kind = route.Kind
			if route.Kind == "remote" {
				remote = string(route.Remote)
			} else {
				value = route.Value
			}
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO subscription_channel_routes(
				channel_id, id, position, enabled, rule_set_id, kind, value, remote_json, exit_kind, exit_node_id, exit_group_id
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			channel.ID, route.ID, position, boolInt(route.Enabled), ruleSet, kind, value, remote,
			route.Exit.Kind, exitNode, exitGroup); err != nil {
			return fmt.Errorf("insert subscription channel route: %w", err)
		}
	}
	if count, err := channelRuleCount(ctx, tx, channel.ID); err != nil {
		return err
	} else if count > MaximumChannelRules {
		return &SubscriptionReferenceError{Path: "routes", Code: "too_many_rules"}
	}
	exitNode, exitGroup, err := channelExitColumns(channel.Config.DefaultExit)
	if err != nil {
		return err
	}
	if exitNode != nil {
		if err := nodeExists(channel.Config.DefaultExit.ID, "default_exit"); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, `UPDATE subscription_channels
		SET default_exit_kind = ?, default_exit_node_id = ?, default_exit_group_id = ? WHERE id = ?`,
		channel.Config.DefaultExit.Kind, exitNode, exitGroup, channel.ID); err != nil {
		return fmt.Errorf("save subscription channel default exit: %w", err)
	}
	return nil
}

// channelRuleCount is the number of rules a channel's enabled exit rules
// hold: each rule set's rules and each rule of the channel's own.
func channelRuleCount(ctx context.Context, q queryRower, channelID string) (int, error) {
	var count int
	err := q.QueryRowContext(ctx, `SELECT coalesce(sum(CASE WHEN r.rule_set_id IS NULL THEN 1 ELSE json_array_length(s.rules) END), 0)
		FROM subscription_channel_routes AS r LEFT JOIN subscription_rule_sets AS s ON s.id = r.rule_set_id
		WHERE r.channel_id = ? AND r.enabled = 1`, channelID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count subscription channel rules: %w", err)
	}
	return count, nil
}

func channelExitColumns(exit SubscriptionChannelExit) (any, any, error) {
	switch exit.Kind {
	case "direct", "reject":
		return nil, nil, nil
	case "node":
		return exit.ID, nil, nil
	case "group":
		return nil, exit.ID, nil
	}
	return nil, nil, fmt.Errorf("invalid subscription channel exit kind %q", exit.Kind)
}

// getSubscriptionChannel reads a channel with its groups, members and exit
// rules in tx, so they come from one snapshot.
func getSubscriptionChannel(ctx context.Context, tx *sql.Tx, id string) (SubscriptionChannel, error) {
	var channel SubscriptionChannel
	var enabled int
	var template sql.NullString
	var exitNode, exitGroup, createdAt, updatedAt string
	err := tx.QueryRowContext(ctx, `SELECT `+subscriptionChannelColumns+` FROM subscription_channels WHERE id = ?`, id).Scan(
		&channel.ID, &channel.Name, &channel.Format, &enabled, &channel.Config.IncompatibleNodes, &template,
		&channel.Config.DefaultExit.Kind, &exitNode, &exitGroup, &createdAt, &updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return SubscriptionChannel{}, fmt.Errorf("%w: %s", ErrSubscriptionChannelNotFound, id)
	}
	if err != nil {
		return SubscriptionChannel{}, fmt.Errorf("get subscription channel: %w", err)
	}
	switch channel.Config.DefaultExit.Kind {
	case "node":
		channel.Config.DefaultExit.ID = exitNode
	case "group":
		channel.Config.DefaultExit.ID = exitGroup
	}
	channel.Enabled = enabled != 0
	if template.Valid {
		channel.Config.Template = &template.String
	}
	if channel.CreatedAt, err = parseTime(createdAt); err != nil {
		return SubscriptionChannel{}, err
	}
	if channel.UpdatedAt, err = parseTime(updatedAt); err != nil {
		return SubscriptionChannel{}, err
	}
	if channel.Config.Groups, err = loadSubscriptionChannelGroups(ctx, tx, id); err != nil {
		return SubscriptionChannel{}, err
	}
	channel.Config.Routes, err = loadSubscriptionChannelRoutes(ctx, tx, id)
	return channel, err
}

func loadSubscriptionChannelGroups(ctx context.Context, tx *sql.Tx, channelID string) ([]SubscriptionChannelGroup, error) {
	rows, err := tx.QueryContext(ctx, `SELECT id, name, type, enabled, health_check_url, health_check_interval, health_check_tolerance, match_json
		FROM subscription_channel_groups WHERE channel_id = ? ORDER BY position`, channelID)
	if err != nil {
		return nil, fmt.Errorf("read subscription channel groups: %w", err)
	}
	groups := []SubscriptionChannelGroup{}
	indices := map[string]int{}
	for rows.Next() {
		var group SubscriptionChannelGroup
		var enabled int
		var url, match sql.NullString
		var interval, tolerance sql.NullInt64
		if err := rows.Scan(&group.ID, &group.Name, &group.Type, &enabled, &url, &interval, &tolerance, &match); err != nil {
			_ = rows.Close()
			return nil, err
		}
		group.Enabled = enabled != 0
		if url.Valid {
			group.HealthCheck = &SubscriptionChannelHealthCheck{URL: url.String, Interval: int(interval.Int64), Tolerance: int(tolerance.Int64)}
		}
		if match.Valid {
			group.Match = json.RawMessage(match.String)
		}
		group.Members = []SubscriptionChannelMember{}
		indices[group.ID] = len(groups)
		groups = append(groups, group)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	rows, err = tx.QueryContext(ctx, `SELECT group_id, coalesce(node_id, ''), coalesce(member_group_id, ''), coalesce(builtin, '')
		FROM subscription_channel_members WHERE channel_id = ? ORDER BY group_id, position`, channelID)
	if err != nil {
		return nil, fmt.Errorf("read subscription channel members: %w", err)
	}
	for rows.Next() {
		var groupID, nodeID, memberGroupID, builtin string
		if err := rows.Scan(&groupID, &nodeID, &memberGroupID, &builtin); err != nil {
			_ = rows.Close()
			return nil, err
		}
		member := SubscriptionChannelMember{Kind: "node", ID: nodeID}
		if memberGroupID != "" {
			member = SubscriptionChannelMember{Kind: "group", ID: memberGroupID}
		} else if builtin != "" {
			member = SubscriptionChannelMember{Kind: "builtin", ID: builtin}
		}
		group := &groups[indices[groupID]]
		group.Members = append(group.Members, member)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return groups, nil
}

func loadSubscriptionChannelRoutes(ctx context.Context, tx *sql.Tx, channelID string) ([]SubscriptionChannelRoute, error) {
	rows, err := tx.QueryContext(ctx, `SELECT id, enabled, coalesce(rule_set_id, ''), coalesce(kind, ''), coalesce(value, ''), remote_json,
		exit_kind, coalesce(exit_node_id, ''), coalesce(exit_group_id, '')
		FROM subscription_channel_routes WHERE channel_id = ? ORDER BY position`, channelID)
	if err != nil {
		return nil, fmt.Errorf("read subscription channel routes: %w", err)
	}
	defer rows.Close()
	routes := []SubscriptionChannelRoute{}
	for rows.Next() {
		var exitNode, exitGroup string
		var route SubscriptionChannelRoute
		var enabled int
		var remote sql.NullString
		if err := rows.Scan(&route.ID, &enabled, &route.RuleSetID, &route.Kind, &route.Value, &remote,
			&route.Exit.Kind, &exitNode, &exitGroup); err != nil {
			return nil, err
		}
		route.Enabled = enabled != 0
		if remote.Valid {
			route.Remote = json.RawMessage(remote.String)
		}
		switch route.Exit.Kind {
		case "node":
			route.Exit.ID = exitNode
		case "group":
			route.Exit.ID = exitGroup
		}
		routes = append(routes, route)
	}
	return routes, rows.Err()
}
