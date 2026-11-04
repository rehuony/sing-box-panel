// SPDX-License-Identifier: GPL-3.0-or-later

// Package database opens a service's local SQLite database. It validates the
// path, holds a lock on the data directory for the lifetime of the handle,
// applies connection pragmas through the DSN, and creates or verifies the
// store's baseline schema identity.
package database

import (
	"context"
	"database/sql"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"sync"

	"modernc.org/sqlite"
)

const (
	// BusyTimeoutMillis is the per-connection busy timeout.
	BusyTimeoutMillis = 5_000

	maxOpenConnections = 4

	// MigrationMarker in a data directory blocks ordinary opens until an
	// interrupted data-directory move is recovered.
	MigrationMarker = ".data-migration"
)

var (
	ErrUnexpectedApplicationID = errors.New("unexpected SQLite application id")
	ErrSchemaUnsupported       = errors.New("SQLite schema is unsupported; use a fresh data directory")
	ErrSchemaTooNew            = errors.New("SQLite schema is newer than this binary")
	ErrSchemaInconsistent      = errors.New("SQLite schema metadata is inconsistent")
)

// Schema is one store's baseline. A store opens only databases with exactly
// this application ID and version and creates it in an empty database.
type Schema struct {
	ApplicationID int
	Version       int
	SQL           string
}

// Info identifies the database format independently of table contents.
type Info struct {
	ApplicationID int
	Version       int
}

// Check rejects an identity other than this schema's.
func (schema Schema) Check(applicationID, version int) error {
	if applicationID != schema.ApplicationID {
		return fmt.Errorf("%w: got %#x, want %#x", ErrUnexpectedApplicationID, applicationID, schema.ApplicationID)
	}
	if version > schema.Version {
		return fmt.Errorf("%w: got %d, supported %d", ErrSchemaTooNew, version, schema.Version)
	}
	if version != schema.Version {
		return fmt.Errorf("%w: got %d, supported %d", ErrSchemaUnsupported, version, schema.Version)
	}
	return nil
}

// DB owns the process-local connection pool for one database file.
//
// Every connection is initialized through the driver DSN. database/sql may
// open physical connections after Open returns, so issuing connection-scoped
// PRAGMAs once against *sql.DB would be insufficient.
type DB struct {
	*sql.DB
	path      string
	schema    Schema
	closeOnce sync.Once
	closeErr  error
	dataLock  *os.File
}

// Open opens (or creates) the database at path, initializes or verifies its
// schema, and returns a shared connection pool. The parent directory must
// already exist. An exclusive open excludes every other owner of the data
// directory, as a data-directory move requires.
func Open(ctx context.Context, path string, schema Schema, exclusive bool) (*DB, error) {
	absPath, err := validatePath(path)
	if err != nil {
		return nil, err
	}
	dataLock, err := lockDirectory(filepath.Dir(absPath), exclusive)
	if err != nil {
		return nil, err
	}
	lockOwned := true
	defer func() {
		if lockOwned && dataLock != nil {
			_ = dataLock.Close()
		}
	}()
	if !exclusive {
		if _, err := os.Lstat(filepath.Join(filepath.Dir(absPath), MigrationMarker)); err == nil {
			return nil, errors.New("data directory migration is incomplete; start the service to recover it")
		} else if !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
	}
	file, err := os.OpenFile(absPath, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return nil, fmt.Errorf("create SQLite database file: %w", err)
	}
	if err := file.Close(); err != nil {
		return nil, fmt.Errorf("close new SQLite database file: %w", err)
	}

	connector, err := sqlite.NewConnector(dsn(absPath))
	if err != nil {
		return nil, fmt.Errorf("construct SQLite connector: %w", err)
	}
	pool := sql.OpenDB(connector)
	pool.SetMaxOpenConns(maxOpenConnections)
	pool.SetMaxIdleConns(maxOpenConnections)
	db := &DB{DB: pool, path: absPath, schema: schema, dataLock: dataLock}
	ok := false
	defer func() {
		if !ok {
			_ = pool.Close()
		}
	}()

	if err := pool.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("open SQLite database: %w", err)
	}
	if err := db.initializeSchema(ctx); err != nil {
		return nil, err
	}
	if _, err := db.Info(ctx); err != nil {
		return nil, err
	}
	if err := os.Chmod(absPath, 0o600); err != nil {
		return nil, fmt.Errorf("secure SQLite database permissions: %w", err)
	}
	ok = true
	lockOwned = false
	return db, nil
}

