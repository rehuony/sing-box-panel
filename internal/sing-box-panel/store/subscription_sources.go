// SPDX-License-Identifier: GPL-3.0-or-later

package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

type SubscriptionSourceKind string

const (
	SubscriptionSourceURL  SubscriptionSourceKind = "url"
	SubscriptionSourceFile SubscriptionSourceKind = "file"
)

const (
	// MaximumSourceContentBytes bounds one source's stored content.
	MaximumSourceContentBytes = 4 << 20
	maximumSourceURLBytes     = 4096
	maximumSourceRules        = 256
	maximumSourceRuleBytes    = 512
	maximumSourceErrorBytes   = 128
)

// SubscriptionSourceFilter holds a source's name rules. The subscription
// service interprets them; the store keeps them as given.
type SubscriptionSourceFilter struct {
	Include []string
	Exclude []string
}

// SubscriptionSource is a source's settings and the state of its content.
// The content itself is read with SubscriptionSourceContent.
type SubscriptionSource struct {
	ID   string
	Name string
	Kind SubscriptionSourceKind
	// URL and RefreshMinutes belong to URL sources only.
	URL            string
	RefreshMinutes int
	Format         string
	Filter         SubscriptionSourceFilter
	ContentSHA256  string
	ContentFormat  string
	ContentAt      time.Time
	LastAttemptAt  *time.Time
	LastError      string
	NextRefreshAt  *time.Time
	// NodeCount is the number of pool nodes the source contributes.
	NodeCount int
	CreatedAt time.Time
	UpdatedAt time.Time
}

// SubscriptionSourceContent is a source's stored content as it last parsed.
type SubscriptionSourceContent struct {
	Content []byte
	Format  string
}

type SubscriptionSourceListFilter struct {
	Cursor *CreatedAtCursor
	Limit  int
}

type SubscriptionSourcePage struct {
	Items []SubscriptionSource
	Next  *CreatedAtCursor
}

// NewSubscriptionSource is a source with the content it was created from.
type NewSubscriptionSource struct {
	Source  SubscriptionSource
	Content []byte
}

// UpdateSubscriptionSourceInput replaces a source's settings when its
// updated_at is still ExpectedUpdatedAt.
type UpdateSubscriptionSourceInput struct {
	ID                string
	Name              string
	URL               string
	RefreshMinutes    int
	Format            string
	Filter            SubscriptionSourceFilter
	NextRefreshAt     *time.Time
	ExpectedUpdatedAt time.Time
	UpdatedAt         time.Time
}

// SubscriptionSourceContentUpdate stores content that parsed: a refresh or an
// upload. Format, when set, also replaces the source's format setting and
// advances its updated_at to UpdatedAt.
type SubscriptionSourceContentUpdate struct {
	ID            string
	Content       []byte
	ContentFormat string
	At            time.Time
	NextRefreshAt *time.Time
	Format        string
	UpdatedAt     time.Time
}

const subscriptionSourceColumns = `
    s.id, s.name, s.kind, s.url, s.refresh_minutes, s.format, s.include_rules, s.exclude_rules,
    s.content_sha256, s.content_format, s.content_at, s.last_attempt_at, s.last_error,
    s.next_refresh_at, s.created_at, s.updated_at,
    (SELECT count(*) FROM subscription_nodes AS n WHERE n.source_id = s.id)`

