// SPDX-License-Identifier: GPL-3.0-or-later

package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
)

// SnapshotDataFiles creates consistent snapshots of the panel database and
// the log database in dir under their own names, including committed WAL
// content. The caller keeps the exclusive data-directory lock until done.
func (s *Store) SnapshotDataFiles(ctx context.Context, dir string) error {
	if _, err := s.db.ExecContext(ctx, "VACUUM INTO ?", filepath.Join(dir, filepath.Base(s.handle.Path()))); err != nil {
		return err
	}
	_, err := s.logs.ExecContext(ctx, "VACUUM INTO ?", filepath.Join(dir, LogsFile))
	return err
}

// RecordDataMove marks this database as the snapshot of a data directory
// move. Cached archives are found by digest under the data directory, so no
// stored path changes.
func (s *Store) RecordDataMove(ctx context.Context, source, destination, id string) error {
	_, err := s.db.ExecContext(ctx, "INSERT INTO data_directory_moves(id,source_path,target_path) VALUES(?,?,?)", id, source, destination)
	return err
}

// VerifyDataMove proves this database is the committed relocation snapshot.
func (s *Store) VerifyDataMove(ctx context.Context, id, source, destination string) error {
	var count int
	if err := s.db.QueryRowContext(ctx, "SELECT count(*) FROM data_directory_moves WHERE id=? AND source_path=? AND target_path=?", id, source, destination).Scan(&count); err != nil {
		return err
	}
	if count != 1 {
		return errors.New("destination database does not match this migration")
	}
	for _, db := range []*sql.DB{s.db, s.logs} {
		var integrity string
		if err := db.QueryRowContext(ctx, "PRAGMA quick_check").Scan(&integrity); err != nil {
			return err
		}
		if integrity != "ok" {
			return fmt.Errorf("destination database integrity check failed")
		}
	}
	return nil
}