func validatePath(path string) (string, error) {
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

func dsn(path string) string {
	u := &url.URL{Scheme: "file", Path: filepath.ToSlash(path)}
	query := u.Query()
	query.Set("mode", "rwc")
	query.Set("_busy_timeout", fmt.Sprint(BusyTimeoutMillis))
	query.Set("_foreign_keys", "on")
	query.Set("_journal_mode", "wal")
	query.Set("_synchronous", "full")
	query.Set("_txlock", "immediate")
	query.Set("_defensive", "1")
	query.Add("_pragma", "trusted_schema(OFF)")
	u.RawQuery = query.Encode()
	return u.String()
}

// initializeSchema creates the baseline in an empty database or verifies the
// identity of an existing one. Earlier, unrelated and newer formats are
// rejected without conversion.
func (db *DB) initializeSchema(ctx context.Context) error {
	return db.WithTx(ctx, func(tx *sql.Tx) error {
		applicationID, err := pragmaInt(ctx, tx, "application_id")
		if err != nil {
			return err
		}
		version, err := pragmaInt(ctx, tx, "user_version")
		if err != nil {
			return err
		}
		if applicationID != 0 {
			return db.schema.Check(applicationID, version)
		}
		hasObjects, err := hasUserSchemaObjects(ctx, tx)
		if err != nil {
			return err
		}
		if version != 0 || hasObjects {
			return fmt.Errorf("%w: refusing to adopt an unidentified non-empty database", ErrUnexpectedApplicationID)
		}
		if _, err := tx.ExecContext(ctx, db.schema.SQL); err != nil {
			return fmt.Errorf("initialize SQLite schema: %w", err)
		}
		if _, err := tx.ExecContext(ctx, fmt.Sprintf("PRAGMA application_id = %d; PRAGMA user_version = %d", db.schema.ApplicationID, db.schema.Version)); err != nil {
			return fmt.Errorf("set SQLite schema identity: %w", err)
		}
		return nil
	})
}

type queryRower interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func hasUserSchemaObjects(ctx context.Context, q queryRower) (bool, error) {
	var count int
	err := q.QueryRowContext(ctx, `SELECT count(*) FROM sqlite_schema WHERE name NOT LIKE 'sqlite_%'`).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("inspect SQLite schema: %w", err)
	}
	return count != 0, nil
}

func pragmaInt(ctx context.Context, q queryRower, name string) (int, error) {
	var value int
	if err := q.QueryRowContext(ctx, "PRAGMA "+name).Scan(&value); err != nil {
		return 0, fmt.Errorf("read SQLite PRAGMA %s: %w", name, err)
	}
	return value, nil
}

// Info returns the persistent schema identity and version.
func (db *DB) Info(ctx context.Context) (Info, error) {
	applicationID, err := pragmaInt(ctx, db.DB, "application_id")
	if err != nil {
		return Info{}, err
	}
	version, err := pragmaInt(ctx, db.DB, "user_version")
	if err != nil {
		return Info{}, err
	}
	if err := db.schema.Check(applicationID, version); err != nil {
		return Info{}, err
	}
	return Info{ApplicationID: applicationID, Version: version}, nil
}

// Path returns the absolute local filesystem path of the database.
func (db *DB) Path() string {
	if db == nil {
		return ""
	}
	return db.path
}

// Close releases every connection and the data-directory lock.
func (db *DB) Close() error {
	if db == nil || db.DB == nil {
		return nil
	}
	db.closeOnce.Do(func() {
		if err := db.DB.Close(); err != nil {
			db.closeErr = fmt.Errorf("close SQLite database: %w", err)
		}
		if db.dataLock != nil {
			db.closeErr = errors.Join(db.closeErr, db.dataLock.Close())
		}
	})
	return db.closeErr
}

// WithTx executes fn in a short IMMEDIATE SQLite transaction. The callback
// must only perform database work needed for one invariant; callers keep
// network, child-process and filesystem effects outside this boundary.
func (db *DB) WithTx(ctx context.Context, fn func(*sql.Tx) error) error {
	if db == nil || db.DB == nil {
		return errors.New("SQLite database is not open")
	}
	if fn == nil {
		return errors.New("SQLite transaction callback is nil")
	}
	tx, err := db.BeginTx(ctx, nil)
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

// LockDirectoryForCleanup excludes all database owners of dataDir until the
// returned file is closed. It must be held in addition to the service lease.
func LockDirectoryForCleanup(dataDir string) (*os.File, error) {
	return lockDirectory(dataDir, true)
}

// ReadApplicationID inspects committed identity without initializing a
// database. With no WAL it reads the SQLite header directly: opening even a
// mode=ro SQLite connection can create WAL sidecars. An existing WAL requires
// its shared-memory file; incomplete recovery state is left untouched and
// reported as an error.
func ReadApplicationID(ctx context.Context, path string) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return 0, err
	}
	wal, err := os.Lstat(abs + "-wal")
	if errors.Is(err, os.ErrNotExist) {
		file, err := os.Open(abs)
		if err != nil {
			return 0, err
		}
		defer file.Close()
		var header [100]byte
		if _, err := io.ReadFull(file, header[:]); err != nil {
			return 0, err
		}
		if string(header[:16]) != "SQLite format 3\x00" {
			return 0, errors.New("not a SQLite database")
		}
		return int(int32(binary.BigEndian.Uint32(header[68:72]))), nil
	}
	if err != nil {
		return 0, err
	}
	shm, err := os.Lstat(abs + "-shm")
	if err != nil {
		return 0, fmt.Errorf("cannot inspect WAL identity without existing shared memory: %w", err)
	}
	if !wal.Mode().IsRegular() || !shm.Mode().IsRegular() {
		return 0, errors.New("database sidecars must be regular files")
	}
	location := url.URL{Scheme: "file", Path: filepath.ToSlash(abs), RawQuery: "mode=ro"}
	connector, err := sqlite.NewConnector(location.String())
	if err != nil {
		return 0, err
	}
	db := sql.OpenDB(connector)
	defer db.Close()
	db.SetMaxOpenConns(1)
	var id int
	err = db.QueryRowContext(ctx, "PRAGMA application_id").Scan(&id)
	return id, err
}
