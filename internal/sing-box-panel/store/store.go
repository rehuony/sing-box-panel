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

	// CurrentSchemaVersion is the only schema this package can open. Version 19
	// is the baseline; databases written by earlier versions are rejected.
	CurrentSchemaVersion = 19
)

// Store owns the panel database.
type Store struct {
	db     *sql.DB
	handle *database.DB
}

// Open opens (or creates) the panel database, initializes the current schema,
// verifies its identity, and returns a shared connection pool.
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
	handle, err := database.Open(ctx, path, database.Schema{
		ApplicationID: ApplicationID, Version: CurrentSchemaVersion, SQL: databaseSchema,
	}, exclusive)
	if err != nil {
		return nil, err
	}
	if _, err := handle.ExecContext(ctx, `DELETE FROM auth_sessions WHERE expires_at <= ?`, time.Now().Unix()); err != nil {
		return nil, errors.Join(fmt.Errorf("clean expired sessions: %w", err), handle.Close())
	}
	return &Store{db: handle.DB, handle: handle}, nil
}

// Close releases all database connections owned by the store.
func (s *Store) Close() error {
	if s == nil {
		return nil
	}
	return s.handle.Close()
}

// Path returns the absolute local filesystem path of the database.
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
