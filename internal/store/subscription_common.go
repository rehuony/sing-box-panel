// SPDX-License-Identifier: GPL-3.0-or-later

package store

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/rehuony/sing-box-panel/internal/jsonstrict"
)

const (
	maximumSubscriptionNameBytes   = 128
	maximumSubscriptionConfigBytes = 64 << 10
	maximumSourceSnapshotBytes     = 4 << 20
	maximumChannelExclusions       = 10_000

	MaximumEnabledSubscriptionChannels       = 256
	MaximumEnabledSubscriptionSources        = 256
	MaximumSubscriptionInputBytes      int64 = 32 << 20
)

var (
	ErrSubscriptionChannelNotFound = errors.New("subscription channel not found")
	ErrSubscriptionChannelExists   = errors.New("subscription channel already exists")
	ErrSubscriptionSourceNotFound  = errors.New("subscription source not found")
	ErrSubscriptionSourceExists    = errors.New("subscription source already exists")
	ErrSubscriptionTokenNotFound   = errors.New("subscription token not found")
	ErrSubscriptionTokenExists     = errors.New("subscription token already exists")
	ErrSubscriptionTokenInactive   = errors.New("subscription token is expired or revoked")
	ErrSubscriptionConflict        = errors.New("subscription resource changed")
	ErrSubscriptionLimitExceeded   = errors.New("subscription resource limit exceeded")
)

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

// SubscriptionFormat identifies one renderer contract.
func canonicalChannelConfig(raw json.RawMessage) (json.RawMessage, error) {
	config, err := DecodeSubscriptionChannelConfig(raw)
	if err != nil {
		return nil, err
	}
	encoded, err := json.Marshal(config)
	if err != nil {
		return nil, fmt.Errorf("encode subscription channel config: %w", err)
	}
	return encoded, nil
}

func strictJSONObject(raw json.RawMessage, maximum int64, fallback string) (json.RawMessage, error) {
	if len(raw) == 0 {
		raw = json.RawMessage(fallback)
	}
	var value map[string]any
	if err := jsonstrict.Decode(raw, maximum, &value); err != nil {
		return nil, err
	}
	if value == nil {
		return nil, errors.New("value must be a non-null JSON object")
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("encode JSON object: %w", err)
	}
	return bytes.Clone(encoded), nil
}

func ensureSubscriptionChannelLimits(
	ctx context.Context,
	q queryRower,
	excludeID string,
	candidate SubscriptionChannel,
) error {
	if !candidate.Enabled {
		return nil
	}
	var enabled int64
	if err := q.QueryRowContext(
		ctx,
		`SELECT count(*) FROM subscription_channels WHERE enabled = 1 AND id <> ?`,
		excludeID,
	).Scan(&enabled); err != nil {
		return fmt.Errorf("count enabled subscription channels: %w", err)
	}
	if enabled >= MaximumEnabledSubscriptionChannels {
		return fmt.Errorf("%w: enabled channel count exceeds %d", ErrSubscriptionLimitExceeded, MaximumEnabledSubscriptionChannels)
	}
	return ensureSubscriptionInputBudget(ctx, q, excludeID, "", int64(len(candidate.Config)))
}

func ensureSubscriptionSourceLimits(
	ctx context.Context,
	q queryRower,
	excludeID string,
	candidate SubscriptionSource,
) error {
	if !candidate.Enabled {
		return nil
	}
	var enabled int64
	if err := q.QueryRowContext(
		ctx,
		`SELECT count(*) FROM subscription_sources WHERE enabled = 1 AND id <> ?`,
		excludeID,
	).Scan(&enabled); err != nil {
		return fmt.Errorf("count enabled subscription sources: %w", err)
	}
	if enabled >= MaximumEnabledSubscriptionSources {
		return fmt.Errorf("%w: enabled source count exceeds %d", ErrSubscriptionLimitExceeded, MaximumEnabledSubscriptionSources)
	}
	candidateBytes := int64(len(candidate.Config))
	return ensureSubscriptionInputBudget(ctx, q, "", excludeID, candidateBytes)
}

func ensureSubscriptionInputBudget(
	ctx context.Context,
	q queryRower,
	excludeChannelID string,
	excludeSourceID string,
	candidateBytes int64,
) error {
	var storedBytes int64
	if err := q.QueryRowContext(
		ctx,
		`SELECT
            coalesce((
                SELECT sum(length(config_json))
                  FROM subscription_channels
                 WHERE enabled = 1 AND id <> ?
            ), 0) +
            coalesce((
				SELECT sum(length(config_json))
                  FROM subscription_sources
                 WHERE enabled = 1 AND id <> ?
            ), 0)`,
		excludeChannelID,
		excludeSourceID,
	).Scan(&storedBytes); err != nil {
		return fmt.Errorf("measure enabled subscription inputs: %w", err)
	}
	if candidateBytes < 0 || storedBytes > MaximumSubscriptionInputBytes-candidateBytes {
		return fmt.Errorf("%w: enabled input bytes exceed %d", ErrSubscriptionLimitExceeded, MaximumSubscriptionInputBytes)
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

func insertSubscriptionToken(ctx context.Context, tx *sql.Tx, token SubscriptionToken) error {
	_, err := tx.ExecContext(
		ctx,
		`INSERT INTO subscription_tokens(
			id, user_id, label, token_sha256, enabled, expires_at, revoked_at, created_at
		 ) VALUES (?, ?, ?, ?, ?, ?, NULL, ?)`,
		token.ID,
		token.UserID,
		token.Label,
		token.TokenSHA256,
		boolInt(token.Enabled),
		nullableSubscriptionTime(token.ExpiresAt),
		formatTaskTime(token.CreatedAt),
	)
	if err != nil {
		return fmt.Errorf("insert subscription token: %w", err)
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
	return value == SubscriptionSourceRemote || value == SubscriptionSourceLocal
}

func validateUniqueSubscriptionStrings(
	values []string,
	valid func(string) bool,
	kind string,
) error {
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		if !valid(value) {
			return fmt.Errorf("subscription channel config contains an invalid %s exclusion", kind)
		}
		if _, duplicate := seen[value]; duplicate {
			return fmt.Errorf("subscription channel config contains a duplicate %s exclusion", kind)
		}
		seen[value] = struct{}{}
	}
	return nil
}

func validSubscriptionTag(value string) bool {
	return value != "" && len(value) <= 512 && utf8.ValidString(value) && !strings.ContainsRune(value, '\x00')
}

func validSubscriptionType(value string) bool {
	if value == "" || len(value) > 64 {
		return false
	}
	for _, character := range value {
		if (character >= 'a' && character <= 'z') || (character >= '0' && character <= '9') ||
			character == '-' || character == '_' {
			continue
		}
		return false
	}
	return true
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

func nullableSubscriptionTime(value *time.Time) any {
	if value == nil {
		return nil
	}
	return formatTaskTime(*value)
}
