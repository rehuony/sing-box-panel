// SPDX-License-Identifier: GPL-3.0-or-later

package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"

	"modernc.org/sqlite"
)

// ErrSnapshotNeedsRecovery is returned for a database whose WAL lacks its
// shared memory: only its owner may recover it.
var ErrSnapshotNeedsRecovery = errors.New("database needs recovery by its owner before a snapshot; start the service once")

// Snapshot writes a consistent copy of the database at source to target,
// which must not exist, as a database in rollback-journal mode. While the
// source has a WAL, as while its owner runs, it copies through a read-only
// connection, which includes what the WAL commits. Without a WAL the file is
// the whole database and is copied as it is; the caller keeps every owner
// out meanwhile, such as by holding the service's lease. A WAL without its
// shared memory is refused: reading it would create files its owner might
// not be able to open.
func Snapshot(ctx context.Context, source, target string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	source, err := filepath.Abs(source)
	if err != nil {
		return err
	}
	if _, err := os.Lstat(target); err == nil {
		return fmt.Errorf("snapshot target %s exists", target)
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	_, walErr := os.Lstat(source + "-wal")
	switch {
	case errors.Is(walErr, os.ErrNotExist):
		if err := copyFile(source, target); err != nil {
			return err
		}
	case walErr != nil:
		return walErr
	default:
		if _, err := os.Lstat(source + "-shm"); err != nil {
			return fmt.Errorf("%w: %v", ErrSnapshotNeedsRecovery, err)
		}
		if err := vacuumInto(ctx, source, target); err != nil {
			return err
		}
	}
	if err := os.Chmod(target, 0o600); err != nil {
		return err
	}
	db, err := OpenFile(target, false)
	if err != nil {
		return err
	}
	_, err = db.ExecContext(ctx, "PRAGMA journal_mode = DELETE")
	return errors.Join(err, db.Close())
}

// OpenFile opens a single connection to the SQLite file at path, such as a
// snapshot, without the locking, migration or WAL of a service's database.
// A read-only open never creates the file.
func OpenFile(path string, readOnly bool) (*sql.DB, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	location := url.URL{Scheme: "file", Path: filepath.ToSlash(abs)}
	query := location.Query()
	query.Set("mode", "rw")
	if readOnly {
		query.Set("mode", "ro")
	}
	query.Set("_busy_timeout", fmt.Sprint(BusyTimeoutMillis))
	query.Set("_foreign_keys", "on")
	query.Add("_pragma", "trusted_schema(OFF)")
	location.RawQuery = query.Encode()
	connector, err := sqlite.NewConnector(location.String())
	if err != nil {
		return nil, err
	}
	db := sql.OpenDB(connector)
	db.SetMaxOpenConns(1)
	return db, nil
}

func vacuumInto(ctx context.Context, source, target string) error {
	db, err := OpenFile(source, true)
	if err != nil {
		return err
	}
	_, err = db.ExecContext(ctx, "VACUUM INTO ?", target)
	return errors.Join(err, db.Close())
}

func copyFile(source, target string) (err error) {
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()
	output, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	defer func() {
		err = errors.Join(err, output.Close())
	}()
	if _, err := io.Copy(output, input); err != nil {
		return err
	}
	return output.Sync()
}

// Verify checks that the database behind db, such as a snapshot to restore,
// can become this schema: it is intact, of this application, no newer than
// the binary, and every migration it records is the binary's. It returns the
// database's schema version.
func (schema Schema) Verify(ctx context.Context, db *sql.DB) (int, error) {
	var integrity string
	if err := db.QueryRowContext(ctx, "PRAGMA integrity_check").Scan(&integrity); err != nil {
		return 0, fmt.Errorf("check SQLite integrity: %w", err)
	}
	if integrity != "ok" {
		return 0, errors.New("SQLite integrity check failed")
	}
	applicationID, err := pragmaInt(ctx, db, "application_id")
	if err != nil {
		return 0, err
	}
	if applicationID != schema.ApplicationID {
		return 0, fmt.Errorf("%w: got %#x, want %#x", ErrUnexpectedApplicationID, applicationID, schema.ApplicationID)
	}
	version, err := pragmaInt(ctx, db, "user_version")
	if err != nil {
		return 0, err
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()
	applied, err := appliedMigrations(ctx, tx)
	if err != nil {
		return 0, err
	}
	if len(applied) > schema.Version() {
		return 0, fmt.Errorf("%w: got %d, supported %d", ErrSchemaTooNew, len(applied), schema.Version())
	}
	for index, record := range applied {
		want := schema.Migrations[index]
		if record.Version != want.Version || record.Name != want.Name || record.SHA256 != migrationDigest(want) {
			return 0, fmt.Errorf("%w: recorded migration %04d_%s differs from this binary's %04d_%s", ErrSchemaInconsistent, record.Version, record.Name, want.Version, want.Name)
		}
	}
	if version == 0 || version != len(applied) {
		return 0, fmt.Errorf("%w: version %d with %d recorded migrations", ErrSchemaInconsistent, version, len(applied))
	}
	return version, nil
}
