// SPDX-License-Identifier: GPL-3.0-or-later

package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
)

// SnapshotDataFile creates a consistent SQLite snapshot, including committed
// WAL content. The caller keeps the exclusive data-directory lock until done.
func (s *Store) SnapshotDataFile(ctx context.Context, destination string) error {
	_, err := s.db.ExecContext(ctx, "VACUUM INTO ?", destination)
	return err
}

// RebaseDataPaths changes only panel-owned storage references. Native sing-box
// configuration and historical evidence are never rewritten by a data move.
func (s *Store) RebaseDataPaths(ctx context.Context, source, destination, id string) error {
	physicalSource, err := filepath.EvalSymlinks(source)
	if err != nil {
		return err
	}
	physicalTarget, err := filepath.EvalSymlinks(destination)
	if err != nil {
		return err
	}
	return s.WithTx(ctx, func(tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, "SELECT id, binary_path FROM core_artifacts")
		if err != nil {
			return err
		}
		type entry struct{ id, path string }
		var entries []entry
		for rows.Next() {
			var e entry
			if err := rows.Scan(&e.id, &e.path); err != nil {
				rows.Close()
				return err
			}
			entries = append(entries, e)
		}
		err = errors.Join(rows.Err(), rows.Close())
		if err != nil {
			return err
		}
		for _, e := range entries {
			next, ok := rebasedPath(e.path, source, physicalTarget)
			if !ok {
				next, ok = rebasedPath(e.path, physicalSource, physicalTarget)
			}
			if ok {
				if _, err := tx.ExecContext(ctx, "UPDATE core_artifacts SET binary_path=? WHERE id=?", next, e.id); err != nil {
					return err
				}
			}
		}
		_, err = tx.ExecContext(ctx, "INSERT INTO data_directory_moves(id,source_path,target_path) VALUES(?,?,?)", id, source, destination)
		return err
	})
}

func rebasedPath(path, source, destination string) (string, bool) {
	rel, err := filepath.Rel(source, path)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return path, false
	}
	return filepath.Join(destination, rel), true
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
	var integrity string
	if err := s.db.QueryRowContext(ctx, "PRAGMA quick_check").Scan(&integrity); err != nil {
		return err
	}
	if integrity != "ok" {
		return fmt.Errorf("destination database integrity check failed")
	}
	return nil
}
