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

func scanCanonicalRevision(row rowScanner) (CanonicalRevision, error) {
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
