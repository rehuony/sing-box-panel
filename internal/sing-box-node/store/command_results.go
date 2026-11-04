// SPDX-License-Identifier: GPL-3.0-or-later

package store

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// CommandResultRetention is how long a command's result answers a repeated
// request ID.
const CommandResultRetention = 24 * time.Hour

// CommandResult returns the encoded result recorded for requestID.
func (s *Store) CommandResult(ctx context.Context, requestID string) ([]byte, bool, error) {
	var result []byte
	err := s.db.QueryRowContext(ctx, `SELECT result FROM command_results WHERE request_id = ?`, requestID).Scan(&result)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, nil
	}
	return result, err == nil, err
}

// RecordCommandResult records the encoded result of requestID and forgets
// results older than CommandResultRetention.
func (s *Store) RecordCommandResult(ctx context.Context, requestID string, result []byte, completedAt time.Time) error {
	return s.WithTx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `DELETE FROM command_results WHERE completed_at < ?`,
			completedAt.Add(-CommandResultRetention).UTC().Format(time.RFC3339Nano)); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, `INSERT INTO command_results(request_id, result, completed_at) VALUES(?,?,?)
			ON CONFLICT(request_id) DO NOTHING`, requestID, result, completedAt.UTC().Format(time.RFC3339Nano))
		return err
	})
}
