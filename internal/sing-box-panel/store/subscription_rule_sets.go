// SPDX-License-Identifier: GPL-3.0-or-later

package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"time"
)

// MaximumRuleSetRulesBytes bounds a rule set's serialized rules.
const MaximumRuleSetRulesBytes = 640 << 10

// DefaultRuleSetIcon is the icon of a rule set created without one.
const DefaultRuleSetIcon = "layers"

// ErrSubscriptionRuleSetsChanged refuses an order that does not name exactly
// the client's current rule sets.
var ErrSubscriptionRuleSetsChanged = errors.New("subscription rule sets changed")

// ruleSetIcon is the form of an icon key; the Web application decides which
// keys it draws.
var ruleSetIcon = regexp.MustCompile(`^[a-z][a-z0-9-]{0,31}$`)

// ValidateRuleSetIcon checks the form of a rule set's icon key.
func ValidateRuleSetIcon(icon string) error {
	if !ruleSetIcon.MatchString(icon) {
		return errors.New("a rule set icon must be 1 to 32 lower-case letters, digits or hyphens, starting with a letter")
	}
	return nil
}

// SubscriptionRuleSet is a client's reusable, ordered rules. The
// subscription service validates the rules; the store keeps them as given.
type SubscriptionRuleSet struct {
	ID     string
	Client SubscriptionFormat
	Name   string
	// Icon is the key of the icon the Web application draws for the set.
	Icon string
	// Rules is the JSON array of the rules in order.
	Rules json.RawMessage
	// Channels are the channels whose exit rules route by the rule set, in
	// name order.
	Channels  []SubscriptionRuleSetChannel
	CreatedAt time.Time
	UpdatedAt time.Time
}

type SubscriptionRuleSetChannel struct {
	ID   string
	Name string
}

// UpdateSubscriptionRuleSetInput replaces a rule set's name, icon and rules
// when its updated_at is still ExpectedUpdatedAt. Its client never changes.
type UpdateSubscriptionRuleSetInput struct {
	ID   string
	Name string
	// Icon replaces the set's icon; empty keeps it.
	Icon              string
	Rules             json.RawMessage
	ExpectedUpdatedAt time.Time
	UpdatedAt         time.Time
}

const subscriptionRuleSetColumns = `id, client, name, icon, rules, created_at, updated_at`

