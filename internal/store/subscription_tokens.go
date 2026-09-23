// SPDX-License-Identifier: GPL-3.0-or-later

package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"time"
)

type SubscriptionToken struct {
	ID            string
	UserID        string
	Label         string
	DownloadLimit *int64
	TokenSHA256   string
	// Secret is accepted at creation only; ordinary token reads return metadata.
	Secret                 string `json:"-"`
	Enabled                bool
	ExpiresAt              *time.Time
	RevokedAt              *time.Time
	SuccessfulRequestCount int64
	BodyResponseCount      int64
	BytesServed            int64
	LastUsedAt             *time.Time
	CreatedAt              time.Time
}

func (s *Store) SubscriptionTokenSecret(ctx context.Context, tokenID string) (string, error) {
	var secret string
	err := s.db.QueryRowContext(ctx, `SELECT secret FROM subscription_token_secrets WHERE token_id = ?`, tokenID).Scan(&secret)
	if errors.Is(err, sql.ErrNoRows) {
		if _, lookupErr := s.GetSubscriptionToken(ctx, tokenID); lookupErr != nil {
			return "", lookupErr
		}
		return "", ErrSubscriptionTokenSecretUnavailable
	}
	if err != nil {
		return "", fmt.Errorf("read subscription token secret: %w", err)
	}
	return secret, nil
}

// Active reports whether a token is usable at the supplied instant. Expiry is
// exclusive: a token is inactive when at is equal to expires_at.
func (token SubscriptionToken) Active(at time.Time) bool {
	if !token.Enabled || token.RevokedAt != nil || token.DownloadLimit != nil && token.BodyResponseCount >= *token.DownloadLimit {
		return false
	}
	return token.ExpiresAt == nil || at.UTC().Before(token.ExpiresAt.UTC())
}

type SubscriptionTokenRotation struct {
	Revoked SubscriptionToken
	Created SubscriptionToken
}

type SubscriptionTokenListFilter struct {
	Cursor *CreatedAtCursor
	Limit  int
	Offset int
}

type SubscriptionTokenPage struct {
	Items []SubscriptionToken
	Next  *CreatedAtCursor
	Total int
}

const subscriptionChannelColumns = `
    id, name, format, public_host, config_json, enabled, created_at, updated_at`

const subscriptionSourceColumns = `
	id, name, source_kind, config_json, current_version_id, enabled,
    created_at, updated_at`

const subscriptionTokenColumns = `
    id, user_id, label, token_sha256, enabled, expires_at, revoked_at,
    successful_request_count, body_response_count, bytes_served,
    last_used_at, created_at, download_limit`

func (s *Store) CreateSubscriptionToken(
	ctx context.Context,
	token SubscriptionToken,
) (SubscriptionToken, error) {
	prepared, err := prepareNewSubscriptionToken(token)
	if err != nil {
		return SubscriptionToken{}, fmt.Errorf("%w: %v", ErrInvalidSubscriptionToken, err)
	}
	var stored SubscriptionToken
	err = s.WithTx(ctx, func(tx *sql.Tx) error {
		if prepared.UserID != "" {
			if _, err := getSubscriptionUser(ctx, tx, prepared.UserID); err != nil {
				return err
			}
		}
		if err := ensureTokenIdentityAvailable(ctx, tx, prepared.ID, prepared.TokenSHA256); err != nil {
			return err
		}
		if err := insertSubscriptionToken(ctx, tx, prepared); err != nil {
			return err
		}
		stored, err = getSubscriptionToken(ctx, tx, prepared.ID)
		return err
	})
	return stored, err
}

func (s *Store) GetSubscriptionToken(
	ctx context.Context,
	tokenID string,
) (SubscriptionToken, error) {
	if err := validateSubscriptionID(tokenID, "token"); err != nil {
		return SubscriptionToken{}, err
	}
	return getSubscriptionToken(ctx, s.db, tokenID)
}

