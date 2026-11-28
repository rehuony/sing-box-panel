// SPDX-License-Identifier: GPL-3.0-or-later

package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

const (
	maximumSubscriptionNameBytes = 128

	// MaximumSubscriptionNodes bounds the node pool and one source's nodes.
	MaximumSubscriptionNodes = 10_000

	MaximumEnabledSubscriptionChannels = 256
	// MaximumSubscriptionSources bounds the sources, whose stored contents
	// together stay within MaximumSubscriptionInputBytes.
	MaximumSubscriptionSources          = 256
	MaximumSubscriptionInputBytes int64 = 32 << 20

	// MaximumChannelRules bounds the rules a channel's enabled exit rules
	// hold together, counting every rule of each rule set they route by.
	MaximumChannelRules = 20_000
)

var (
	ErrSubscriptionChannelNotFound        = errors.New("subscription channel not found")
	ErrSubscriptionChannelExists          = errors.New("subscription channel already exists")
	ErrSubscriptionSourceNotFound         = errors.New("subscription source not found")
	ErrSubscriptionSourceExists           = errors.New("subscription source already exists")
	ErrSubscriptionRuleSetNotFound        = errors.New("subscription rule set not found")
	ErrSubscriptionRuleSetExists          = errors.New("subscription rule set already exists")
	ErrInvalidSubscriptionToken           = errors.New("invalid subscription token")
	ErrSubscriptionTokenSecretUnavailable = errors.New("subscription token secret unavailable")
	ErrSubscriptionTokenNotFound          = errors.New("subscription token not found")
	ErrSubscriptionTokenExists            = errors.New("subscription token already exists")
	ErrSubscriptionTokenInactive          = errors.New("subscription token is expired or revoked")
	ErrSubscriptionConflict               = errors.New("subscription resource changed")
	ErrSubscriptionLimitExceeded          = errors.New("subscription resource limit exceeded")
	ErrSubscriptionReference              = errors.New("subscription reference is invalid")

	// ErrSubscriptionSourcesChanged, ErrSubscriptionChannelsChanged and
	// ErrSubscriptionTokensChanged refuse an order that does not name exactly
	// the current sources, channels or keys.
	ErrSubscriptionSourcesChanged  = errors.New("subscription sources changed")
	ErrSubscriptionChannelsChanged = errors.New("subscription channels changed")
	ErrSubscriptionTokensChanged   = errors.New("subscription tokens changed")
)

// SubscriptionReferenceError names a reference to a pool node or rule set
// that does not exist, a rule set of another client, or rules a channel
// cannot hold. Path locates it in the request, Code is fixed.
type SubscriptionReferenceError struct {
	Path string
	Code string
}

func (err *SubscriptionReferenceError) Error() string {
	return fmt.Sprintf("%v: %s: %s", ErrSubscriptionReference, err.Path, err.Code)
}

func (err *SubscriptionReferenceError) Unwrap() error { return ErrSubscriptionReference }

// SubscriptionConflictError reports a failed updated_at compare-and-swap.
type SubscriptionConflictError struct {
	Resource string
	ID       string
	Expected time.Time
	Actual   time.Time
}

func (err *SubscriptionConflictError) Error() string {
	return fmt.Sprintf(
		"%v: %s %q expected updated_at %s, actual %s",
		ErrSubscriptionConflict,
		err.Resource,
		err.ID,
		err.Expected.UTC().Format(time.RFC3339Nano),
		err.Actual.UTC().Format(time.RFC3339Nano),
	)
}

func (err *SubscriptionConflictError) Unwrap() error { return ErrSubscriptionConflict }

func ensureSubscriptionChannelLimits(
	ctx context.Context,
	q queryRower,
	excludeID string,
	enabled bool,
) error {
	if !enabled {
		return nil
	}
	var count int64
	if err := q.QueryRowContext(
		ctx,
		`SELECT count(*) FROM subscription_channels WHERE enabled = 1 AND id <> ?`,
		excludeID,
	).Scan(&count); err != nil {
		return fmt.Errorf("count enabled subscription channels: %w", err)
	}
	if count >= MaximumEnabledSubscriptionChannels {
		return fmt.Errorf("%w: enabled channel count exceeds %d", ErrSubscriptionLimitExceeded, MaximumEnabledSubscriptionChannels)
	}
	return nil
}

func ensureChannelIdentityAvailable(
	ctx context.Context,
	q queryRower,
	id string,
	name string,
	excludeID string,
) error {
	var existingID string
	err := q.QueryRowContext(
		ctx,
		`SELECT id FROM subscription_channels
          WHERE (id = ? OR name = ?) AND id <> ?
          ORDER BY id LIMIT 1`,
		id,
		name,
		excludeID,
	).Scan(&existingID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("check subscription channel identity: %w", err)
	}
	return fmt.Errorf("%w: id or name belongs to %s", ErrSubscriptionChannelExists, existingID)
}

func ensureSourceIdentityAvailable(
	ctx context.Context,
	q queryRower,
	id string,
	name string,
	excludeID string,
) error {
	var existingID string
	err := q.QueryRowContext(
		ctx,
		`SELECT id FROM subscription_sources
          WHERE (id = ? OR name = ?) AND id <> ?
          ORDER BY id LIMIT 1`,
		id,
		name,
		excludeID,
	).Scan(&existingID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("check subscription source identity: %w", err)
	}
	return fmt.Errorf("%w: id or name belongs to %s", ErrSubscriptionSourceExists, existingID)
}

