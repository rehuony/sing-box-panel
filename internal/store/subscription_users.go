// SPDX-License-Identifier: GPL-3.0-or-later

package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

var (
	ErrSubscriptionUserNotFound = errors.New("subscription user not found")
	ErrSubscriptionUserExists   = errors.New("subscription user already exists")
)

type SubscriptionUser struct {
	ID          string
	Name        string
	Description string
	Enabled     bool
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type SubscriptionUserListFilter struct {
	Cursor *CreatedAtCursor
	Limit  int
}

type SubscriptionUserPage struct {
	Items []SubscriptionUser
	Next  *CreatedAtCursor
}

type UpdateSubscriptionUserInput struct {
	ID                string
	Name              string
	Description       string
	Enabled           bool
	ExpectedUpdatedAt time.Time
	UpdatedAt         time.Time
}

type SubscriptionUserGrants struct {
	User   SubscriptionUser
	Grants []string
}

const subscriptionUserColumns = `
    id, name, description, enabled, created_at, updated_at`

func (s *Store) CreateSubscriptionUser(ctx context.Context, user SubscriptionUser) (SubscriptionUser, error) {
	prepared, err := prepareNewSubscriptionUser(user)
	if err != nil {
		return SubscriptionUser{}, err
	}
	var stored SubscriptionUser
	err = s.WithTx(ctx, func(tx *sql.Tx) error {
		if err := ensureSubscriptionUserIdentity(ctx, tx, prepared.ID, prepared.Name, ""); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, `INSERT INTO subscription_users(
            id, name, description, enabled, created_at, updated_at
        ) VALUES (?, ?, ?, ?, ?, ?)`,
			prepared.ID, prepared.Name, prepared.Description, boolInt(prepared.Enabled),
			formatTaskTime(prepared.CreatedAt), formatTaskTime(prepared.UpdatedAt))
		if err != nil {
			return fmt.Errorf("insert subscription user: %w", err)
		}
		stored, err = getSubscriptionUser(ctx, tx, prepared.ID)
		return err
	})
	return stored, err
}

func (s *Store) GetSubscriptionUser(ctx context.Context, id string) (SubscriptionUser, error) {
	if err := validateSubscriptionID(id, "user"); err != nil {
		return SubscriptionUser{}, err
	}
	return getSubscriptionUser(ctx, s.db, id)
}

func (s *Store) ListSubscriptionUsers(ctx context.Context, filter SubscriptionUserListFilter) (SubscriptionUserPage, error) {
	limit, err := normalizePageLimit(filter.Limit)
	if err != nil {
		return SubscriptionUserPage{}, err
	}
	if err := validateCreatedAtCursor(filter.Cursor); err != nil {
		return SubscriptionUserPage{}, err
	}
	query := `SELECT ` + subscriptionUserColumns + ` FROM subscription_users`
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
		return SubscriptionUserPage{}, fmt.Errorf("list subscription users: %w", err)
	}
	defer rows.Close()
	items := make([]SubscriptionUser, 0, limit+1)
	for rows.Next() {
		user, scanErr := scanSubscriptionUser(rows)
		if scanErr != nil {
			return SubscriptionUserPage{}, fmt.Errorf("scan subscription user: %w", scanErr)
		}
		items = append(items, user)
	}
	if err := rows.Err(); err != nil {
		return SubscriptionUserPage{}, fmt.Errorf("iterate subscription users: %w", err)
	}
	page := SubscriptionUserPage{Items: items}
	if len(items) > limit {
		page.Items = items[:limit]
		last := page.Items[len(page.Items)-1]
		page.Next = &CreatedAtCursor{CreatedAt: last.CreatedAt, ID: last.ID}
	}
	return page, nil
}

func (s *Store) UpdateSubscriptionUser(ctx context.Context, input UpdateSubscriptionUserInput) (SubscriptionUser, error) {
	prepared, err := prepareSubscriptionUserUpdate(input)
	if err != nil {
		return SubscriptionUser{}, err
	}
	var stored SubscriptionUser
	err = s.WithTx(ctx, func(tx *sql.Tx) error {
		current, err := getSubscriptionUser(ctx, tx, prepared.ID)
		if err != nil {
			return err
		}
		if !current.UpdatedAt.Equal(prepared.ExpectedUpdatedAt) {
			return subscriptionConflict("user", prepared.ID, prepared.ExpectedUpdatedAt, current.UpdatedAt)
		}
		if err := ensureSubscriptionUserIdentity(ctx, tx, prepared.ID, prepared.Name, prepared.ID); err != nil {
			return err
		}
		result, err := tx.ExecContext(ctx, `UPDATE subscription_users
            SET name = ?, description = ?, enabled = ?, updated_at = ?
            WHERE id = ? AND updated_at = ?`,
			prepared.Name, prepared.Description, boolInt(prepared.Enabled), formatTaskTime(prepared.UpdatedAt),
			prepared.ID, formatTaskTime(prepared.ExpectedUpdatedAt))
		if err != nil {
			return fmt.Errorf("update subscription user: %w", err)
		}
		if err := requireSingleSubscriptionWrite(result, "update subscription user"); err != nil {
			return err
		}
		stored, err = getSubscriptionUser(ctx, tx, prepared.ID)
		return err
	})
	return stored, err
}

