// SPDX-License-Identifier: GPL-3.0-or-later

// Package database opens a service's local SQLite database. It validates the
// path, holds a lock on the data directory for the lifetime of the handle,
// applies connection pragmas through the DSN, and brings the store's schema to
// its latest version by applying its numbered migrations in order.
package database

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"sync"
	"time"

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

// Schema is one store's database identity and the migrations that build its
// tables, in order. Migration N takes a database from version N-1 to N; an
// empty database is version 0 and receives every migration.
type Schema struct {
	ApplicationID int
	Migrations    []Migration
}

// Version is the schema version the migrations build.
func (schema Schema) Version() int {
	return len(schema.Migrations)
}

// Migration is one numbered schema change. It is applied once, inside the
// transaction that records it, and never edited after it ships: a recorded
// migration whose text differs from the binary's is refused.
type Migration struct {
	Version int
	Name    string
	SQL     string
}

// Info identifies the database format independently of table contents.
type Info struct {
	ApplicationID int
	Version       int
}

// Check rejects an identity other than this schema's latest version.
func (schema Schema) Check(applicationID, version int) error {
	if applicationID != schema.ApplicationID {
		return fmt.Errorf("%w: got %#x, want %#x", ErrUnexpectedApplicationID, applicationID, schema.ApplicationID)
	}
	if version > schema.Version() {
		return fmt.Errorf("%w: got %d, supported %d", ErrSchemaTooNew, version, schema.Version())
	}
	if version != schema.Version() {
		return fmt.Errorf("%w: version %d, migrations build %d", ErrSchemaInconsistent, version, schema.Version())
	}
	return nil
}

var migrationFileName = regexp.MustCompile(`^([0-9]{4})_([a-z0-9]+(?:_[a-z0-9]+)*)\.sql$`)

// LoadMigrations reads a store's migrations from dir in fsys: files named
// NNNN_name.sql, numbered from 0001 without gaps.
func LoadMigrations(fsys fs.FS, dir string) ([]Migration, error) {
	entries, err := fs.ReadDir(fsys, dir)
	if err != nil {
		return nil, fmt.Errorf("read migrations: %w", err)
	}
	migrations := make([]Migration, 0, len(entries))
	for _, entry := range entries {
		match := migrationFileName.FindStringSubmatch(entry.Name())
		if entry.IsDir() || match == nil {
			return nil, fmt.Errorf("migration %q is not named NNNN_name.sql", entry.Name())
		}
		version, _ := strconv.Atoi(match[1])
		if version != len(migrations)+1 {
			return nil, fmt.Errorf("migration %q is out of sequence: want version %d", entry.Name(), len(migrations)+1)
		}
		content, err := fs.ReadFile(fsys, path.Join(dir, entry.Name()))
		if err != nil {
			return nil, fmt.Errorf("read migration %q: %w", entry.Name(), err)
		}
		migrations = append(migrations, Migration{Version: version, Name: match[2], SQL: string(content)})
	}
	if len(migrations) == 0 {
		return nil, errors.New("read migrations: none found")
	}
	return migrations, nil
}

// historyTable records every applied migration. Its presence also tells a
// database built by migrations from one written before the migration
// baseline, which may carry the same application ID.
const historyTable = `CREATE TABLE migration_history (
    version INTEGER PRIMARY KEY CHECK (version > 0),
    name TEXT NOT NULL CHECK (name <> ''),
    sha256 TEXT NOT NULL CHECK (length(sha256) = 64 AND sha256 NOT GLOB '*[^0-9a-f]*'),
    applied_at TEXT NOT NULL CHECK (applied_at <> '')
) STRICT`

func migrationDigest(migration Migration) string {
	sum := sha256.Sum256([]byte(migration.SQL))
	return hex.EncodeToString(sum[:])
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
	db, err := openLocked(ctx, absPath, schema, dataLock)
	if err != nil {
		return nil, err
	}
	lockOwned = false
	return db, nil
}

// OpenBeside opens (or creates) another database of the same service, named
// name in owner's data directory, under owner's directory lock. It is
// initialized and verified as Open does; close it before owner.
func OpenBeside(ctx context.Context, owner *DB, name string, schema Schema) (*DB, error) {
	if owner == nil || owner.DB == nil {
		return nil, errors.New("SQLite database is not open")
	}
	if name == "" || filepath.Base(name) != name {
		return nil, fmt.Errorf("SQLite database name %q must be a file name", name)
	}
	absPath, err := validatePath(filepath.Join(filepath.Dir(owner.path), name))
	if err != nil {
		return nil, err
	}
	return openLocked(ctx, absPath, schema, nil)
}

