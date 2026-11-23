// SPDX-License-Identifier: GPL-3.0-or-later

package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/rehuony/sing-box-panel/internal/configuration"
)

var ErrRevisionConflict = errors.New("a revision with this ID holds different configuration")

// RecordRevision stores a configuration revision the panel sent and makes it
// the head that runtime commands snapshot. The ID is the panel's revision ID;
// a revision already received under that ID must hold the same document.
func (s *Store) RecordRevision(ctx context.Context, id string, document json.RawMessage, receivedAt time.Time) (CanonicalRevision, error) {
	prepared, err := prepareCanonicalSave(NewCanonicalRevision{
		ID: id, SchemaVersion: configuration.SchemaVersion, Document: document, CommandID: id, CreatedAt: receivedAt,
	})
	if err != nil {
		return CanonicalRevision{}, err
	}
	var result CanonicalRevision
	err = s.WithTx(ctx, func(tx *sql.Tx) error {
		existing, err := getCanonicalRevision(tx.QueryRowContext(ctx,
			`SELECT `+canonicalRevisionColumns+` FROM canonical_revisions WHERE id = ?`, id))
		switch {
		case err == nil:
			if existing.SHA256 != prepared.SHA256 {
				return ErrRevisionConflict
			}
			result = existing
			_, err = tx.ExecContext(ctx, `UPDATE hub_state SET head_revision_id = ?, updated_at = ? WHERE singleton = 1`,
				id, receivedAt.UTC().Format(time.RFC3339Nano))
			return err
		case !errors.Is(err, ErrCanonicalRevisionNotFound):
			return err
		}
		result, err = saveCanonicalRevisionTx(ctx, tx, prepared)
		return err
	})
	return result, err
}