func (s *Store) DeleteSubscriptionUser(ctx context.Context, id string, expectedUpdatedAt time.Time) error {
	if err := validateSubscriptionID(id, "user"); err != nil {
		return err
	}
	expected, err := requiredUTC(expectedUpdatedAt, "expected user updated_at")
	if err != nil {
		return err
	}
	return s.WithTx(ctx, func(tx *sql.Tx) error {
		current, err := getSubscriptionUser(ctx, tx, id)
		if err != nil {
			return err
		}
		if !current.UpdatedAt.Equal(expected) {
			return subscriptionConflict("user", id, expected, current.UpdatedAt)
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM subscription_tokens WHERE user_id = ?`, id); err != nil {
			return fmt.Errorf("delete subscription user tokens: %w", err)
		}
		result, err := tx.ExecContext(ctx, `DELETE FROM subscription_users WHERE id = ? AND updated_at = ?`, id, formatTaskTime(expected))
		if err != nil {
			return fmt.Errorf("delete subscription user: %w", err)
		}
		return requireSingleSubscriptionWrite(result, "delete subscription user")
	})
}

func (s *Store) SubscriptionUserGrants(ctx context.Context, userID string) (SubscriptionUserGrants, error) {
	if err := validateSubscriptionID(userID, "user"); err != nil {
		return SubscriptionUserGrants{}, err
	}
	user, err := getSubscriptionUser(ctx, s.db, userID)
	if err != nil {
		return SubscriptionUserGrants{}, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT node_key FROM subscription_user_node_grants WHERE user_id = ? ORDER BY node_key`, userID)
	if err != nil {
		return SubscriptionUserGrants{}, fmt.Errorf("list subscription user grants: %w", err)
	}
	defer rows.Close()
	grants := make([]string, 0)
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			return SubscriptionUserGrants{}, err
		}
		grants = append(grants, key)
	}
	return SubscriptionUserGrants{User: user, Grants: grants}, rows.Err()
}