// CreateSubscriptionRuleSet stores a new rule set at the end of its client's
// order.
func (s *Store) CreateSubscriptionRuleSet(ctx context.Context, set SubscriptionRuleSet) (SubscriptionRuleSet, error) {
	prepared, err := prepareSubscriptionRuleSet(set)
	if err != nil {
		return SubscriptionRuleSet{}, err
	}
	var stored SubscriptionRuleSet
	err = s.WithTx(ctx, func(tx *sql.Tx) error {
		if err := ensureRuleSetIdentityAvailable(ctx, tx, prepared.ID, prepared.Client, prepared.Name, ""); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO subscription_rule_sets(`+subscriptionRuleSetColumns+`, position)
			VALUES (?, ?, ?, ?, ?, ?, ?, (SELECT coalesce(max(position) + 1, 0) FROM subscription_rule_sets WHERE client = ?))`,
			prepared.ID, string(prepared.Client), prepared.Name, prepared.Icon, string(prepared.Rules),
			formatTime(prepared.CreatedAt), formatTime(prepared.UpdatedAt), string(prepared.Client)); err != nil {
			return fmt.Errorf("insert subscription rule set: %w", err)
		}
		stored, err = getSubscriptionRuleSet(ctx, tx, prepared.ID)
		return err
	})
	return stored, err
}

func (s *Store) GetSubscriptionRuleSet(ctx context.Context, ruleSetID string) (SubscriptionRuleSet, error) {
	if err := validateSubscriptionID(ruleSetID, "rule set"); err != nil {
		return SubscriptionRuleSet{}, fmt.Errorf("%w: %s", ErrSubscriptionRuleSetNotFound, ruleSetID)
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return SubscriptionRuleSet{}, err
	}
	defer func() { _ = tx.Rollback() }()
	set, err := getSubscriptionRuleSet(ctx, tx, ruleSetID)
	if err != nil {
		return SubscriptionRuleSet{}, err
	}
	return set, tx.Commit()
}

// ListSubscriptionRuleSets returns every rule set, or only client's when it
// is not empty, each client's sets in their saved order.
func (s *Store) ListSubscriptionRuleSets(ctx context.Context, client SubscriptionFormat) ([]SubscriptionRuleSet, error) {
	if client != "" && !validSubscriptionFormat(client) {
		return nil, fmt.Errorf("%w: invalid rule set client %q", ErrInvalidQuery, client)
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	sets, err := loadSubscriptionRuleSets(ctx, tx, `WHERE ? = '' OR client = ? ORDER BY client, position, id`, string(client), string(client))
	if err != nil {
		return nil, err
	}
	indices := make(map[string]int, len(sets))
	for index, set := range sets {
		indices[set.ID] = index
	}
	rows, err := tx.QueryContext(ctx, `SELECT DISTINCT r.rule_set_id, c.id, c.name
		FROM subscription_channel_routes AS r JOIN subscription_channels AS c ON c.id = r.channel_id
		WHERE r.rule_set_id IS NOT NULL ORDER BY c.name, c.id`)
	if err != nil {
		return nil, fmt.Errorf("read subscription rule set channels: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var ruleSetID string
		var channel SubscriptionRuleSetChannel
		if err := rows.Scan(&ruleSetID, &channel.ID, &channel.Name); err != nil {
			return nil, err
		}
		if index, listed := indices[ruleSetID]; listed {
			sets[index].Channels = append(sets[index].Channels, channel)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return sets, tx.Commit()
}

// SubscriptionRuleSetsByID returns the rule sets ids name that exist, without
// their channels.
func (s *Store) SubscriptionRuleSetsByID(ctx context.Context, ids []string) ([]SubscriptionRuleSet, error) {
	encoded, err := json.Marshal(ids)
	if err != nil {
		return nil, err
	}
	return loadSubscriptionRuleSets(ctx, s.db, `WHERE id IN (SELECT value FROM json_each(?)) ORDER BY id`, string(encoded))
}

// UpdateSubscriptionRuleSet replaces a rule set's name, icon and rules. It fails
// with a SubscriptionReferenceError when a channel that routes by the rule
// set would hold more than MaximumChannelRules rules.
func (s *Store) UpdateSubscriptionRuleSet(ctx context.Context, input UpdateSubscriptionRuleSetInput) (SubscriptionRuleSet, error) {
	if err := validateSubscriptionID(input.ID, "rule set"); err != nil {
		return SubscriptionRuleSet{}, fmt.Errorf("%w: %s", ErrSubscriptionRuleSetNotFound, input.ID)
	}
	expected, err := requiredUTC(input.ExpectedUpdatedAt, "expected rule set updated_at")
	if err != nil {
		return SubscriptionRuleSet{}, err
	}
	updated, err := nextSubscriptionUpdateTime(input.UpdatedAt, expected)
	if err != nil {
		return SubscriptionRuleSet{}, err
	}
	var stored SubscriptionRuleSet
	err = s.WithTx(ctx, func(tx *sql.Tx) error {
		current, err := getSubscriptionRuleSet(ctx, tx, input.ID)
		if err != nil {
			return err
		}
		if !current.UpdatedAt.Equal(expected) {
			return subscriptionConflict("rule set", input.ID, expected, current.UpdatedAt)
		}
		next := current
		next.Name, next.Rules, next.UpdatedAt = input.Name, input.Rules, updated
		if input.Icon != "" {
			next.Icon = input.Icon
		}
		if next, err = prepareSubscriptionRuleSet(next); err != nil {
			return err
		}
		if err := ensureRuleSetIdentityAvailable(ctx, tx, next.ID, next.Client, next.Name, next.ID); err != nil {
			return err
		}
		result, err := tx.ExecContext(ctx, `UPDATE subscription_rule_sets SET name = ?, icon = ?, rules = ?, updated_at = ?
			WHERE id = ? AND updated_at = ?`, next.Name, next.Icon, string(next.Rules), formatTime(updated), next.ID, formatTime(expected))
		if err != nil {
			return fmt.Errorf("update subscription rule set: %w", err)
		}
		if err := requireSingleSubscriptionWrite(result, "update subscription rule set"); err != nil {
			return err
		}
		var overflowing bool
		if err := tx.QueryRowContext(ctx, `SELECT EXISTS(
				SELECT 1 FROM subscription_channel_routes AS r LEFT JOIN subscription_rule_sets AS s ON s.id = r.rule_set_id
				WHERE r.enabled = 1 AND r.channel_id IN (SELECT channel_id FROM subscription_channel_routes WHERE rule_set_id = ?)
				GROUP BY r.channel_id
				HAVING sum(CASE WHEN r.rule_set_id IS NULL THEN 1 ELSE json_array_length(s.rules) END) > ?)`,
			next.ID, MaximumChannelRules).Scan(&overflowing); err != nil {
			return fmt.Errorf("count subscription channel rules: %w", err)
		}
		if overflowing {
			return &SubscriptionReferenceError{Path: "rules", Code: "too_many_rules"}
		}
		stored, err = getSubscriptionRuleSet(ctx, tx, next.ID)
		return err
	})
	return stored, err
}

// DeleteSubscriptionRuleSet deletes a rule set and the exit rules that route
// by it. Each channel that loses an exit rule takes a new updated_at, at or
// after at, so an edit of it in progress no longer saves.
func (s *Store) DeleteSubscriptionRuleSet(ctx context.Context, ruleSetID string, expectedUpdatedAt, at time.Time) error {
	if err := validateSubscriptionID(ruleSetID, "rule set"); err != nil {
		return fmt.Errorf("%w: %s", ErrSubscriptionRuleSetNotFound, ruleSetID)
	}
	expected, err := requiredUTC(expectedUpdatedAt, "expected rule set updated_at")
	if err != nil {
		return err
	}
	if at, err = requiredUTC(at, "rule set deletion time"); err != nil {
		return err
	}
	return s.WithTx(ctx, func(tx *sql.Tx) error {
		current, err := getSubscriptionRuleSet(ctx, tx, ruleSetID)
		if err != nil {
			return err
		}
		if !current.UpdatedAt.Equal(expected) {
			return subscriptionConflict("rule set", ruleSetID, expected, current.UpdatedAt)
		}
		for _, channel := range current.Channels {
			var raw string
			if err := tx.QueryRowContext(ctx, `SELECT updated_at FROM subscription_channels WHERE id = ?`, channel.ID).Scan(&raw); err != nil {
				return fmt.Errorf("read subscription channel version: %w", err)
			}
			version, err := parseTime(raw)
			if err != nil {
				return err
			}
			next := at
			if !next.After(version) {
				next = version.Add(time.Nanosecond)
			}
			if _, err := tx.ExecContext(ctx, `UPDATE subscription_channels SET updated_at = ? WHERE id = ?`, formatTime(next), channel.ID); err != nil {
				return fmt.Errorf("advance subscription channel version: %w", err)
			}
		}
		result, err := tx.ExecContext(ctx, `DELETE FROM subscription_rule_sets WHERE id = ? AND updated_at = ?`, ruleSetID, formatTime(expected))
		if err != nil {
			return fmt.Errorf("delete subscription rule set: %w", err)
		}
		return requireSingleSubscriptionWrite(result, "delete subscription rule set")
	})
}

// SetSubscriptionRuleSetOrder saves the order of client's rule sets. ids
// must name each of the client's rule sets exactly once; otherwise they
// changed since they were read and it fails with
// ErrSubscriptionRuleSetsChanged.
func (s *Store) SetSubscriptionRuleSetOrder(ctx context.Context, client SubscriptionFormat, ids []string) error {
	if !validSubscriptionFormat(client) {
		return fmt.Errorf("%w: invalid rule set client %q", ErrInvalidQuery, client)
	}
	return s.saveSubscriptionOrder(ctx, "subscription_rule_sets", ids, ErrSubscriptionRuleSetsChanged, `WHERE client = ?`, string(client))
}

func prepareSubscriptionRuleSet(set SubscriptionRuleSet) (SubscriptionRuleSet, error) {
	if err := validateSubscriptionID(set.ID, "rule set"); err != nil {
		return SubscriptionRuleSet{}, err
	}
	if err := validateSubscriptionName(set.Name); err != nil {
		return SubscriptionRuleSet{}, err
	}
	if !validSubscriptionFormat(set.Client) {
		return SubscriptionRuleSet{}, fmt.Errorf("invalid subscription rule set client %q", set.Client)
	}
	if set.Icon == "" {
		set.Icon = DefaultRuleSetIcon
	}
	if err := ValidateRuleSetIcon(set.Icon); err != nil {
		return SubscriptionRuleSet{}, err
	}
	var rules []json.RawMessage
	if len(set.Rules) > MaximumRuleSetRulesBytes || json.Unmarshal(set.Rules, &rules) != nil || rules == nil {
		return SubscriptionRuleSet{}, errors.New("subscription rule set rules must be a JSON array of at most 640 KiB")
	}
	if set.CreatedAt.IsZero() {
		set.CreatedAt = time.Now().UTC()
	}
	set.CreatedAt = set.CreatedAt.UTC()
	if set.UpdatedAt.IsZero() {
		set.UpdatedAt = set.CreatedAt
	}
	set.UpdatedAt = set.UpdatedAt.UTC()
	return set, nil
}

// ensureRuleSetIdentityAvailable fails when another rule set than excludeID
// has id, or has name among client's rule sets, ignoring case.
func ensureRuleSetIdentityAvailable(ctx context.Context, q queryRower, id string, client SubscriptionFormat, name, excludeID string) error {
	var existingID string
	err := q.QueryRowContext(ctx, `SELECT id FROM subscription_rule_sets
		WHERE (id = ? OR (client = ? AND name = ?)) AND id <> ?
		ORDER BY id LIMIT 1`, id, string(client), name, excludeID).Scan(&existingID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("check subscription rule set identity: %w", err)
	}
	return fmt.Errorf("%w: id or name belongs to %s", ErrSubscriptionRuleSetExists, existingID)
}

// getSubscriptionRuleSet reads a rule set with the channels that route by it.
func getSubscriptionRuleSet(ctx context.Context, tx *sql.Tx, id string) (SubscriptionRuleSet, error) {
	sets, err := loadSubscriptionRuleSets(ctx, tx, `WHERE id = ?`, id)
	if err != nil {
		return SubscriptionRuleSet{}, err
	}
	if len(sets) == 0 {
		return SubscriptionRuleSet{}, fmt.Errorf("%w: %s", ErrSubscriptionRuleSetNotFound, id)
	}
	set := sets[0]
	rows, err := tx.QueryContext(ctx, `SELECT DISTINCT c.id, c.name
		FROM subscription_channel_routes AS r JOIN subscription_channels AS c ON c.id = r.channel_id
		WHERE r.rule_set_id = ? ORDER BY c.name, c.id`, id)
	if err != nil {
		return SubscriptionRuleSet{}, fmt.Errorf("read subscription rule set channels: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var channel SubscriptionRuleSetChannel
		if err := rows.Scan(&channel.ID, &channel.Name); err != nil {
			return SubscriptionRuleSet{}, err
		}
		set.Channels = append(set.Channels, channel)
	}
	return set, rows.Err()
}

// loadSubscriptionRuleSets reads the rule sets a filter and order clause
// selects, with no channels.
func loadSubscriptionRuleSets(ctx context.Context, q queryer, clause string, args ...any) ([]SubscriptionRuleSet, error) {
	rows, err := q.QueryContext(ctx, `SELECT `+subscriptionRuleSetColumns+` FROM subscription_rule_sets `+clause, args...)
	if err != nil {
		return nil, fmt.Errorf("read subscription rule sets: %w", err)
	}
	defer rows.Close()
	sets := []SubscriptionRuleSet{}
	for rows.Next() {
		var set SubscriptionRuleSet
		var rules, createdAt, updatedAt string
		if err := rows.Scan(&set.ID, &set.Client, &set.Name, &set.Icon, &rules, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		set.Rules, set.Channels = json.RawMessage(rules), []SubscriptionRuleSetChannel{}
		if set.CreatedAt, err = parseTime(createdAt); err != nil {
			return nil, err
		}
		if set.UpdatedAt, err = parseTime(updatedAt); err != nil {
			return nil, err
		}
		sets = append(sets, set)
	}
	return sets, rows.Err()
}
