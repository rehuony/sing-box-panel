package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

var ErrCanonicalRevisionNotFound = errors.New("canonical revision not found")

const canonicalRevisionColumns = `
    id, sequence, parent_id, schema_version, document_json,
    sha256, command_id, created_at`

// CanonicalRevisionListFilter selects a newest-first sequence page.
// Cursor is exclusive; nil starts from the current newest revision.
type CanonicalRevisionListFilter struct {
	Cursor *CanonicalRevisionCursor
	Limit  int
}

type CanonicalRevisionCursor struct {
	BeforeSequence int64
}

type CanonicalRevisionPage struct {
	Items []CanonicalRevision
	Next  *CanonicalRevisionCursor
}

// GetCanonicalRevision returns one immutable revision by ID.
func (s *Store) GetCanonicalRevision(
	ctx context.Context,
	revisionID string,
) (CanonicalRevision, error) {
	if revisionID == "" {
		return CanonicalRevision{}, errors.New("canonical revision id is empty")
	}
	return getCanonicalRevision(
		s.db.QueryRowContext(
			ctx,
			`SELECT `+canonicalRevisionColumns+` FROM canonical_revisions WHERE id = ?`,
			revisionID,
		),
	)
}

// GetCanonicalRevisionBySequence returns one immutable revision by its stable
// monotonically increasing sequence.
func (s *Store) GetCanonicalRevisionBySequence(
	ctx context.Context,
	sequence int64,
) (CanonicalRevision, error) {
	if sequence < 1 {
		return CanonicalRevision{}, errors.New("canonical revision sequence must be positive")
	}
	return getCanonicalRevision(
		s.db.QueryRowContext(
			ctx,
			`SELECT `+canonicalRevisionColumns+` FROM canonical_revisions WHERE sequence = ?`,
			sequence,
		),
	)
}

// ListCanonicalRevisions returns immutable snapshots newest first.
func (s *Store) ListCanonicalRevisions(
	ctx context.Context,
	filter CanonicalRevisionListFilter,
) (CanonicalRevisionPage, error) {
	limit, err := normalizePageLimit(filter.Limit)
	if err != nil {
		return CanonicalRevisionPage{}, err
	}
	if filter.Cursor != nil && filter.Cursor.BeforeSequence < 1 {
		return CanonicalRevisionPage{}, errors.New("canonical revision cursor must be positive")
	}

	query := `SELECT ` + canonicalRevisionColumns + ` FROM canonical_revisions`
	args := make([]any, 0, 2)
	if filter.Cursor != nil {
		query += ` WHERE sequence < ?`
		args = append(args, filter.Cursor.BeforeSequence)
	}
	query += ` ORDER BY sequence DESC LIMIT ?`
	args = append(args, limit+1)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return CanonicalRevisionPage{}, fmt.Errorf("list canonical revisions: %w", err)
	}
	defer rows.Close()

	items := make([]CanonicalRevision, 0, limit+1)
	for rows.Next() {
		revision, err := scanCanonicalRevision(rows)
		if err != nil {
			return CanonicalRevisionPage{}, fmt.Errorf("scan listed canonical revision: %w", err)
		}
		items = append(items, revision)
	}
	if err := rows.Err(); err != nil {
		return CanonicalRevisionPage{}, fmt.Errorf("iterate listed canonical revisions: %w", err)
	}

	page := CanonicalRevisionPage{Items: items}
	if len(items) > limit {
		page.Items = items[:limit]
		page.Next = &CanonicalRevisionCursor{
			BeforeSequence: page.Items[len(page.Items)-1].Sequence,
		}
	}
	return page, nil
}

func getCanonicalRevision(row *sql.Row) (CanonicalRevision, error) {
	revision, err := scanCanonicalRevision(row)
	if errors.Is(err, sql.ErrNoRows) {
		return CanonicalRevision{}, ErrCanonicalRevisionNotFound
	}
	if err != nil {
		return CanonicalRevision{}, fmt.Errorf("get canonical revision: %w", err)
	}
	return revision, nil
}

func scanCanonicalRevision(row taskScanner) (CanonicalRevision, error) {
	var (
		revision  CanonicalRevision
		parentID  sql.NullString
		document  string
		createdAt string
	)
	if err := row.Scan(
		&revision.ID,
		&revision.Sequence,
		&parentID,
		&revision.SchemaVersion,
		&document,
		&revision.SHA256,
		&revision.CommandID,
		&createdAt,
	); err != nil {
		return CanonicalRevision{}, err
	}
	revision.ParentID = valueOrEmpty(parentID)
	revision.Document = append(json.RawMessage(nil), document...)
	parsed, err := time.Parse(time.RFC3339Nano, createdAt)
	if err != nil {
		return CanonicalRevision{}, fmt.Errorf("parse created_at: %w", err)
	}
	revision.CreatedAt = parsed
	return revision, nil
}
