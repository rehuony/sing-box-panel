package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/rehuony/sing-box-panel/internal/database"
)

const (
	// ApplicationID identifies the current incompatible storage epoch.
	ApplicationID = 0x53425034
	// LogsApplicationID identifies the log database.
	LogsApplicationID = 0x5342504C
	// LogsFile is the log database's file name, beside the panel database.
	LogsFile = "logs.db"
)

// Store owns the panel database and, beside it, the log database.
type Store struct {
	db         *sql.DB
	handle     *database.DB
	logs       *sql.DB
	logsHandle *database.DB
}

// Open opens (or creates) the panel database at path and the log database
// beside it, applies the migrations they have not recorded, verifies their
// identity, and returns shared connection pools.
//
// The parent directory must already exist. Directory creation and ownership are
// responsibilities of the application composition root.
func Open(ctx context.Context, path string) (*Store, error) {
	return open(ctx, path, false)
}

// OpenForMigration excludes every other database owner while storage moves.
func OpenForMigration(ctx context.Context, path string) (*Store, error) {
	return open(ctx, path, true)
}

func open(ctx context.Context, path string, exclusive bool) (*Store, error) {
	schema, err := Schema()
	if err != nil {
		return nil, err
	}
	logSchema, err := LogSchema()
	if err != nil {
		return nil, err
	}
	handle, err := database.Open(ctx, path, schema, exclusive)
	if err != nil {
		return nil, err
	}
	logsHandle, err := database.OpenBeside(ctx, handle, LogsFile, logSchema)
	if err != nil {
		return nil, errors.Join(fmt.Errorf("open log database: %w", err), handle.Close())
	}
	store := &Store{db: handle.DB, handle: handle, logs: logsHandle.DB, logsHandle: logsHandle}
	if err := store.moveLogEntries(ctx); err != nil {
		return nil, errors.Join(fmt.Errorf("move log entries to the log database: %w", err), store.Close())
	}
	if _, err := handle.ExecContext(ctx, `DELETE FROM auth_sessions WHERE expires_at <= ?`, time.Now().Unix()); err != nil {
		return nil, errors.Join(fmt.Errorf("clean expired sessions: %w", err), store.Close())
	}
	return store, nil
}

// moveLogEntries copies the log entries that the panel database's upgrade
// set aside into the log database, then drops their table. The copy commits
// before the drop, and copying again after an interruption adds nothing,
// because every entry keeps its ID.
func (s *Store) moveLogEntries(ctx context.Context) error {
	var pending int
	if err := s.db.QueryRowContext(ctx, `SELECT count(*) FROM sqlite_schema WHERE type = 'table' AND name = 'log_entries_to_move'`).Scan(&pending); err != nil {
		return err
	}
	if pending == 0 {
		return nil
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id, occurred_at, source, level, code, message, metadata_json FROM log_entries_to_move`)
	if err != nil {
		return err
	}
	err = s.logsHandle.WithTx(ctx, func(tx *sql.Tx) error {
		insert, err := tx.PrepareContext(ctx, `INSERT INTO log_entries(id, occurred_at, source, level, code, message, metadata_json)
            VALUES (?, ?, ?, ?, ?, ?, ?) ON CONFLICT(id) DO NOTHING`)
		if err != nil {
			return err
		}
		defer insert.Close()
		var values [7]string
		for rows.Next() {
			if err := rows.Scan(&values[0], &values[1], &values[2], &values[3], &values[4], &values[5], &values[6]); err != nil {
				return err
			}
			if _, err := insert.ExecContext(ctx, values[0], values[1], values[2], values[3], values[4], values[5], values[6]); err != nil {
				return err
			}
		}
		return rows.Err()
	})
	err = errors.Join(err, rows.Close())
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `DROP TABLE log_entries_to_move`)
	return err
}

// Close releases all database connections owned by the store.
func (s *Store) Close() error {
	if s == nil {
		return nil
	}
	return errors.Join(s.logsHandle.Close(), s.handle.Close())
}

// Path returns the absolute local filesystem path of the panel database.
func (s *Store) Path() string {
	if s == nil {
		return ""
	}
	return s.handle.Path()
}

// WithTx executes fn in a short IMMEDIATE SQLite transaction. The callback
// must only perform database work needed for one invariant; callers must keep
// network, child-process, and filesystem effects outside this boundary.
func (s *Store) WithTx(ctx context.Context, fn func(*sql.Tx) error) error {
	if s == nil {
		return errors.New("SQLite store is not open")
	}
	return s.handle.WithTx(ctx, fn)
}
