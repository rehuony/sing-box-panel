package store

import (
	"context"
	"database/sql"
	"errors"

	"github.com/rehuony/sing-box-panel/internal/database"
)

const (
	// ApplicationID identifies the node database ("SBPN").
	ApplicationID = 0x5342504E

	// CurrentSchemaVersion is the only schema this package can open; databases
	// written by earlier versions are rejected.
	CurrentSchemaVersion = 1
)

// Store owns the node database: the installed cores, the configurations the
// panel sent, checked startup artifacts, activation bundles, runtime intents,
// observations and recovery, and the results of completed commands.
type Store struct {
	db     *sql.DB
	handle *database.DB
}

// Open opens (or creates) the node database, initializes the current schema,
// verifies its identity, and returns a shared connection pool.
//
// The parent directory must already exist. Directory creation and ownership are
// responsibilities of the application composition root.
func Open(ctx context.Context, path string) (*Store, error) {
	return open(ctx, path, false)
}

func open(ctx context.Context, path string, exclusive bool) (*Store, error) {
	handle, err := database.Open(ctx, path, database.Schema{
		ApplicationID: ApplicationID, Version: CurrentSchemaVersion, SQL: databaseSchema,
	}, exclusive)
	if err != nil {
		return nil, err
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
