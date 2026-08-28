// SPDX-License-Identifier: GPL-3.0-or-later

package store

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/rehuony/sing-box-panel/internal/jsonstrict"
)

type SubscriptionFormat string

const (
	SubscriptionFormatSingBox SubscriptionFormat = "sing-box"
	SubscriptionFormatMihomo  SubscriptionFormat = "mihomo"
	SubscriptionFormatLoon    SubscriptionFormat = "loon"
)

// SubscriptionChannelConfig is the strict, renderer-owned channel policy.
type SubscriptionChannelConfig struct {
	ExcludeTags  []string `json:"exclude_tags,omitempty"`
	ExcludeTypes []string `json:"exclude_types,omitempty"`
}

type SubscriptionChannel struct {
	ID         string
	Name       string
	Format     SubscriptionFormat
	PublicHost string
	Config     json.RawMessage
	Enabled    bool
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

type SubscriptionChannelSummary struct {
	ID         string
	Name       string
	Format     SubscriptionFormat
	PublicHost string
	Enabled    bool
	CreatedAt  time.Time
	UpdatedAt  time.Time
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
	ID                string
	Name              string
	Format            SubscriptionFormat
	PublicHost        string
	Config            json.RawMessage
	Enabled           bool
	ExpectedUpdatedAt time.Time
	UpdatedAt         time.Time
}

func (s *Store) CreateSubscriptionChannel(
	ctx context.Context,
	channel SubscriptionChannel,
) (SubscriptionChannel, error) {
	prepared, err := prepareNewSubscriptionChannel(channel)
	if err != nil {
		return SubscriptionChannel{}, err
	}
	var stored SubscriptionChannel
	err = s.WithTx(ctx, func(tx *sql.Tx) error {
		if err := ensureChannelIdentityAvailable(ctx, tx, prepared.ID, prepared.Name, ""); err != nil {
			return err
		}
		if err := ensureSubscriptionChannelLimits(ctx, tx, "", prepared); err != nil {
			return err
		}
		if _, err := tx.ExecContext(
			ctx,
			`INSERT INTO subscription_channels(
	                id, name, format, public_host, config_json, enabled, created_at, updated_at
	             ) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			prepared.ID,
			prepared.Name,
			string(prepared.Format),
			prepared.PublicHost,
			string(prepared.Config),
			boolInt(prepared.Enabled),
			formatTaskTime(prepared.CreatedAt),
			formatTaskTime(prepared.UpdatedAt),
		); err != nil {
			return fmt.Errorf("insert subscription channel: %w", err)
		}
		stored, err = getSubscriptionChannel(ctx, tx, prepared.ID)
		return err
	})
	return stored, err
}

func (s *Store) GetSubscriptionChannel(
	ctx context.Context,
	channelID string,
) (SubscriptionChannel, error) {
	if err := validateSubscriptionID(channelID, "channel"); err != nil {
		return SubscriptionChannel{}, err
	}
	return getSubscriptionChannel(ctx, s.db, channelID)
}

func (s *Store) ListSubscriptionChannels(
	ctx context.Context,
	filter SubscriptionChannelListFilter,
) (SubscriptionChannelPage, error) {
	limit, err := normalizePageLimit(filter.Limit)
	if err != nil {
		return SubscriptionChannelPage{}, err
	}
	if err := validateCreatedAtCursor(filter.Cursor); err != nil {
		return SubscriptionChannelPage{}, err
	}
	query := `SELECT id, name, format, public_host, enabled, created_at, updated_at
	           FROM subscription_channels`
	args := make([]any, 0, 4)
	if filter.Cursor != nil {
		query += ` WHERE (created_at < ? OR (created_at = ? AND id < ?))`
		cursorTime := formatTaskTime(filter.Cursor.CreatedAt)
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
		channel, err := scanSubscriptionChannelSummary(rows)
		if err != nil {
			return SubscriptionChannelPage{}, fmt.Errorf("scan subscription channel: %w", err)
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

func (s *Store) UpdateSubscriptionChannel(
	ctx context.Context,
	input UpdateSubscriptionChannelInput,
) (SubscriptionChannel, error) {
	prepared, err := prepareSubscriptionChannelUpdate(input)
	if err != nil {
		return SubscriptionChannel{}, err
	}
	var stored SubscriptionChannel
	err = s.WithTx(ctx, func(tx *sql.Tx) error {
		current, err := getSubscriptionChannel(ctx, tx, prepared.ID)
		if err != nil {
			return err
		}
		if !current.UpdatedAt.Equal(prepared.ExpectedUpdatedAt) {
			return subscriptionConflict("channel", prepared.ID, prepared.ExpectedUpdatedAt, current.UpdatedAt)
		}
		if err := ensureChannelIdentityAvailable(ctx, tx, prepared.ID, prepared.Name, prepared.ID); err != nil {
			return err
		}
		if err := ensureSubscriptionChannelLimits(ctx, tx, prepared.ID, SubscriptionChannel{
			ID: prepared.ID, Name: prepared.Name, Format: prepared.Format, PublicHost: prepared.PublicHost,
			Config: prepared.Config, Enabled: prepared.Enabled,
		}); err != nil {
			return err
		}
		result, err := tx.ExecContext(
			ctx,
			`UPDATE subscription_channels
	                SET name = ?, format = ?, public_host = ?, config_json = ?, enabled = ?, updated_at = ?
	              WHERE id = ? AND updated_at = ?`,
			prepared.Name,
			string(prepared.Format),
			prepared.PublicHost,
			string(prepared.Config),
			boolInt(prepared.Enabled),
			formatTaskTime(prepared.UpdatedAt),
			prepared.ID,
			formatTaskTime(prepared.ExpectedUpdatedAt),
		)
		if err != nil {
			return fmt.Errorf("update subscription channel: %w", err)
		}
		if err := requireSingleSubscriptionWrite(result, "update subscription channel"); err != nil {
			return err
		}
		stored, err = getSubscriptionChannel(ctx, tx, prepared.ID)
		return err
	})
	return stored, err
}

func (s *Store) DeleteSubscriptionChannel(
	ctx context.Context,
	channelID string,
	expectedUpdatedAt time.Time,
) error {
	if err := validateSubscriptionID(channelID, "channel"); err != nil {
		return err
	}
	expectedUpdatedAt, err := requiredUTC(expectedUpdatedAt, "expected channel updated_at")
	if err != nil {
		return err
	}
	return s.WithTx(ctx, func(tx *sql.Tx) error {
		current, err := getSubscriptionChannel(ctx, tx, channelID)
		if err != nil {
			return err
		}
		if !current.UpdatedAt.Equal(expectedUpdatedAt) {
			return subscriptionConflict("channel", channelID, expectedUpdatedAt, current.UpdatedAt)
		}
		result, err := tx.ExecContext(
			ctx,
			`DELETE FROM subscription_channels WHERE id = ? AND updated_at = ?`,
			channelID,
			formatTaskTime(expectedUpdatedAt),
		)
		if err != nil {
			return fmt.Errorf("delete subscription channel: %w", err)
		}
		return requireSingleSubscriptionWrite(result, "delete subscription channel")
	})
}

func DecodeSubscriptionChannelConfig(raw json.RawMessage) (SubscriptionChannelConfig, error) {
	if len(raw) == 0 {
		raw = json.RawMessage(`{}`)
	}
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return SubscriptionChannelConfig{}, errors.New("subscription channel config must be a non-null JSON object")
	}
	var config SubscriptionChannelConfig
	if err := jsonstrict.Decode(raw, maximumSubscriptionConfigBytes, &config); err != nil {
		return SubscriptionChannelConfig{}, fmt.Errorf("subscription channel config: %w", err)
	}
	var fields map[string]json.RawMessage
	if err := jsonstrict.Decode(raw, maximumSubscriptionConfigBytes, &fields); err != nil || fields == nil {
		return SubscriptionChannelConfig{}, errors.New("subscription channel config must be a non-null JSON object")
	}
	for name, value := range fields {
		if bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return SubscriptionChannelConfig{}, fmt.Errorf("subscription channel config field %q cannot be null", name)
		}
	}
	if config.ExcludeTags == nil {
		config.ExcludeTags = []string{}
	}
	if config.ExcludeTypes == nil {
		config.ExcludeTypes = []string{}
	}
	if len(config.ExcludeTags) > maximumChannelExclusions || len(config.ExcludeTypes) > maximumChannelExclusions {
		return SubscriptionChannelConfig{}, errors.New("subscription channel config has too many exclusions")
	}
	if err := validateUniqueSubscriptionStrings(config.ExcludeTags, validSubscriptionTag, "tag"); err != nil {
		return SubscriptionChannelConfig{}, err
	}
	if err := validateUniqueSubscriptionStrings(config.ExcludeTypes, validSubscriptionType, "type"); err != nil {
		return SubscriptionChannelConfig{}, err
	}
	return config, nil
}

func prepareNewSubscriptionChannel(channel SubscriptionChannel) (SubscriptionChannel, error) {
	if err := validateSubscriptionID(channel.ID, "channel"); err != nil {
		return SubscriptionChannel{}, err
	}
	if err := validateSubscriptionName(channel.Name); err != nil {
		return SubscriptionChannel{}, err
	}
	if !validSubscriptionFormat(channel.Format) {
		return SubscriptionChannel{}, fmt.Errorf("invalid subscription format %q", channel.Format)
	}
	publicHost, err := normalizeSubscriptionPublicHost(channel.PublicHost)
	if err != nil {
		return SubscriptionChannel{}, err
	}
	config, err := canonicalChannelConfig(channel.Config)
	if err != nil {
		return SubscriptionChannel{}, err
	}
	createdAt := channel.CreatedAt
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	} else {
		createdAt = createdAt.UTC()
	}
	channel.PublicHost = publicHost
	channel.Config = config
	channel.CreatedAt = createdAt
	channel.UpdatedAt = createdAt
	return channel, nil
}

func prepareSubscriptionChannelUpdate(
	input UpdateSubscriptionChannelInput,
) (UpdateSubscriptionChannelInput, error) {
	if err := validateSubscriptionID(input.ID, "channel"); err != nil {
		return UpdateSubscriptionChannelInput{}, err
	}
	if err := validateSubscriptionName(input.Name); err != nil {
		return UpdateSubscriptionChannelInput{}, err
	}
	if !validSubscriptionFormat(input.Format) {
		return UpdateSubscriptionChannelInput{}, fmt.Errorf("invalid subscription format %q", input.Format)
	}
	publicHost, err := normalizeSubscriptionPublicHost(input.PublicHost)
	if err != nil {
		return UpdateSubscriptionChannelInput{}, err
	}
	config, err := canonicalChannelConfig(input.Config)
	if err != nil {
		return UpdateSubscriptionChannelInput{}, err
	}
	input.ExpectedUpdatedAt, err = requiredUTC(input.ExpectedUpdatedAt, "expected channel updated_at")
	if err != nil {
		return UpdateSubscriptionChannelInput{}, err
	}
	input.UpdatedAt, err = nextSubscriptionUpdateTime(input.UpdatedAt, input.ExpectedUpdatedAt)
	if err != nil {
		return UpdateSubscriptionChannelInput{}, err
	}
	input.PublicHost = publicHost
	input.Config = config
	return input, nil
}

func normalizeSubscriptionPublicHost(value string) (string, error) {
	if value == "" || value != strings.TrimSpace(value) || len(value) > 253 ||
		strings.ContainsAny(value, "/@?#[]") {
		return "", errors.New("subscription channel public_host must be a normalized host without scheme or port")
	}
	if net.ParseIP(value) != nil {
		return strings.ToLower(value), nil
	}
	if strings.Contains(value, ":") || strings.HasPrefix(value, ".") || strings.HasSuffix(value, ".") {
		return "", errors.New("subscription channel public_host is invalid")
	}
	for _, label := range strings.Split(value, ".") {
		if label == "" || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return "", errors.New("subscription channel public_host is invalid")
		}
		for _, character := range label {
			if (character >= 'a' && character <= 'z') || (character >= 'A' && character <= 'Z') ||
				(character >= '0' && character <= '9') || character == '-' {
				continue
			}
			return "", errors.New("subscription channel public_host is invalid")
		}
	}
	return strings.ToLower(value), nil
}

func getSubscriptionChannel(ctx context.Context, q queryRower, id string) (SubscriptionChannel, error) {
	channel, err := scanSubscriptionChannel(q.QueryRowContext(
		ctx,
		`SELECT `+subscriptionChannelColumns+` FROM subscription_channels WHERE id = ?`,
		id,
	))
	if errors.Is(err, sql.ErrNoRows) {
		return SubscriptionChannel{}, fmt.Errorf("%w: %s", ErrSubscriptionChannelNotFound, id)
	}
	if err != nil {
		return SubscriptionChannel{}, fmt.Errorf("get subscription channel: %w", err)
	}
	return channel, nil
}

func scanSubscriptionChannel(row taskScanner) (SubscriptionChannel, error) {
	var channel SubscriptionChannel
	var config, createdAt, updatedAt string
	var enabled int
	if err := row.Scan(
		&channel.ID,
		&channel.Name,
		&channel.Format,
		&channel.PublicHost,
		&config,
		&enabled,
		&createdAt,
		&updatedAt,
	); err != nil {
		return SubscriptionChannel{}, err
	}
	channel.Config = append(json.RawMessage(nil), config...)
	channel.Enabled = enabled != 0
	var err error
	channel.CreatedAt, err = parseTaskTime(createdAt)
	if err != nil {
		return SubscriptionChannel{}, fmt.Errorf("parse created_at: %w", err)
	}
	channel.UpdatedAt, err = parseTaskTime(updatedAt)
	if err != nil {
		return SubscriptionChannel{}, fmt.Errorf("parse updated_at: %w", err)
	}
	return channel, nil
}

func scanSubscriptionChannelSummary(row taskScanner) (SubscriptionChannelSummary, error) {
	var channel SubscriptionChannelSummary
	var createdAt, updatedAt string
	var enabled int
	if err := row.Scan(
		&channel.ID,
		&channel.Name,
		&channel.Format,
		&channel.PublicHost,
		&enabled,
		&createdAt,
		&updatedAt,
	); err != nil {
		return SubscriptionChannelSummary{}, err
	}
	channel.Enabled = enabled != 0
	var err error
	channel.CreatedAt, err = parseTaskTime(createdAt)
	if err != nil {
		return SubscriptionChannelSummary{}, fmt.Errorf("parse created_at: %w", err)
	}
	channel.UpdatedAt, err = parseTaskTime(updatedAt)
	if err != nil {
		return SubscriptionChannelSummary{}, fmt.Errorf("parse updated_at: %w", err)
	}
	return channel, nil
}
