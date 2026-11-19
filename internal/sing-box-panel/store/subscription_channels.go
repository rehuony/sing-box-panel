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

// SubscriptionChannel is a channel with its strategy groups. The
// subscription service validates the configuration; the store keeps it and
// checks that every pool node it names exists.
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
	Template    *string
	DefaultExit SubscriptionChannelExit
	Groups      []SubscriptionChannelGroup
}

// SubscriptionChannelExit is where traffic goes: direct, reject, a pool node,
// a group of the channel or, for a rule, its group's default.
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
	Members     []SubscriptionChannelMember
	Rules       []SubscriptionChannelRule
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

type SubscriptionChannelRule struct {
	ID        string
	SortIndex int64
	Enabled   bool
	Kind      string
	Value     string
	// Remote is the rule set's metadata, remote rules only.
	Remote json.RawMessage
	Exit   SubscriptionChannelExit
}

type SubscriptionChannelSummary struct {
	ID        string
	Name      string
	Format    SubscriptionFormat
	Enabled   bool
	CreatedAt time.Time
	UpdatedAt time.Time
}

type SubscriptionChannelListFilter struct {
	Cursor *CreatedAtCursor
	Limit  int
}

type SubscriptionChannelPage struct {
	Items []SubscriptionChannelSummary
	Next  *CreatedAtCursor
}

type UpdateSubscriptionChannelInput struct {
	Channel           SubscriptionChannel
	ExpectedUpdatedAt time.Time
}

const subscriptionChannelColumns = `
    id, name, format, enabled, incompatible_nodes, template, default_exit_kind,
    coalesce(default_exit_node_id, ''), coalesce(default_exit_group_id, ''), created_at, updated_at`

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
				id, name, format, enabled, incompatible_nodes, template, created_at, updated_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
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

func (s *Store) ListSubscriptionChannels(ctx context.Context, filter SubscriptionChannelListFilter) (SubscriptionChannelPage, error) {
	limit, err := normalizePageLimit(filter.Limit)
	if err != nil {
		return SubscriptionChannelPage{}, err
	}
	if err := validateCreatedAtCursor(filter.Cursor); err != nil {
		return SubscriptionChannelPage{}, err
	}
	query := `SELECT id, name, format, enabled, created_at, updated_at FROM subscription_channels`
	args := make([]any, 0, 4)
	if filter.Cursor != nil {
		query += ` WHERE (created_at < ? OR (created_at = ? AND id < ?))`
		cursorTime := formatTime(filter.Cursor.CreatedAt)
		args = append(args, cursorTime, cursorTime, filter.Cursor.ID)
	}
	query += ` ORDER BY created_at DESC, id DESC LIMIT ?`
	args = append(args, limit+1)
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return SubscriptionChannelPage{}, fmt.Errorf("list subscription channels: %w", err)
	}
	defer rows.Close()
	items := make([]SubscriptionChannelSummary, 0, limit+1)
	for rows.Next() {
		var channel SubscriptionChannelSummary
		var enabled int
		var createdAt, updatedAt string
		if err := rows.Scan(&channel.ID, &channel.Name, &channel.Format, &enabled, &createdAt, &updatedAt); err != nil {
			return SubscriptionChannelPage{}, fmt.Errorf("scan subscription channel: %w", err)
		}
		channel.Enabled = enabled != 0
		if channel.CreatedAt, err = parseTime(createdAt); err != nil {
			return SubscriptionChannelPage{}, err
		}
		if channel.UpdatedAt, err = parseTime(updatedAt); err != nil {
			return SubscriptionChannelPage{}, err
		}
		items = append(items, channel)
	}
	if err := rows.Err(); err != nil {
		return SubscriptionChannelPage{}, fmt.Errorf("iterate subscription channels: %w", err)
	}
	page := SubscriptionChannelPage{Items: items}
	if len(items) > limit {
		page.Items = items[:limit]
		last := page.Items[len(page.Items)-1]
		page.Next = &CreatedAtCursor{CreatedAt: last.CreatedAt, ID: last.ID}
	}
	return page, nil
}

