package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sync"

	"modernc.org/sqlite"
)

const (
	// ApplicationID is the SQLite application_id for sing-box-panel ("SBPN").
	ApplicationID = 0x5342504e

	// CurrentSchemaVersion is the newest schema this package can open.
	CurrentSchemaVersion = 2

	defaultBusyTimeoutMillis  = 5_000
	defaultMaxOpenConnections = 4
)

var (
	ErrUnexpectedApplicationID = errors.New("unexpected SQLite application id")
	ErrSchemaTooNew            = errors.New("SQLite schema is newer than this binary")
	ErrSchemaInconsistent      = errors.New("SQLite schema metadata is inconsistent")
)

// Store owns the process-local connection pool for one panel database.
//
// Every connection is initialized through the driver DSN. This is important:
// database/sql may open physical connections after Open returns, so issuing
// connection-scoped PRAGMAs once against *sql.DB would be insufficient.
type Store struct {
	db        *sql.DB
	path      string
	closeOnce sync.Once
	closeErr  error
}

// Open opens (or creates) a local SQLite database, applies embedded migrations,
// verifies its identity, and returns a shared connection pool.
//
// The parent directory must already exist. Directory creation and ownership are
// responsibilities of the application composition root.
func Open(ctx context.Context, path string) (*Store, error) {
	absPath, err := validateDatabasePath(path)
	if err != nil {
		return nil, err
	}
	file, err := os.OpenFile(absPath, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return nil, fmt.Errorf("create SQLite database file: %w", err)
	}
	if err := file.Close(); err != nil {
		return nil, fmt.Errorf("close new SQLite database file: %w", err)
	}

	connector, err := sqlite.NewConnector(databaseDSN(absPath))
	if err != nil {
		return nil, fmt.Errorf("construct SQLite connector: %w", err)
	}

	db := sql.OpenDB(connector)
	db.SetMaxOpenConns(defaultMaxOpenConnections)
	db.SetMaxIdleConns(defaultMaxOpenConnections)

	store := &Store{db: db, path: absPath}
	ok := false
	defer func() {
		if !ok {
			_ = db.Close()
		}
	}()

	if err := db.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("open SQLite database: %w", err)
	}
	if err := store.migrate(ctx); err != nil {
		return nil, err
	}
	if _, err := store.SchemaInfo(ctx); err != nil {
		return nil, err
	}
	if err := os.Chmod(absPath, 0o600); err != nil {
		return nil, fmt.Errorf("secure SQLite database permissions: %w", err)
	}

	ok = true
	return store, nil
}

func validateDatabasePath(path string) (string, error) {
	if path == "" {
		return "", errors.New("SQLite database path is empty")
	}
	if path == ":memory:" {
		return "", errors.New("SQLite database must use a local filesystem path")
	}

	absPath, err := filepath.Abs(filepath.Clean(path))
	if err != nil {
		return "", fmt.Errorf("resolve SQLite database path: %w", err)
	}
	parent, err := os.Stat(filepath.Dir(absPath))
	if err != nil {
		return "", fmt.Errorf("inspect SQLite database directory: %w", err)
	}
	if !parent.IsDir() {
		return "", fmt.Errorf("SQLite database parent is not a directory: %s", filepath.Dir(absPath))
	}
	if info, err := os.Stat(absPath); err == nil && info.IsDir() {
		return "", fmt.Errorf("SQLite database path is a directory: %s", absPath)
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("inspect SQLite database path: %w", err)
	}

	return absPath, nil
}

func databaseDSN(path string) string {
	u := &url.URL{Scheme: "file", Path: filepath.ToSlash(path)}
	query := u.Query()
	query.Set("mode", "rwc")
	query.Set("_busy_timeout", fmt.Sprint(defaultBusyTimeoutMillis))
	query.Set("_foreign_keys", "on")
	query.Set("_journal_mode", "wal")
	query.Set("_synchronous", "full")
	query.Set("_txlock", "immediate")
	query.Set("_defensive", "1")
	query.Add("_pragma", "trusted_schema(OFF)")
	u.RawQuery = query.Encode()
	return u.String()
}

// Close releases all database connections owned by the store.
func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	s.closeOnce.Do(func() {
		if err := s.db.Close(); err != nil {
			s.closeErr = fmt.Errorf("close SQLite database: %w", err)
		}
	})
	return s.closeErr
}

// Path returns the absolute local filesystem path of the database.
func (s *Store) Path() string {
	if s == nil {
		return ""
	}
	return s.path
}

// WithTx executes fn in a short IMMEDIATE SQLite transaction. The callback
// must only perform database work needed for one invariant; callers must keep
// network, child-process, and filesystem effects outside this boundary.
func (s *Store) WithTx(ctx context.Context, fn func(*sql.Tx) error) error {
	if s == nil || s.db == nil {
		return errors.New("SQLite store is not open")
	}
	if fn == nil {
		return errors.New("SQLite transaction callback is nil")
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin SQLite transaction: %w", err)
	}
	defer func() {
		_ = tx.Rollback()
	}()

	if err := fn(tx); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit SQLite transaction: %w", err)
	}
	return nil
}
