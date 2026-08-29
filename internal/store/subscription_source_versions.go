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
	"time"
)

var (
	ErrSubscriptionSourceVersionNotFound = errors.New("subscription source version not found")
	ErrSubscriptionSourceVersionExists   = errors.New("subscription source version already exists")
)

type SubscriptionSourceVersion struct {
	ID              string
	SourceID        string
	Format          string
	RawBody         []byte
	NormalizedNodes json.RawMessage
	Diagnostics     json.RawMessage
	SHA256          string
	FetchedAt       time.Time
	CreatedAt       time.Time
}

type SaveSubscriptionSourceVersionInput struct {
	Version                 SubscriptionSourceVersion
	ExpectedSourceUpdatedAt time.Time
	UpdatedAt               time.Time
	RefreshTask             *EnqueueTaskInput
}

type SubscriptionSourceVersionListFilter struct {
	SourceID string
	Cursor   *CreatedAtCursor
	Limit    int
}

type SubscriptionSourceVersionPage struct {
	Items []SubscriptionSourceVersion
	Next  *CreatedAtCursor
}

type SubscriptionSourceVersionSave struct {
	Source  SubscriptionSource
	Version SubscriptionSourceVersion
}

const subscriptionSourceVersionColumns = `
    id, source_id, format, raw_body, normalized_nodes_json, diagnostics_json,
    sha256, fetched_at, created_at`