func (s *Store) CreateSubscriptionSource(ctx context.Context, input NewSubscriptionSource) (SubscriptionSource, error) {
	source, err := prepareSubscriptionSource(input.Source)
	if err != nil {
		return SubscriptionSource{}, err
	}
	if err := validateSourceContent(input.Content, source.ContentFormat); err != nil {
		return SubscriptionSource{}, err
	}
	if source.ContentAt.IsZero() {
		return SubscriptionSource{}, errors.New("subscription source content time is required")
	}
	include, exclude, err := encodeSourceFilter(source.Filter)
	if err != nil {
		return SubscriptionSource{}, err
	}
	var stored SubscriptionSource
	err = s.WithTx(ctx, func(tx *sql.Tx) error {
		if err := ensureSourceIdentityAvailable(ctx, tx, source.ID, source.Name, ""); err != nil {
			return err
		}
		if err := ensureSubscriptionSourceLimits(ctx, tx, "", int64(len(input.Content))); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO subscription_sources(
				id, name, kind, url, refresh_minutes, format, include_rules, exclude_rules,
				content, content_sha256, content_format, content_at, last_attempt_at,
				next_refresh_at, created_at, updated_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			source.ID, source.Name, string(source.Kind), nullableSourceURL(source), nullableRefreshMinutes(source),
			source.Format, include, exclude, input.Content, contentDigest(input.Content), source.ContentFormat,
			formatTime(source.ContentAt), nullableTime(source.LastAttemptAt), nullableTime(source.NextRefreshAt),
			formatTime(source.CreatedAt), formatTime(source.UpdatedAt),
		); err != nil {
			return fmt.Errorf("insert subscription source: %w", err)
		}
		stored, err = getSubscriptionSource(ctx, tx, source.ID)
		return err
	})
	return stored, err
}

func (s *Store) GetSubscriptionSource(ctx context.Context, sourceID string) (SubscriptionSource, error) {
	if err := validateSubscriptionID(sourceID, "source"); err != nil {
		return SubscriptionSource{}, fmt.Errorf("%w: %s", ErrSubscriptionSourceNotFound, sourceID)
	}
	return getSubscriptionSource(ctx, s.db, sourceID)
}

// SubscriptionSourceContent returns the source's stored content.
func (s *Store) SubscriptionSourceContent(ctx context.Context, sourceID string) (SubscriptionSourceContent, error) {
	if err := validateSubscriptionID(sourceID, "source"); err != nil {
		return SubscriptionSourceContent{}, fmt.Errorf("%w: %s", ErrSubscriptionSourceNotFound, sourceID)
	}
	var value SubscriptionSourceContent
	err := s.db.QueryRowContext(ctx, `SELECT content, content_format FROM subscription_sources WHERE id = ?`, sourceID).
		Scan(&value.Content, &value.Format)
	if errors.Is(err, sql.ErrNoRows) {
		return SubscriptionSourceContent{}, fmt.Errorf("%w: %s", ErrSubscriptionSourceNotFound, sourceID)
	}
	if err != nil {
		return SubscriptionSourceContent{}, fmt.Errorf("read subscription source content: %w", err)
	}
	return value, nil
}

func (s *Store) ListSubscriptionSources(ctx context.Context, filter SubscriptionSourceListFilter) (SubscriptionSourcePage, error) {
	limit, err := normalizePageLimit(filter.Limit)
	if err != nil {
		return SubscriptionSourcePage{}, err
	}
	if err := validateCreatedAtCursor(filter.Cursor); err != nil {
		return SubscriptionSourcePage{}, err
	}
	query := `SELECT ` + subscriptionSourceColumns + ` FROM subscription_sources AS s`
	args := make([]any, 0, 4)
	if filter.Cursor != nil {
		query += ` WHERE (s.created_at < ? OR (s.created_at = ? AND s.id < ?))`
		cursorTime := formatTime(filter.Cursor.CreatedAt)
		args = append(args, cursorTime, cursorTime, filter.Cursor.ID)
	}
	query += ` ORDER BY s.created_at DESC, s.id DESC LIMIT ?`
	args = append(args, limit+1)
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return SubscriptionSourcePage{}, fmt.Errorf("list subscription sources: %w", err)
	}
	defer rows.Close()
	items := make([]SubscriptionSource, 0, limit+1)
	for rows.Next() {
		source, err := scanSubscriptionSource(rows)
		if err != nil {
			return SubscriptionSourcePage{}, fmt.Errorf("scan subscription source: %w", err)
		}
		items = append(items, source)
	}
	if err := rows.Err(); err != nil {
		return SubscriptionSourcePage{}, fmt.Errorf("iterate subscription sources: %w", err)
	}
	page := SubscriptionSourcePage{Items: items}
	if len(items) > limit {
		page.Items = items[:limit]
		last := page.Items[len(page.Items)-1]
		page.Next = &CreatedAtCursor{CreatedAt: last.CreatedAt, ID: last.ID}
	}
	return page, nil
}