// openLocked opens the database at absPath once its directory lock, if any,
// is held. On success the handle owns dataLock; on failure the caller keeps it.
func openLocked(ctx context.Context, absPath string, schema Schema, dataLock *os.File) (*DB, error) {
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
	if err := db.migrate(ctx); err != nil {
		return nil, err
	}
	if _, err := db.Info(ctx); err != nil {
		return nil, err
	}
	if err := os.Chmod(absPath, 0o600); err != nil {
		return nil, fmt.Errorf("secure SQLite database permissions: %w", err)
	}
	ok = true
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

// migrate brings the database to the schema's latest version in one
// transaction. An empty database receives the history table, the application
// ID and every migration; a database of this application receives the
// migrations it has not recorded. Unidentified, foreign and pre-baseline
// databases, databases newer than the binary and recorded migrations that
// differ from the binary's are refused without changing anything.
func (db *DB) migrate(ctx context.Context) error {
	return db.WithTx(ctx, func(tx *sql.Tx) error {
		applicationID, err := pragmaInt(ctx, tx, "application_id")
		if err != nil {
			return err
		}
		version, err := pragmaInt(ctx, tx, "user_version")
		if err != nil {
			return err
		}
		switch {
		case applicationID == 0:
			hasObjects, err := hasUserSchemaObjects(ctx, tx)
			if err != nil {
				return err
			}
			if version != 0 || hasObjects {
				return fmt.Errorf("%w: refusing to adopt an unidentified non-empty database", ErrUnexpectedApplicationID)
			}
			if _, err := tx.ExecContext(ctx, historyTable); err != nil {
				return fmt.Errorf("create SQLite migration history: %w", err)
			}
			if _, err := tx.ExecContext(ctx, fmt.Sprintf("PRAGMA application_id = %d", db.schema.ApplicationID)); err != nil {
				return fmt.Errorf("set SQLite application id: %w", err)
			}
		case applicationID != db.schema.ApplicationID:
			return fmt.Errorf("%w: got %#x, want %#x", ErrUnexpectedApplicationID, applicationID, db.schema.ApplicationID)
		default:
			var tracked int
			if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM sqlite_schema WHERE type = 'table' AND name = 'migration_history'`).Scan(&tracked); err != nil {
				return fmt.Errorf("inspect SQLite migration history: %w", err)
			}
			if tracked == 0 {
				return fmt.Errorf("%w: schema version %d predates the migration baseline", ErrSchemaUnsupported, version)
			}
		}
		applied, err := appliedMigrations(ctx, tx)
		if err != nil {
			return err
		}
		if len(applied) > db.schema.Version() {
			return fmt.Errorf("%w: got %d, supported %d", ErrSchemaTooNew, len(applied), db.schema.Version())
		}
		for index, record := range applied {
			want := db.schema.Migrations[index]
			if record.Version != want.Version || record.Name != want.Name || record.SHA256 != migrationDigest(want) {
				return fmt.Errorf("%w: recorded migration %04d_%s differs from this binary's %04d_%s", ErrSchemaInconsistent, record.Version, record.Name, want.Version, want.Name)
			}
		}
		if version != len(applied) {
			return fmt.Errorf("%w: version %d with %d recorded migrations", ErrSchemaInconsistent, version, len(applied))
		}
		for _, migration := range db.schema.Migrations[len(applied):] {
			if _, err := tx.ExecContext(ctx, migration.SQL); err != nil {
				return fmt.Errorf("apply migration %04d_%s: %w", migration.Version, migration.Name, err)
			}
			if _, err := tx.ExecContext(ctx, `INSERT INTO migration_history(version, name, sha256, applied_at) VALUES (?, ?, ?, ?)`,
				migration.Version, migration.Name, migrationDigest(migration), time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
				return fmt.Errorf("record migration %04d_%s: %w", migration.Version, migration.Name, err)
			}
		}
		if len(applied) < db.schema.Version() {
			if _, err := tx.ExecContext(ctx, fmt.Sprintf("PRAGMA user_version = %d", db.schema.Version())); err != nil {
				return fmt.Errorf("set SQLite schema version: %w", err)
			}
		}
		return nil
	})
}

type appliedMigration struct {
	Version int
	Name    string
	SHA256  string
}

func appliedMigrations(ctx context.Context, tx *sql.Tx) ([]appliedMigration, error) {
	rows, err := tx.QueryContext(ctx, `SELECT version, name, sha256 FROM migration_history ORDER BY version`)
	if err != nil {
		return nil, fmt.Errorf("read SQLite migration history: %w", err)
	}
	defer rows.Close()
	var applied []appliedMigration
	for rows.Next() {
		var record appliedMigration
		if err := rows.Scan(&record.Version, &record.Name, &record.SHA256); err != nil {
			return nil, fmt.Errorf("read SQLite migration history: %w", err)
		}
		applied = append(applied, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read SQLite migration history: %w", err)
	}
	return applied, nil
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