func (s *Store) SaveSubscriptionSourceVersion(
	ctx context.Context,
	input SaveSubscriptionSourceVersionInput,
) (SubscriptionSourceVersionSave, error) {
	version, err := prepareSubscriptionSourceVersion(input.Version)
	if err != nil {
		return SubscriptionSourceVersionSave{}, err
	}
	expected, err := requiredUTC(input.ExpectedSourceUpdatedAt, "expected source updated_at")
	if err != nil {
		return SubscriptionSourceVersionSave{}, err
	}
	updated, err := nextSubscriptionUpdateTime(input.UpdatedAt, expected)
	if err != nil {
		return SubscriptionSourceVersionSave{}, err
	}
	refreshTask, err := prepareSubscriptionRefreshTask(input.RefreshTask)
	if err != nil {
		return SubscriptionSourceVersionSave{}, err
	}
	var saved SubscriptionSourceVersionSave
	err = s.WithTx(ctx, func(tx *sql.Tx) error {
		source, err := getSubscriptionSource(ctx, tx, version.SourceID)
		if err != nil {
			return err
		}
		if !source.UpdatedAt.Equal(expected) {
			return subscriptionConflict("source", source.ID, expected, source.UpdatedAt)
		}
		existing, existingErr := getSubscriptionSourceVersionByDigest(ctx, tx, source.ID, version.SHA256)
		switch {
		case existingErr == nil:
			version = existing
		case errors.Is(existingErr, ErrSubscriptionSourceVersionNotFound):
			if _, err := tx.ExecContext(ctx, `INSERT INTO subscription_source_versions(
                    id, source_id, format, raw_body, normalized_nodes_json, diagnostics_json,
                    sha256, fetched_at, created_at
                ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
				version.ID, version.SourceID, version.Format, version.RawBody,
				string(version.NormalizedNodes), string(version.Diagnostics), version.SHA256,
				formatTaskTime(version.FetchedAt), formatTaskTime(version.CreatedAt)); err != nil {
				return fmt.Errorf("insert subscription source version: %w", err)
			}
		default:
			return existingErr
		}
		write, err := tx.ExecContext(ctx, `UPDATE subscription_sources
            SET current_version_id = ?, updated_at = ?
            WHERE id = ? AND updated_at = ?`,
			version.ID, formatTaskTime(updated), source.ID, formatTaskTime(expected))
		if err != nil {
			return fmt.Errorf("activate subscription source version: %w", err)
		}
		if err := requireSingleSubscriptionWrite(write, "activate subscription source version"); err != nil {
			return err
		}
		saved.Source, err = getSubscriptionSource(ctx, tx, source.ID)
		if err != nil {
			return err
		}
		saved.Version = cloneSubscriptionSourceVersion(version)
		return enqueueSubscriptionRefreshTaskTx(ctx, tx, refreshTask)
	})
	return saved, err
}

func (s *Store) GetSubscriptionSourceVersion(
	ctx context.Context,
	sourceID string,
	versionID string,
) (SubscriptionSourceVersion, error) {
	if err := validateSubscriptionID(sourceID, "source"); err != nil {
		return SubscriptionSourceVersion{}, err
	}
	if err := validateSubscriptionID(versionID, "version"); err != nil {
		return SubscriptionSourceVersion{}, err
	}
	return getSubscriptionSourceVersion(ctx, s.db, sourceID, versionID)
}

func (s *Store) ListSubscriptionSourceVersions(
	ctx context.Context,
	filter SubscriptionSourceVersionListFilter,
) (SubscriptionSourceVersionPage, error) {
	if err := validateSubscriptionID(filter.SourceID, "source"); err != nil {
		return SubscriptionSourceVersionPage{}, err
	}
	limit, err := normalizePageLimit(filter.Limit)
	if err != nil {
		return SubscriptionSourceVersionPage{}, err
	}
	if err := validateCreatedAtCursor(filter.Cursor); err != nil {
		return SubscriptionSourceVersionPage{}, err
	}
	if _, err := s.GetSubscriptionSource(ctx, filter.SourceID); err != nil {
		return SubscriptionSourceVersionPage{}, err
	}
	query := `SELECT ` + subscriptionSourceVersionColumns + ` FROM subscription_source_versions WHERE source_id = ?`
	args := []any{filter.SourceID}
	if filter.Cursor != nil {
		query += ` AND (created_at < ? OR (created_at = ? AND id < ?))`
		cursorTime := formatTaskTime(filter.Cursor.CreatedAt)
		args = append(args, cursorTime, cursorTime, filter.Cursor.ID)
	}
	query += ` ORDER BY created_at DESC, id DESC LIMIT ?`
	args = append(args, limit+1)
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return SubscriptionSourceVersionPage{}, fmt.Errorf("list subscription source versions: %w", err)
	}
	defer rows.Close()
	items := make([]SubscriptionSourceVersion, 0, limit+1)
	for rows.Next() {
		version, scanErr := scanSubscriptionSourceVersion(rows)
		if scanErr != nil {
			return SubscriptionSourceVersionPage{}, scanErr
		}
		items = append(items, version)
	}
	if err := rows.Err(); err != nil {
		return SubscriptionSourceVersionPage{}, err
	}
	page := SubscriptionSourceVersionPage{Items: items}
	if len(items) > limit {
		page.Items = items[:limit]
		last := page.Items[len(page.Items)-1]
		page.Next = &CreatedAtCursor{CreatedAt: last.CreatedAt, ID: last.ID}
	}
	return page, nil
}

func (s *Store) ActivateSubscriptionSourceVersion(
	ctx context.Context,
	sourceID string,
	versionID string,
	expectedUpdatedAt time.Time,
	updatedAt time.Time,
) (SubscriptionSource, error) {
	return s.ActivateSubscriptionSourceVersionAndTask(ctx, sourceID, versionID, expectedUpdatedAt, updatedAt, nil)
}

// ActivateSubscriptionSourceVersionAndTask changes the current immutable
// version and schedules the next refresh in the same transaction.
func (s *Store) ActivateSubscriptionSourceVersionAndTask(
	ctx context.Context,
	sourceID string,
	versionID string,
	expectedUpdatedAt time.Time,
	updatedAt time.Time,
	refreshTask *EnqueueTaskInput,
) (SubscriptionSource, error) {
	if err := validateSubscriptionID(sourceID, "source"); err != nil {
		return SubscriptionSource{}, err
	}
	if err := validateSubscriptionID(versionID, "version"); err != nil {
		return SubscriptionSource{}, err
	}
	expected, err := requiredUTC(expectedUpdatedAt, "expected source updated_at")
	if err != nil {
		return SubscriptionSource{}, err
	}
	updated, err := nextSubscriptionUpdateTime(updatedAt, expected)
	if err != nil {
		return SubscriptionSource{}, err
	}
	preparedTask, err := prepareSubscriptionRefreshTask(refreshTask)
	if err != nil {
		return SubscriptionSource{}, err
	}
	var stored SubscriptionSource
	err = s.WithTx(ctx, func(tx *sql.Tx) error {
		source, err := getSubscriptionSource(ctx, tx, sourceID)
		if err != nil {
			return err
		}
		if !source.UpdatedAt.Equal(expected) {
			return subscriptionConflict("source", sourceID, expected, source.UpdatedAt)
		}
		if _, err := getSubscriptionSourceVersion(ctx, tx, sourceID, versionID); err != nil {
			return err
		}
		write, err := tx.ExecContext(ctx, `UPDATE subscription_sources SET current_version_id = ?, updated_at = ?
            WHERE id = ? AND updated_at = ?`, versionID, formatTaskTime(updated), sourceID, formatTaskTime(expected))
		if err != nil {
			return err
		}
		if err := requireSingleSubscriptionWrite(write, "restore subscription source version"); err != nil {
			return err
		}
		stored, err = getSubscriptionSource(ctx, tx, sourceID)
		if err != nil {
			return err
		}
		return enqueueSubscriptionRefreshTaskTx(ctx, tx, preparedTask)
	})
	return stored, err
}

func prepareSubscriptionSourceVersion(version SubscriptionSourceVersion) (SubscriptionSourceVersion, error) {
	if err := validateSubscriptionID(version.ID, "version"); err != nil {
		return SubscriptionSourceVersion{}, err
	}
	if err := validateSubscriptionID(version.SourceID, "source"); err != nil {
		return SubscriptionSourceVersion{}, err
	}
	switch version.Format {
	case "sing-box-json", "mihomo-yaml", "uri-list":
	default:
		return SubscriptionSourceVersion{}, errors.New("invalid subscription source version format")
	}
	if len(version.RawBody) == 0 || len(version.RawBody) > maximumSourceSnapshotBytes {
		return SubscriptionSourceVersion{}, errors.New("subscription source version raw body is invalid")
	}
	nodes, err := canonicalJSONArrayWithLimit(version.NormalizedNodes, `[]`, maximumSourceSnapshotBytes)
	if err != nil {
		return SubscriptionSourceVersion{}, fmt.Errorf("subscription source version nodes: %w", err)
	}
	diagnostics, err := canonicalJSONArrayWithLimit(version.Diagnostics, `[]`, maximumSubscriptionConfigBytes)
	if err != nil {
		return SubscriptionSourceVersion{}, fmt.Errorf("subscription source version diagnostics: %w", err)
	}
	digest := sha256.Sum256(version.RawBody)
	actualDigest := hex.EncodeToString(digest[:])
	if version.SHA256 != "" && version.SHA256 != actualDigest {
		return SubscriptionSourceVersion{}, errors.New("subscription source version digest mismatch")
	}
	if version.FetchedAt.IsZero() || version.CreatedAt.IsZero() {
		return SubscriptionSourceVersion{}, errors.New("subscription source version timestamps are required")
	}
	version.RawBody = bytes.Clone(version.RawBody)
	version.NormalizedNodes = nodes
	version.Diagnostics = diagnostics
	version.SHA256 = actualDigest
	version.FetchedAt = version.FetchedAt.UTC()
	version.CreatedAt = version.CreatedAt.UTC()
	return version, nil
}

func getSubscriptionSourceVersion(
	ctx context.Context,
	q queryRower,
	sourceID string,
	versionID string,
) (SubscriptionSourceVersion, error) {
	version, err := scanSubscriptionSourceVersion(q.QueryRowContext(ctx,
		`SELECT `+subscriptionSourceVersionColumns+` FROM subscription_source_versions WHERE source_id = ? AND id = ?`,
		sourceID, versionID))
	if errors.Is(err, sql.ErrNoRows) {
		return SubscriptionSourceVersion{}, fmt.Errorf("%w: %s", ErrSubscriptionSourceVersionNotFound, versionID)
	}
	return version, err
}

func getSubscriptionSourceVersionByDigest(
	ctx context.Context,
	q queryRower,
	sourceID string,
	digest string,
) (SubscriptionSourceVersion, error) {
	version, err := scanSubscriptionSourceVersion(q.QueryRowContext(ctx,
		`SELECT `+subscriptionSourceVersionColumns+` FROM subscription_source_versions WHERE source_id = ? AND sha256 = ?`,
		sourceID, digest))
	if errors.Is(err, sql.ErrNoRows) {
		return SubscriptionSourceVersion{}, ErrSubscriptionSourceVersionNotFound
	}
	return version, err
}

func scanSubscriptionSourceVersion(row taskScanner) (SubscriptionSourceVersion, error) {
	var version SubscriptionSourceVersion
	var nodes, diagnostics, fetchedAt, createdAt string
	if err := row.Scan(
		&version.ID, &version.SourceID, &version.Format, &version.RawBody,
		&nodes, &diagnostics, &version.SHA256, &fetchedAt, &createdAt,
	); err != nil {
		return SubscriptionSourceVersion{}, err
	}
	version.RawBody = bytes.Clone(version.RawBody)
	version.NormalizedNodes = json.RawMessage(nodes)
	version.Diagnostics = json.RawMessage(diagnostics)
	var err error
	version.FetchedAt, err = parseTaskTime(fetchedAt)
	if err != nil {
		return SubscriptionSourceVersion{}, err
	}
	version.CreatedAt, err = parseTaskTime(createdAt)
	return version, err
}

func cloneSubscriptionSourceVersion(version SubscriptionSourceVersion) SubscriptionSourceVersion {
	version.RawBody = bytes.Clone(version.RawBody)
	version.NormalizedNodes = bytes.Clone(version.NormalizedNodes)
	version.Diagnostics = bytes.Clone(version.Diagnostics)
	return version
}