// UpdateSubscriptionSource replaces a source's settings. The kind and the
// content stay as they are.
func (s *Store) UpdateSubscriptionSource(ctx context.Context, input UpdateSubscriptionSourceInput) (SubscriptionSource, error) {
	if err := validateSubscriptionID(input.ID, "source"); err != nil {
		return SubscriptionSource{}, fmt.Errorf("%w: %s", ErrSubscriptionSourceNotFound, input.ID)
	}
	expected, err := requiredUTC(input.ExpectedUpdatedAt, "expected source updated_at")
	if err != nil {
		return SubscriptionSource{}, err
	}
	updated, err := nextSubscriptionUpdateTime(input.UpdatedAt, expected)
	if err != nil {
		return SubscriptionSource{}, err
	}
	var stored SubscriptionSource
	err = s.WithTx(ctx, func(tx *sql.Tx) error {
		current, err := getSubscriptionSource(ctx, tx, input.ID)
		if err != nil {
			return err
		}
		if !current.UpdatedAt.Equal(expected) {
			return subscriptionConflict("source", input.ID, expected, current.UpdatedAt)
		}
		next := current
		next.Name, next.URL, next.RefreshMinutes = input.Name, input.URL, input.RefreshMinutes
		next.Format, next.Filter, next.NextRefreshAt = input.Format, input.Filter, input.NextRefreshAt
		next.UpdatedAt = updated
		if next, err = prepareSubscriptionSource(next); err != nil {
			return err
		}
		include, exclude, err := encodeSourceFilter(next.Filter)
		if err != nil {
			return err
		}
		if err := ensureSourceIdentityAvailable(ctx, tx, next.ID, next.Name, next.ID); err != nil {
			return err
		}
		result, err := tx.ExecContext(ctx, `UPDATE subscription_sources
			SET name = ?, url = ?, refresh_minutes = ?, format = ?, include_rules = ?, exclude_rules = ?,
			    next_refresh_at = ?, updated_at = ?
			WHERE id = ? AND updated_at = ?`,
			next.Name, nullableSourceURL(next), nullableRefreshMinutes(next), next.Format, include, exclude,
			nullableTime(next.NextRefreshAt), formatTime(updated), next.ID, formatTime(expected))
		if err != nil {
			return fmt.Errorf("update subscription source: %w", err)
		}
		if err := requireSingleSubscriptionWrite(result, "update subscription source"); err != nil {
			return err
		}
		stored, err = getSubscriptionSource(ctx, tx, input.ID)
		return err
	})
	return stored, err
}