// UpdateSubscriptionChannel replaces a channel, its groups, members and rules
// together while its updated_at is still ExpectedUpdatedAt.
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
		for _, table := range []string{"subscription_channel_rules", "subscription_channel_members", "subscription_channel_groups"} {
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

// saveSubscriptionChannelGroups inserts the channel's groups, their members
// and rules, then its default exit. A reference to a node that is not in the
// pool fails with a SubscriptionReferenceError naming it.
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
		var url, interval, tolerance any
		if check := group.HealthCheck; check != nil {
			url, interval, tolerance = check.URL, check.Interval, check.Tolerance
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO subscription_channel_groups(
				channel_id, id, position, name, type, enabled, health_check_url, health_check_interval, health_check_tolerance
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			channel.ID, group.ID, index, group.Name, group.Type, boolInt(group.Enabled), url, interval, tolerance); err != nil {
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
		for position, rule := range group.Rules {
			exitNode, exitGroup, err := channelExitColumns(rule.Exit)
			if err != nil {
				return err
			}
			if exitNode != nil {
				if err := nodeExists(rule.Exit.ID, fmt.Sprintf("groups[%d].rules[%d].exit", groupIndex, position)); err != nil {
					return err
				}
			}
			var value, remote any
			if rule.Kind == "remote" {
				remote = string(rule.Remote)
			} else {
				value = rule.Value
			}
			if _, err := tx.ExecContext(ctx, `INSERT INTO subscription_channel_rules(
					channel_id, id, group_id, position, sort_index, enabled, kind, value, remote_json, exit_kind, exit_node_id, exit_group_id
				) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
				channel.ID, rule.ID, group.ID, position, rule.SortIndex, boolInt(rule.Enabled), rule.Kind, value, remote,
				rule.Exit.Kind, exitNode, exitGroup); err != nil {
				return fmt.Errorf("insert subscription channel rule: %w", err)
			}
		}
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

func channelExitColumns(exit SubscriptionChannelExit) (any, any, error) {
	switch exit.Kind {
	case "direct", "reject", "group-default":
		return nil, nil, nil
	case "node":
		return exit.ID, nil, nil
	case "group":
		return nil, exit.ID, nil
	}
	return nil, nil, fmt.Errorf("invalid subscription channel exit kind %q", exit.Kind)
}

// getSubscriptionChannel reads a channel with its groups, members and rules
// in tx, so they come from one snapshot.
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
	channel.Config.Groups, err = loadSubscriptionChannelGroups(ctx, tx, id)
	return channel, err
}

func loadSubscriptionChannelGroups(ctx context.Context, tx *sql.Tx, channelID string) ([]SubscriptionChannelGroup, error) {
	rows, err := tx.QueryContext(ctx, `SELECT id, name, type, enabled, health_check_url, health_check_interval, health_check_tolerance
		FROM subscription_channel_groups WHERE channel_id = ? ORDER BY position`, channelID)
	if err != nil {
		return nil, fmt.Errorf("read subscription channel groups: %w", err)
	}
	groups := []SubscriptionChannelGroup{}
	indices := map[string]int{}
	for rows.Next() {
		var group SubscriptionChannelGroup
		var enabled int
		var url sql.NullString
		var interval, tolerance sql.NullInt64
		if err := rows.Scan(&group.ID, &group.Name, &group.Type, &enabled, &url, &interval, &tolerance); err != nil {
			_ = rows.Close()
			return nil, err
		}
		group.Enabled = enabled != 0
		if url.Valid {
			group.HealthCheck = &SubscriptionChannelHealthCheck{URL: url.String, Interval: int(interval.Int64), Tolerance: int(tolerance.Int64)}
		}
		group.Members, group.Rules = []SubscriptionChannelMember{}, []SubscriptionChannelRule{}
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
	rows, err = tx.QueryContext(ctx, `SELECT group_id, id, sort_index, enabled, kind, coalesce(value, ''), remote_json,
		exit_kind, coalesce(exit_node_id, ''), coalesce(exit_group_id, '')
		FROM subscription_channel_rules WHERE channel_id = ? ORDER BY group_id, position`, channelID)
	if err != nil {
		return nil, fmt.Errorf("read subscription channel rules: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var groupID, exitNode, exitGroup string
		var rule SubscriptionChannelRule
		var enabled int
		var remote sql.NullString
		if err := rows.Scan(&groupID, &rule.ID, &rule.SortIndex, &enabled, &rule.Kind, &rule.Value, &remote,
			&rule.Exit.Kind, &exitNode, &exitGroup); err != nil {
			return nil, err
		}
		rule.Enabled = enabled != 0
		if remote.Valid {
			rule.Remote = json.RawMessage(remote.String)
		}
		switch rule.Exit.Kind {
		case "node":
			rule.Exit.ID = exitNode
		case "group":
			rule.Exit.ID = exitGroup
		}
		group := &groups[indices[groupID]]
		group.Rules = append(group.Rules, rule)
	}
	return groups, rows.Err()
}