func ensureTokenIdentityAvailable(ctx context.Context, q queryRower, id string, digest string) error {
	var existingID string
	err := q.QueryRowContext(
		ctx,
		`SELECT id FROM subscription_tokens
          WHERE id = ? OR token_sha256 = ?
          ORDER BY id LIMIT 1`,
		id,
		digest,
	).Scan(&existingID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("check subscription token identity: %w", err)
	}
	return fmt.Errorf("%w: id or digest belongs to %s", ErrSubscriptionTokenExists, existingID)
}

// saveSubscriptionOrder saves the order of the rows of table that scope
// selects: each takes its ID's index in ids as its position. ids must name
// each of those rows exactly once; otherwise they changed since they were
// read and it fails with changed. No row's updated_at changes.
func (s *Store) saveSubscriptionOrder(ctx context.Context, table string, ids []string, changed error, scope string, args ...any) error {
	return s.WithTx(ctx, func(tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, `SELECT id FROM `+table+` `+scope, args...)
		if err != nil {
			return fmt.Errorf("read %s: %w", table, err)
		}
		current := map[string]bool{}
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err != nil {
				_ = rows.Close()
				return err
			}
			current[id] = true
		}
		if err := rows.Close(); err != nil {
			return err
		}
		if err := rows.Err(); err != nil {
			return err
		}
		if len(ids) != len(current) {
			return changed
		}
		for _, id := range ids {
			if !current[id] {
				return changed
			}
			delete(current, id)
		}
		statement, err := tx.PrepareContext(ctx, `UPDATE `+table+` SET position = ? WHERE id = ?`)
		if err != nil {
			return err
		}
		defer statement.Close()
		for position, id := range ids {
			if _, err := statement.ExecContext(ctx, position, id); err != nil {
				return fmt.Errorf("save %s order: %w", table, err)
			}
		}
		return nil
	})
}

// insertSubscriptionToken stores a new key at position in the keys' order,
// or at its end when position is nil.
func insertSubscriptionToken(ctx context.Context, tx *sql.Tx, token SubscriptionToken, position *int64) error {
	_, err := tx.ExecContext(
		ctx,
		`INSERT INTO subscription_tokens(
			id, user_id, label, token_sha256, enabled, expires_at, revoked_at, created_at, download_limit,
            successful_request_count, body_response_count, bytes_served, last_used_at, position
		 ) VALUES (?, NULLIF(?, ''), ?, ?, ?, ?, NULL, ?, ?, ?, ?, ?, ?,
            coalesce(?, (SELECT coalesce(max(position) + 1, 0) FROM subscription_tokens)))`,
		token.ID,
		token.UserID,
		token.Label,
		token.TokenSHA256,
		boolInt(token.Enabled),
		nullableTime(token.ExpiresAt),
		formatTime(token.CreatedAt),
		token.DownloadLimit, token.SuccessfulRequestCount, token.BodyResponseCount, token.BytesServed, nullableTime(token.LastUsedAt),
		position,
	)
	if err != nil {
		return fmt.Errorf("insert subscription token: %w", err)
	}
	if token.Secret != "" {
		if _, err := tx.ExecContext(ctx, `INSERT INTO subscription_token_secrets(token_id, secret) VALUES (?, ?)`, token.ID, token.Secret); err != nil {
			return fmt.Errorf("store subscription token secret: %w", err)
		}
	}
	return nil
}

func validateSubscriptionID(value string, kind string) error {
	if value == "" || value != strings.TrimSpace(value) || len(value) > 128 || strings.ContainsRune(value, '\x00') {
		return fmt.Errorf("subscription %s id is invalid", kind)
	}
	return nil
}

func validateSubscriptionName(value string) error {
	if value == "" || value != strings.TrimSpace(value) || len(value) > maximumSubscriptionNameBytes ||
		!utf8.ValidString(value) {
		return errors.New("subscription name must be normalized non-empty UTF-8 of at most 128 bytes")
	}
	for _, character := range value {
		if unicode.IsControl(character) {
			return errors.New("subscription name contains a control character")
		}
	}
	return nil
}

func validSubscriptionFormat(value SubscriptionFormat) bool {
	return value == SubscriptionFormatSingBox || value == SubscriptionFormatMihomo || value == SubscriptionFormatLoon
}

func validSubscriptionSourceKind(value SubscriptionSourceKind) bool {
	return value == SubscriptionSourceURL || value == SubscriptionSourceFile
}

func normalizeTokenDigest(value string) (string, error) {
	if len(value) != sha256.Size*2 || strings.ToLower(value) != value {
		return "", errors.New("subscription token digest must be lowercase SHA-256")
	}
	decoded, err := hex.DecodeString(value)
	if err != nil || len(decoded) != sha256.Size {
		return "", errors.New("subscription token digest must be lowercase SHA-256")
	}
	return value, nil
}

func requiredUTC(value time.Time, name string) (time.Time, error) {
	if value.IsZero() {
		return time.Time{}, fmt.Errorf("%s is required", name)
	}
	return value.UTC(), nil
}

func nextSubscriptionUpdateTime(value time.Time, expected time.Time) (time.Time, error) {
	value, err := requiredUTC(value, "subscription updated_at")
	if err != nil {
		return time.Time{}, err
	}
	if !value.After(expected) {
		return time.Time{}, errors.New("subscription updated_at must advance beyond expected updated_at")
	}
	return value, nil
}

func subscriptionConflict(resource string, id string, expected, actual time.Time) error {
	return &SubscriptionConflictError{
		Resource: resource,
		ID:       id,
		Expected: expected,
		Actual:   actual,
	}
}

func requireSingleSubscriptionWrite(result sql.Result, operation string) error {
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("inspect %s: %w", operation, err)
	}
	if rows != 1 {
		return fmt.Errorf("%s affected %d rows, want 1", operation, rows)
	}
	return nil
}