// ReplaceSubscriptionSourceContent stores content that parsed and clears the
// source's last error.
func (s *Store) ReplaceSubscriptionSourceContent(ctx context.Context, input SubscriptionSourceContentUpdate) (SubscriptionSource, error) {
	if err := validateSubscriptionID(input.ID, "source"); err != nil {
		return SubscriptionSource{}, fmt.Errorf("%w: %s", ErrSubscriptionSourceNotFound, input.ID)
	}
	if err := validateSourceContent(input.Content, input.ContentFormat); err != nil {
		return SubscriptionSource{}, err
	}
	at, err := requiredUTC(input.At, "subscription source content time")
	if err != nil {
		return SubscriptionSource{}, err
	}
	var stored SubscriptionSource
	err = s.WithTx(ctx, func(tx *sql.Tx) error {
		current, err := getSubscriptionSource(ctx, tx, input.ID)
		if err != nil {
			return err
		}
		if current.Kind == SubscriptionSourceFile && input.NextRefreshAt != nil {
			return errors.New("a file subscription source is not refreshed")
		}
		if err := ensureSubscriptionSourceLimits(ctx, tx, current.ID, int64(len(input.Content))); err != nil {
			return err
		}
		format, updated := current.Format, current.UpdatedAt
		if input.Format != "" && input.Format != current.Format {
			if !validSourceFormatChoice(input.Format) {
				return fmt.Errorf("invalid subscription source format %q", input.Format)
			}
			format = input.Format
			if updated, err = nextSubscriptionUpdateTime(input.UpdatedAt, current.UpdatedAt); err != nil {
				return err
			}
		}
		result, err := tx.ExecContext(ctx, `UPDATE subscription_sources
			SET content = ?, content_sha256 = ?, content_format = ?, content_at = ?, last_attempt_at = ?,
			    last_error = NULL, next_refresh_at = ?, format = ?, updated_at = ?
			WHERE id = ?`,
			input.Content, contentDigest(input.Content), input.ContentFormat, formatTime(at), formatTime(at),
			nullableTime(input.NextRefreshAt), format, formatTime(updated), current.ID)
		if err != nil {
			return fmt.Errorf("replace subscription source content: %w", err)
		}
		if err := requireSingleSubscriptionWrite(result, "replace subscription source content"); err != nil {
			return err
		}
		stored, err = getSubscriptionSource(ctx, tx, current.ID)
		return err
	})
	return stored, err
}

// RecordSubscriptionSourceFailure records a failed fetch or parse of new
// content, which keeps the current content, and schedules the next attempt.
func (s *Store) RecordSubscriptionSourceFailure(ctx context.Context, sourceID, code string, at time.Time, next *time.Time) (SubscriptionSource, error) {
	if err := validateSourceError(code); err != nil {
		return SubscriptionSource{}, err
	}
	var stored SubscriptionSource
	err := s.WithTx(ctx, func(tx *sql.Tx) error {
		result, err := tx.ExecContext(ctx, `UPDATE subscription_sources
			SET last_error = ?, last_attempt_at = ?, next_refresh_at = CASE WHEN kind = 'url' THEN ? END
			WHERE id = ?`, code, formatTime(at), nullableTime(next), sourceID)
		if err != nil {
			return fmt.Errorf("record subscription source failure: %w", err)
		}
		if rows, err := result.RowsAffected(); err != nil {
			return err
		} else if rows == 0 {
			return fmt.Errorf("%w: %s", ErrSubscriptionSourceNotFound, sourceID)
		}
		stored, err = getSubscriptionSource(ctx, tx, sourceID)
		return err
	})
	return stored, err
}

func (s *Store) DeleteSubscriptionSource(ctx context.Context, sourceID string, expectedUpdatedAt time.Time) error {
	if err := validateSubscriptionID(sourceID, "source"); err != nil {
		return fmt.Errorf("%w: %s", ErrSubscriptionSourceNotFound, sourceID)
	}
	expected, err := requiredUTC(expectedUpdatedAt, "expected source updated_at")
	if err != nil {
		return err
	}
	return s.WithTx(ctx, func(tx *sql.Tx) error {
		current, err := getSubscriptionSource(ctx, tx, sourceID)
		if err != nil {
			return err
		}
		if !current.UpdatedAt.Equal(expected) {
			return subscriptionConflict("source", sourceID, expected, current.UpdatedAt)
		}
		result, err := tx.ExecContext(ctx, `DELETE FROM subscription_sources WHERE id = ? AND updated_at = ?`, sourceID, formatTime(expected))
		if err != nil {
			return fmt.Errorf("delete subscription source: %w", err)
		}
		return requireSingleSubscriptionWrite(result, "delete subscription source")
	})
}