func (s *Store) ListSubscriptionTokens(
	ctx context.Context,
	filter SubscriptionTokenListFilter,
) (SubscriptionTokenPage, error) {
	limit, err := normalizePageLimit(filter.Limit)
	if err != nil {
		return SubscriptionTokenPage{}, err
	}
	if err := validateCreatedAtCursor(filter.Cursor); err != nil {
		return SubscriptionTokenPage{}, err
	}
	if err := validatePageOffset(filter.Offset, filter.Cursor != nil); err != nil {
		return SubscriptionTokenPage{}, err
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return SubscriptionTokenPage{}, err
	}
	defer tx.Rollback()
	var total int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM subscription_tokens`).Scan(&total); err != nil {
		return SubscriptionTokenPage{}, fmt.Errorf("count subscription tokens: %w", err)
	}
	query := `SELECT ` + subscriptionTokenColumns + ` FROM subscription_tokens`
	args := make([]any, 0, 4)
	if filter.Cursor != nil {
		query += ` WHERE (created_at < ? OR (created_at = ? AND id < ?))`
		cursorTime := formatTime(filter.Cursor.CreatedAt)
		args = append(args, cursorTime, cursorTime, filter.Cursor.ID)
	}
	query += ` ORDER BY created_at DESC, id DESC LIMIT ? OFFSET ?`
	args = append(args, limit+1, filter.Offset)
	rows, err := tx.QueryContext(ctx, query, args...)
	if err != nil {
		return SubscriptionTokenPage{}, fmt.Errorf("list subscription tokens: %w", err)
	}
	defer rows.Close()
	items := make([]SubscriptionToken, 0, limit+1)
	for rows.Next() {
		token, err := scanSubscriptionToken(rows)
		if err != nil {
			return SubscriptionTokenPage{}, fmt.Errorf("scan subscription token: %w", err)
		}
		items = append(items, token)
	}
	if err := rows.Err(); err != nil {
		return SubscriptionTokenPage{}, fmt.Errorf("iterate subscription tokens: %w", err)
	}
	page := SubscriptionTokenPage{Items: items, Total: total}
	if len(items) > limit {
		page.Items = items[:limit]
		last := page.Items[len(page.Items)-1]
		page.Next = &CreatedAtCursor{CreatedAt: last.CreatedAt, ID: last.ID}
	}
	return page, tx.Commit()
}

// FindActiveSubscriptionToken resolves a one-way digest and applies immediate
// revoke/expiry policy. Callers should present the same public error for not
// found and inactive tokens.
func (s *Store) FindActiveSubscriptionToken(
	ctx context.Context,
	tokenSHA256 string,
	at time.Time,
) (SubscriptionToken, error) {
	digest, err := normalizeTokenDigest(tokenSHA256)
	if err != nil {
		return SubscriptionToken{}, err
	}
	at, err = requiredUTC(at, "token validation time")
	if err != nil {
		return SubscriptionToken{}, err
	}
	token, err := scanSubscriptionToken(s.db.QueryRowContext(
		ctx,
		`SELECT `+subscriptionTokenColumns+`
           FROM subscription_tokens WHERE token_sha256 = ?`,
		digest,
	))
	if errors.Is(err, sql.ErrNoRows) {
		return SubscriptionToken{}, ErrSubscriptionTokenNotFound
	}
	if err != nil {
		return SubscriptionToken{}, fmt.Errorf("find subscription token: %w", err)
	}
	if !token.Active(at) {
		return SubscriptionToken{}, fmt.Errorf("%w: %s", ErrSubscriptionTokenInactive, token.ID)
	}
	if token.UserID == "" {
		return token, nil
	}
	user, err := getSubscriptionUser(ctx, s.db, token.UserID)
	if err != nil || !user.Enabled {
		return SubscriptionToken{}, fmt.Errorf("%w: %s", ErrSubscriptionTokenInactive, token.ID)
	}
	return token, nil
}

// RotateSubscriptionToken atomically revokes an unrevoked token and inserts
// its replacement, preserving enablement and usage restrictions.
func (s *Store) RotateSubscriptionToken(
	ctx context.Context,
	oldTokenID string,
	replacement SubscriptionToken,
	rotatedAt time.Time,
) (SubscriptionTokenRotation, error) {
	if err := validateSubscriptionID(oldTokenID, "token"); err != nil {
		return SubscriptionTokenRotation{}, err
	}
	rotatedAt, err := requiredUTC(rotatedAt, "token rotation time")
	if err != nil {
		return SubscriptionTokenRotation{}, err
	}
	replacement.CreatedAt = rotatedAt
	prepared, err := prepareNewSubscriptionToken(replacement)
	if err != nil {
		return SubscriptionTokenRotation{}, fmt.Errorf("%w: %v", ErrInvalidSubscriptionToken, err)
	}
	var rotation SubscriptionTokenRotation
	err = s.WithTx(ctx, func(tx *sql.Tx) error {
		current, err := getSubscriptionToken(ctx, tx, oldTokenID)
		if err != nil {
			return err
		}
		if current.RevokedAt != nil {
			return fmt.Errorf("%w: %s", ErrSubscriptionTokenInactive, current.ID)
		}
		if prepared.UserID != current.UserID {
			return errors.New("rotated subscription token must retain its user")
		}
		if err := ensureTokenIdentityAvailable(ctx, tx, prepared.ID, prepared.TokenSHA256); err != nil {
			return err
		}
		result, err := tx.ExecContext(
			ctx,
			`UPDATE subscription_tokens
                SET revoked_at = ?
              WHERE id = ? AND revoked_at IS NULL`,
			formatTime(rotatedAt),
			oldTokenID,
		)
		if err != nil {
			return fmt.Errorf("revoke rotated subscription token: %w", err)
		}
		if err := requireSingleSubscriptionWrite(result, "revoke rotated subscription token"); err != nil {
			return err
		}
		// Rotation replaces the secret, not the key's shared quota or access scope.
		prepared.Enabled = current.Enabled
		prepared.DownloadLimit = current.DownloadLimit
		prepared.SuccessfulRequestCount = current.SuccessfulRequestCount
		prepared.BodyResponseCount = current.BodyResponseCount
		prepared.BytesServed = current.BytesServed
		prepared.LastUsedAt = current.LastUsedAt
		if prepared.ExpiresAt == nil {
			prepared.ExpiresAt = current.ExpiresAt
		}
		if err := insertSubscriptionToken(ctx, tx, prepared); err != nil {
			return err
		}
		rotation.Revoked, err = getSubscriptionToken(ctx, tx, oldTokenID)
		if err != nil {
			return err
		}
		rotation.Created, err = getSubscriptionToken(ctx, tx, prepared.ID)
		return err
	})
	return rotation, err
}

func (s *Store) RevokeSubscriptionToken(
	ctx context.Context,
	tokenID string,
	revokedAt time.Time,
) (SubscriptionToken, error) {
	if err := validateSubscriptionID(tokenID, "token"); err != nil {
		return SubscriptionToken{}, err
	}
	revokedAt, err := requiredUTC(revokedAt, "token revocation time")
	if err != nil {
		return SubscriptionToken{}, err
	}
	var stored SubscriptionToken
	err = s.WithTx(ctx, func(tx *sql.Tx) error {
		current, err := getSubscriptionToken(ctx, tx, tokenID)
		if err != nil {
			return err
		}
		if current.RevokedAt != nil {
			stored = current
			return nil
		}
		if revokedAt.Before(current.CreatedAt) {
			return errors.New("token revocation time precedes creation")
		}
		result, err := tx.ExecContext(
			ctx,
			`UPDATE subscription_tokens SET revoked_at = ? WHERE id = ? AND revoked_at IS NULL`,
			formatTime(revokedAt),
			tokenID,
		)
		if err != nil {
			return fmt.Errorf("revoke subscription token: %w", err)
		}
		if err := requireSingleSubscriptionWrite(result, "revoke subscription token"); err != nil {
			return err
		}
		stored, err = getSubscriptionToken(ctx, tx, tokenID)
		return err
	})
	return stored, err
}

func (s *Store) SetSubscriptionTokenEnabled(ctx context.Context, tokenID string, enabled bool) (SubscriptionToken, error) {
	if err := validateSubscriptionID(tokenID, "token"); err != nil {
		return SubscriptionToken{}, err
	}
	var stored SubscriptionToken
	err := s.WithTx(ctx, func(tx *sql.Tx) error {
		current, err := getSubscriptionToken(ctx, tx, tokenID)
		if err != nil {
			return err
		}
		if current.RevokedAt != nil && enabled {
			return fmt.Errorf("%w: revoked token cannot be enabled", ErrSubscriptionTokenInactive)
		}
		if current.Enabled == enabled {
			stored = current
			return nil
		}
		result, err := tx.ExecContext(ctx, `UPDATE subscription_tokens SET enabled = ? WHERE id = ?`, boolInt(enabled), tokenID)
		if err != nil {
			return fmt.Errorf("set subscription token enabled: %w", err)
		}
		if err := requireSingleSubscriptionWrite(result, "set subscription token enabled"); err != nil {
			return err
		}
		stored, err = getSubscriptionToken(ctx, tx, tokenID)
		return err
	})
	return stored, err
}

func (s *Store) DeleteSubscriptionToken(ctx context.Context, tokenID string) error {
	if err := validateSubscriptionID(tokenID, "token"); err != nil {
		return err
	}
	result, err := s.db.ExecContext(ctx, `DELETE FROM subscription_tokens WHERE id = ?`, tokenID)
	if err != nil {
		return fmt.Errorf("delete subscription token: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows != 1 {
		return fmt.Errorf("%w: %s", ErrSubscriptionTokenNotFound, tokenID)
	}
	return nil
}

// RecordSubscriptionTokenUse records server-side response attempts without
// retaining request addresses, user agents, URLs, or response bodies.
func (s *Store) RecordSubscriptionTokenUse(
	ctx context.Context,
	tokenID string,
	at time.Time,
	bodyBytes int64,
) error {
	if err := validateSubscriptionID(tokenID, "token"); err != nil {
		return err
	}
	at, err := requiredUTC(at, "subscription token use time")
	if err != nil {
		return err
	}
	if bodyBytes < 0 {
		return errors.New("subscription response bytes cannot be negative")
	}
	result, err := s.db.ExecContext(ctx, `UPDATE subscription_tokens
        SET successful_request_count = successful_request_count + 1,
            body_response_count = body_response_count + CASE WHEN ? > 0 THEN 1 ELSE 0 END,
            bytes_served = bytes_served + ?, last_used_at = ?
        WHERE id = ? AND enabled = 1 AND revoked_at IS NULL
          AND (expires_at IS NULL OR expires_at > ?)
          AND (user_id IS NULL OR EXISTS (SELECT 1 FROM subscription_users AS u WHERE u.id = subscription_tokens.user_id AND u.enabled = 1))
          AND (download_limit IS NULL OR body_response_count < download_limit)`,
		bodyBytes, bodyBytes, formatTime(at), tokenID, formatTime(at))
	if err != nil {
		return fmt.Errorf("record subscription token use: %w", err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count != 1 {
		return ErrSubscriptionTokenInactive
	}
	return nil
}

// DecodeSubscriptionChannelConfig validates and returns the normalized channel
// policy used by application preview rendering.
func prepareNewSubscriptionToken(token SubscriptionToken) (SubscriptionToken, error) {
	if err := validateSubscriptionID(token.ID, "token"); err != nil {
		return SubscriptionToken{}, err
	}
	if token.UserID != "" {
		if err := validateSubscriptionID(token.UserID, "user"); err != nil {
			return SubscriptionToken{}, err
		}
	}
	if token.DownloadLimit != nil && (*token.DownloadLimit < 1 || *token.DownloadLimit > 1000000000) {
		return SubscriptionToken{}, errors.New("download limit must be between 1 and 1000000000")
	}
	if err := validateSubscriptionName(token.Label); err != nil {
		return SubscriptionToken{}, fmt.Errorf("subscription token label: %w", err)
	}
	digest, err := normalizeTokenDigest(token.TokenSHA256)
	if err != nil {
		return SubscriptionToken{}, err
	}
	if token.Secret != "" {
		sum := sha256.Sum256([]byte(token.Secret))
		if len(token.Secret) > 512 || hex.EncodeToString(sum[:]) != digest {
			return SubscriptionToken{}, errors.New("subscription token secret does not match its digest")
		}
	}
	createdAt := token.CreatedAt
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	} else {
		createdAt = createdAt.UTC()
	}
	if token.ExpiresAt != nil {
		expiresAt := token.ExpiresAt.UTC()
		if !expiresAt.After(createdAt) {
			return SubscriptionToken{}, errors.New("subscription token expiry must follow creation")
		}
		token.ExpiresAt = &expiresAt
	}
	if token.RevokedAt != nil {
		return SubscriptionToken{}, errors.New("new subscription token cannot be revoked")
	}
	if token.SuccessfulRequestCount != 0 || token.BodyResponseCount != 0 || token.BytesServed != 0 || token.LastUsedAt != nil {
		return SubscriptionToken{}, errors.New("new subscription token cannot contain usage statistics")
	}
	token.TokenSHA256 = digest
	token.CreatedAt = createdAt
	return token, nil
}

func getSubscriptionToken(ctx context.Context, q queryRower, id string) (SubscriptionToken, error) {
	token, err := scanSubscriptionToken(q.QueryRowContext(
		ctx,
		`SELECT `+subscriptionTokenColumns+` FROM subscription_tokens WHERE id = ?`,
		id,
	))
	if errors.Is(err, sql.ErrNoRows) {
		return SubscriptionToken{}, fmt.Errorf("%w: %s", ErrSubscriptionTokenNotFound, id)
	}
	if err != nil {
		return SubscriptionToken{}, fmt.Errorf("get subscription token: %w", err)
	}
	return token, nil
}

func scanSubscriptionToken(row rowScanner) (SubscriptionToken, error) {
	var token SubscriptionToken
	var expiresAt, revokedAt, lastUsedAt, userID sql.NullString
	var enabled int
	var createdAt string
	if err := row.Scan(
		&token.ID,
		&userID,
		&token.Label,
		&token.TokenSHA256,
		&enabled,
		&expiresAt,
		&revokedAt,
		&token.SuccessfulRequestCount,
		&token.BodyResponseCount,
		&token.BytesServed,
		&lastUsedAt,
		&createdAt,
		&token.DownloadLimit,
	); err != nil {
		return SubscriptionToken{}, err
	}
	var err error
	token.UserID = userID.String
	token.Enabled = enabled == 1
	token.CreatedAt, err = parseTime(createdAt)
	if err != nil {
		return SubscriptionToken{}, fmt.Errorf("parse created_at: %w", err)
	}
	if expiresAt.Valid {
		parsed, err := parseTime(expiresAt.String)
		if err != nil {
			return SubscriptionToken{}, fmt.Errorf("parse expires_at: %w", err)
		}
		token.ExpiresAt = &parsed
	}
	if revokedAt.Valid {
		parsed, err := parseTime(revokedAt.String)
		if err != nil {
			return SubscriptionToken{}, fmt.Errorf("parse revoked_at: %w", err)
		}
		token.RevokedAt = &parsed
	}
	if lastUsedAt.Valid {
		parsed, err := parseTime(lastUsedAt.String)
		if err != nil {
			return SubscriptionToken{}, fmt.Errorf("parse last_used_at: %w", err)
		}
		token.LastUsedAt = &parsed
	}
	return token, nil
}
