package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/rehuony/sing-box-panel/internal/configuration"
)

// CanonicalRevision is one immutable full snapshot of canonical configuration.
type CanonicalRevision struct {
	ID            string
	Sequence      int64
	ParentID      string
	SchemaVersion int
	Document      json.RawMessage
	SHA256        string
	CommandID     string
	CreatedAt     time.Time
}

// NewCanonicalRevision contains caller-owned identity and canonical bytes. The
// store derives the parent, sequence, and digest inside the CAS operation.
type NewCanonicalRevision struct {
	ID            string
	SchemaVersion int
	Document      json.RawMessage
	CommandID     string
	CreatedAt     time.Time
}

// saveCanonicalRevisionTx records a parsed revision of nodeID's
// configuration and makes it the node's head. A byte-identical document reuses
// the current head.
func saveCanonicalRevisionTx(
	ctx context.Context,
	tx *sql.Tx,
	nodeID string,
	prepared CanonicalRevision,
) (CanonicalRevision, error) {
	var currentHead sql.NullString
	err := tx.QueryRowContext(ctx, `SELECT head_revision_id FROM node_configurations WHERE node_id = ?`, nodeID).Scan(&currentHead)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return CanonicalRevision{}, fmt.Errorf("read canonical head: %w", err)
	}
	actualHead := valueOrEmpty(currentHead)
	sequence := int64(1)
	if currentHead.Valid {
		head, err := getCanonicalRevision(tx.QueryRowContext(
			ctx,
			`SELECT `+canonicalRevisionColumns+` FROM canonical_revisions WHERE id = ?`,
			currentHead.String,
		))
		if err != nil {
			return CanonicalRevision{}, fmt.Errorf("read canonical head revision: %w", err)
		}
		if head.SHA256 == prepared.SHA256 {
			return head, nil
		}
		sequence = head.Sequence + 1
	}
	prepared.Sequence = sequence
	prepared.ParentID = actualHead
	if _, err := tx.ExecContext(
		ctx,
		`INSERT INTO canonical_revisions(
            id, node_id, sequence, parent_id, schema_version, document_json,
            sha256, command_id, created_at
         ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		prepared.ID,
		nodeID,
		prepared.Sequence,
		nullIfEmpty(prepared.ParentID),
		prepared.SchemaVersion,
		string(prepared.Document),
		prepared.SHA256,
		prepared.CommandID,
		prepared.CreatedAt.Format(time.RFC3339Nano),
	); err != nil {
		return CanonicalRevision{}, fmt.Errorf("insert canonical revision: %w", err)
	}
	return prepared, nil
}

func prepareCanonicalSave(
	revision NewCanonicalRevision,
) (CanonicalRevision, error) {
	if strings.TrimSpace(revision.ID) == "" {
		return CanonicalRevision{}, errors.New("canonical revision id is empty")
	}
	if revision.SchemaVersion != configuration.SchemaVersion {
		return CanonicalRevision{}, fmt.Errorf(
			"canonical schema version must be exactly %d",
			configuration.SchemaVersion,
		)
	}
	document, err := configuration.Parse(revision.Document)
	if err != nil {
		return CanonicalRevision{}, fmt.Errorf("canonical document: %w", err)
	}
	revision.Document = document.CanonicalJSON()
	if strings.TrimSpace(revision.CommandID) == "" {
		return CanonicalRevision{}, errors.New("canonical command id is empty")
	}
	if revision.CreatedAt.IsZero() {
		revision.CreatedAt = time.Now().UTC()
	} else {
		revision.CreatedAt = revision.CreatedAt.UTC()
	}

	digest := sha256.Sum256(revision.Document)
	return CanonicalRevision{
		ID:            revision.ID,
		SchemaVersion: revision.SchemaVersion,
		Document:      append(json.RawMessage(nil), revision.Document...),
		SHA256:        hex.EncodeToString(digest[:]),
		CommandID:     revision.CommandID,
		CreatedAt:     revision.CreatedAt,
	}, nil
}

func nullIfEmpty(value string) any {
	if value == "" {
		return nil
	}
	return value
}

// Head returns nodeID's latest parsed revision, or nil before the first
// configuration that parses.
func (s *Store) Head(ctx context.Context, nodeID string) (*CanonicalRevision, error) {
	revision, err := getCanonicalRevision(s.db.QueryRowContext(ctx, `SELECT `+canonicalRevisionColumns+`
        FROM canonical_revisions WHERE id = (SELECT head_revision_id FROM node_configurations WHERE node_id = ?)`, nodeID))
	if errors.Is(err, ErrCanonicalRevisionNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &revision, nil
}

func valueOrEmpty(value sql.NullString) string {
	if value.Valid {
		return value.String
	}
	return ""
}