// DueSubscriptionSources returns the URL sources whose next refresh is due,
// earliest first.
func (s *Store) DueSubscriptionSources(ctx context.Context, now time.Time) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id FROM subscription_sources
		WHERE kind = 'url' AND next_refresh_at IS NOT NULL AND next_refresh_at <= ?
		ORDER BY next_refresh_at, id LIMIT 200`, formatTime(now))
	if err != nil {
		return nil, fmt.Errorf("read due subscription sources: %w", err)
	}
	defer rows.Close()
	ids := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func prepareSubscriptionSource(source SubscriptionSource) (SubscriptionSource, error) {
	if err := validateSubscriptionID(source.ID, "source"); err != nil {
		return SubscriptionSource{}, err
	}
	if err := validateSubscriptionName(source.Name); err != nil {
		return SubscriptionSource{}, err
	}
	if !validSubscriptionSourceKind(source.Kind) {
		return SubscriptionSource{}, fmt.Errorf("invalid subscription source kind %q", source.Kind)
	}
	if !validSourceFormatChoice(source.Format) {
		return SubscriptionSource{}, fmt.Errorf("invalid subscription source format %q", source.Format)
	}
	if source.Kind == SubscriptionSourceURL {
		if source.URL == "" || len(source.URL) > maximumSourceURLBytes {
			return SubscriptionSource{}, errors.New("a URL subscription source needs a URL of at most 4096 bytes")
		}
		if source.RefreshMinutes != 0 && (source.RefreshMinutes < 15 || source.RefreshMinutes > 10080) {
			return SubscriptionSource{}, errors.New("subscription source refresh interval must be 0 or 15 to 10080 minutes")
		}
	} else if source.URL != "" || source.RefreshMinutes != 0 || source.NextRefreshAt != nil {
		return SubscriptionSource{}, errors.New("a file subscription source has no URL or refresh interval")
	}
	if err := validateSourceRules(source.Filter.Include); err != nil {
		return SubscriptionSource{}, err
	}
	if err := validateSourceRules(source.Filter.Exclude); err != nil {
		return SubscriptionSource{}, err
	}
	if source.CreatedAt.IsZero() {
		source.CreatedAt = time.Now().UTC()
	}
	source.CreatedAt = source.CreatedAt.UTC()
	if source.UpdatedAt.IsZero() {
		source.UpdatedAt = source.CreatedAt
	}
	source.UpdatedAt = source.UpdatedAt.UTC()
	return source, nil
}

func validSourceFormatChoice(value string) bool {
	return value == "auto" || validSourceContentFormat(value)
}

func validSourceContentFormat(value string) bool {
	return value == "sing-box" || value == "mihomo" || value == "share-links"
}

func validateSourceContent(content []byte, format string) error {
	if len(content) == 0 || len(content) > MaximumSourceContentBytes {
		return errors.New("subscription source content must be 1 byte to 4 MiB")
	}
	if !validSourceContentFormat(format) {
		return fmt.Errorf("invalid subscription source content format %q", format)
	}
	return nil
}

func validateSourceRules(rules []string) error {
	if len(rules) > maximumSourceRules {
		return errors.New("a subscription source filter has at most 256 rules per list")
	}
	for _, rule := range rules {
		if rule == "" || len(rule) > maximumSourceRuleBytes {
			return errors.New("a subscription source filter rule must be 1 to 512 bytes")
		}
	}
	return nil
}

func validateSourceError(code string) error {
	if code == "" || len(code) > maximumSourceErrorBytes {
		return errors.New("a subscription source error code must be 1 to 128 bytes")
	}
	return nil
}

func encodeSourceFilter(filter SubscriptionSourceFilter) (string, string, error) {
	encode := func(rules []string) (string, error) {
		if rules == nil {
			rules = []string{}
		}
		encoded, err := json.Marshal(rules)
		return string(encoded), err
	}
	include, err := encode(filter.Include)
	if err != nil {
		return "", "", err
	}
	exclude, err := encode(filter.Exclude)
	return include, exclude, err
}

func contentDigest(content []byte) string {
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:])
}

func nullableSourceURL(source SubscriptionSource) any {
	if source.Kind != SubscriptionSourceURL {
		return nil
	}
	return source.URL
}

func nullableRefreshMinutes(source SubscriptionSource) any {
	if source.Kind != SubscriptionSourceURL {
		return nil
	}
	return source.RefreshMinutes
}

// ensureSubscriptionSourceLimits bounds the number of sources and the total
// size of their contents, counting candidateBytes in place of excludeID's.
func ensureSubscriptionSourceLimits(ctx context.Context, q queryRower, excludeID string, candidateBytes int64) error {
	var count, size int64
	if err := q.QueryRowContext(ctx,
		`SELECT count(*), coalesce(sum(length(content)), 0) FROM subscription_sources WHERE id <> ?`, excludeID,
	).Scan(&count, &size); err != nil {
		return fmt.Errorf("measure subscription sources: %w", err)
	}
	if excludeID == "" && count >= MaximumSubscriptionSources {
		return fmt.Errorf("%w: source count exceeds %d", ErrSubscriptionLimitExceeded, MaximumSubscriptionSources)
	}
	if size > MaximumSubscriptionInputBytes-candidateBytes {
		return fmt.Errorf("%w: source contents exceed %d bytes", ErrSubscriptionLimitExceeded, MaximumSubscriptionInputBytes)
	}
	return nil
}

func getSubscriptionSource(ctx context.Context, q queryRower, id string) (SubscriptionSource, error) {
	source, err := scanSubscriptionSource(q.QueryRowContext(ctx,
		`SELECT `+subscriptionSourceColumns+` FROM subscription_sources AS s WHERE s.id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return SubscriptionSource{}, fmt.Errorf("%w: %s", ErrSubscriptionSourceNotFound, id)
	}
	if err != nil {
		return SubscriptionSource{}, fmt.Errorf("get subscription source: %w", err)
	}
	return source, nil
}