func (s *Store) ReplaceSubscriptionUserGrants(
	ctx context.Context,
	userID string,
	nodeKeys []string,
	expectedUpdatedAt time.Time,
	updatedAt time.Time,
) (SubscriptionUserGrants, error) {
	if err := validateSubscriptionID(userID, "user"); err != nil {
		return SubscriptionUserGrants{}, err
	}
	expected, err := requiredUTC(expectedUpdatedAt, "expected user updated_at")
	if err != nil {
		return SubscriptionUserGrants{}, err
	}
	updated, err := nextSubscriptionUpdateTime(updatedAt, expected)
	if err != nil {
		return SubscriptionUserGrants{}, err
	}
	keys, err := normalizeNodeGrantKeys(nodeKeys)
	if err != nil {
		return SubscriptionUserGrants{}, err
	}
	var result SubscriptionUserGrants
	err = s.WithTx(ctx, func(tx *sql.Tx) error {
		user, err := getSubscriptionUser(ctx, tx, userID)
		if err != nil {
			return err
		}
		if !user.UpdatedAt.Equal(expected) {
			return subscriptionConflict("user", userID, expected, user.UpdatedAt)
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM subscription_user_node_grants WHERE user_id = ?`, userID); err != nil {
			return fmt.Errorf("replace subscription user grants: %w", err)
		}
		for _, key := range keys {
			if _, err := tx.ExecContext(ctx, `INSERT INTO subscription_user_node_grants(user_id, node_key, created_at) VALUES (?, ?, ?)`, userID, key, formatTaskTime(updated)); err != nil {
				return fmt.Errorf("insert subscription user grant: %w", err)
			}
		}
		write, err := tx.ExecContext(ctx, `UPDATE subscription_users SET updated_at = ? WHERE id = ? AND updated_at = ?`, formatTaskTime(updated), userID, formatTaskTime(expected))
		if err != nil {
			return fmt.Errorf("advance subscription user grant version: %w", err)
		}
		if err := requireSingleSubscriptionWrite(write, "advance subscription user grant version"); err != nil {
			return err
		}
		user.UpdatedAt = updated
		result = SubscriptionUserGrants{User: user, Grants: keys}
		return nil
	})
	return result, err
}

func prepareNewSubscriptionUser(user SubscriptionUser) (SubscriptionUser, error) {
	if err := validateSubscriptionID(user.ID, "user"); err != nil {
		return SubscriptionUser{}, err
	}
	if err := validateSubscriptionName(user.Name); err != nil {
		return SubscriptionUser{}, err
	}
	if err := validateSubscriptionDescription(user.Description); err != nil {
		return SubscriptionUser{}, err
	}
	if user.CreatedAt.IsZero() {
		user.CreatedAt = time.Now().UTC()
	} else {
		user.CreatedAt = user.CreatedAt.UTC()
	}
	user.UpdatedAt = user.CreatedAt
	return user, nil
}

func prepareSubscriptionUserUpdate(input UpdateSubscriptionUserInput) (UpdateSubscriptionUserInput, error) {
	if err := validateSubscriptionID(input.ID, "user"); err != nil {
		return UpdateSubscriptionUserInput{}, err
	}
	if err := validateSubscriptionName(input.Name); err != nil {
		return UpdateSubscriptionUserInput{}, err
	}
	if err := validateSubscriptionDescription(input.Description); err != nil {
		return UpdateSubscriptionUserInput{}, err
	}
	var err error
	input.ExpectedUpdatedAt, err = requiredUTC(input.ExpectedUpdatedAt, "expected user updated_at")
	if err != nil {
		return UpdateSubscriptionUserInput{}, err
	}
	input.UpdatedAt, err = nextSubscriptionUpdateTime(input.UpdatedAt, input.ExpectedUpdatedAt)
	return input, err
}

func getSubscriptionUser(ctx context.Context, q queryRower, id string) (SubscriptionUser, error) {
	user, err := scanSubscriptionUser(q.QueryRowContext(ctx, `SELECT `+subscriptionUserColumns+` FROM subscription_users WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return SubscriptionUser{}, fmt.Errorf("%w: %s", ErrSubscriptionUserNotFound, id)
	}
	if err != nil {
		return SubscriptionUser{}, fmt.Errorf("get subscription user: %w", err)
	}
	return user, nil
}

func scanSubscriptionUser(row taskScanner) (SubscriptionUser, error) {
	var user SubscriptionUser
	var enabled int
	var createdAt, updatedAt string
	if err := row.Scan(&user.ID, &user.Name, &user.Description, &enabled, &createdAt, &updatedAt); err != nil {
		return SubscriptionUser{}, err
	}
	user.Enabled = enabled == 1
	var err error
	user.CreatedAt, err = parseTaskTime(createdAt)
	if err != nil {
		return SubscriptionUser{}, fmt.Errorf("parse user created_at: %w", err)
	}
	user.UpdatedAt, err = parseTaskTime(updatedAt)
	if err != nil {
		return SubscriptionUser{}, fmt.Errorf("parse user updated_at: %w", err)
	}
	return user, nil
}

func ensureSubscriptionUserIdentity(ctx context.Context, q queryRower, id, name, excludeID string) error {
	var existing string
	err := q.QueryRowContext(ctx, `SELECT id FROM subscription_users WHERE (id = ? OR name = ?) AND id <> ? ORDER BY id LIMIT 1`, id, name, excludeID).Scan(&existing)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("check subscription user identity: %w", err)
	}
	return fmt.Errorf("%w: id or name belongs to %s", ErrSubscriptionUserExists, existing)
}

func validateSubscriptionDescription(value string) error {
	if len(value) > 1024 || !utf8.ValidString(value) {
		return errors.New("subscription user description must be valid UTF-8 of at most 1024 bytes")
	}
	for _, character := range value {
		if character == '\x00' || (unicode.IsControl(character) && character != '\n' && character != '\t') {
			return errors.New("subscription user description contains a control character")
		}
	}
	return nil
}

func normalizeNodeGrantKeys(values []string) ([]string, error) {
	if len(values) > 10_000 {
		return nil, fmt.Errorf("%w: too many node grants", ErrSubscriptionLimitExceeded)
	}
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		if value == "" || value != strings.TrimSpace(value) || len(value) > 512 || !utf8.ValidString(value) || strings.ContainsRune(value, '\x00') {
			return nil, errors.New("subscription node grant key is invalid")
		}
		if _, duplicate := seen[value]; duplicate {
			return nil, errors.New("subscription node grant key is duplicated")
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	sort.Strings(result)
	return result, nil
}