func scanSubscriptionSource(row rowScanner) (SubscriptionSource, error) {
	var source SubscriptionSource
	var url, lastAttempt, lastError, nextRefresh sql.NullString
	var refresh sql.NullInt64
	var include, exclude, contentAt, createdAt, updatedAt string
	if err := row.Scan(&source.ID, &source.Name, &source.Kind, &url, &refresh, &source.Format, &include, &exclude,
		&source.ContentSHA256, &source.ContentFormat, &contentAt, &lastAttempt, &lastError, &nextRefresh,
		&createdAt, &updatedAt, &source.NodeCount); err != nil {
		return SubscriptionSource{}, err
	}
	source.URL, source.RefreshMinutes, source.LastError = url.String, int(refresh.Int64), lastError.String
	if err := json.Unmarshal([]byte(include), &source.Filter.Include); err != nil {
		return SubscriptionSource{}, fmt.Errorf("decode include rules: %w", err)
	}
	if err := json.Unmarshal([]byte(exclude), &source.Filter.Exclude); err != nil {
		return SubscriptionSource{}, fmt.Errorf("decode exclude rules: %w", err)
	}
	var err error
	if source.ContentAt, err = parseTime(contentAt); err != nil {
		return SubscriptionSource{}, err
	}
	if source.LastAttemptAt, err = parseNullableTime(lastAttempt); err != nil {
		return SubscriptionSource{}, err
	}
	if source.NextRefreshAt, err = parseNullableTime(nextRefresh); err != nil {
		return SubscriptionSource{}, err
	}
	if source.CreatedAt, err = parseTime(createdAt); err != nil {
		return SubscriptionSource{}, err
	}
	if source.UpdatedAt, err = parseTime(updatedAt); err != nil {
		return SubscriptionSource{}, err
	}
	return source, nil
}

func parseNullableTime(value sql.NullString) (*time.Time, error) {
	if !value.Valid {
		return nil, nil
	}
	parsed, err := parseTime(value.String)
	if err != nil {
		return nil, err
	}
	return &parsed, nil
}
