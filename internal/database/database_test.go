// SPDX-License-Identifier: GPL-3.0-or-later
package database

import (
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"testing"
	"testing/fstest"
)

const testApplicationID = 0x54455354

func testSchema(t *testing.T, files fstest.MapFS) Schema {
	t.Helper()
	migrations, err := LoadMigrations(files, "migrations")
	if err != nil {
		t.Fatal(err)
	}
	return Schema{ApplicationID: testApplicationID, Migrations: migrations}
}

var (
	baseline = &fstest.MapFile{Data: []byte(`CREATE TABLE items (id TEXT PRIMARY KEY) STRICT; INSERT INTO items VALUES ('seed');`)}
	addition = &fstest.MapFile{Data: []byte(`CREATE TABLE notes (item_id TEXT NOT NULL REFERENCES items(id)) STRICT; INSERT INTO notes SELECT id FROM items;`)}
)

func openTest(t *testing.T, path string, schema Schema) (*DB, error) {
	t.Helper()
	db, err := Open(t.Context(), path, schema, false)
	if err == nil {
		t.Cleanup(func() { _ = db.Close() })
	}
	return db, err
}

func count(t *testing.T, db *sql.DB, query string) int {
	t.Helper()
	var n int
	if err := db.QueryRowContext(t.Context(), query).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestMigrationsApplyInOrderOnceAndOnlyWhenPending(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")
	first := testSchema(t, fstest.MapFS{"migrations/0001_baseline.sql": baseline})
	db, err := openTest(t, path, first)
	if err != nil {
		t.Fatal(err)
	}
	if info, err := db.Info(t.Context()); err != nil || info != (Info{ApplicationID: testApplicationID, Version: 1}) {
		t.Fatalf("Info() = %+v, %v", info, err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	second := testSchema(t, fstest.MapFS{"migrations/0001_baseline.sql": baseline, "migrations/0002_notes.sql": addition})
	for range 2 {
		db, err = openTest(t, path, second)
		if err != nil {
			t.Fatal(err)
		}
		if info, err := db.Info(t.Context()); err != nil || info.Version != 2 {
			t.Fatalf("Info() = %+v, %v", info, err)
		}
		// The baseline's row is not inserted again, and the addition ran once.
		if items, notes := count(t, db.DB, `SELECT count(*) FROM items`), count(t, db.DB, `SELECT count(*) FROM notes`); items != 1 || notes != 1 {
			t.Fatalf("items %d, notes %d", items, notes)
		}
		if recorded := count(t, db.DB, `SELECT count(*) FROM migration_history`); recorded != 2 {
			t.Fatalf("recorded migrations: %d", recorded)
		}
		if err := db.Close(); err != nil {
			t.Fatal(err)
		}
	}
}

func TestMigrateRefusesWithoutChangingTheDatabase(t *testing.T) {
	first := testSchema(t, fstest.MapFS{"migrations/0001_baseline.sql": baseline})
	second := testSchema(t, fstest.MapFS{"migrations/0001_baseline.sql": baseline, "migrations/0002_notes.sql": addition})
	edited := testSchema(t, fstest.MapFS{"migrations/0001_baseline.sql": {Data: []byte(`CREATE TABLE items (id TEXT) STRICT;`)}})
	failing := testSchema(t, fstest.MapFS{"migrations/0001_baseline.sql": baseline, "migrations/0002_broken.sql": {Data: []byte(`CREATE TABLE later (id TEXT) STRICT; SELECT missing FROM nowhere;`)}})
	raw := func(t *testing.T, path, statements string) {
		t.Helper()
		db, err := sql.Open("sqlite", path)
		if err != nil {
			t.Fatal(err)
		}
		defer db.Close()
		if _, err := db.ExecContext(t.Context(), statements); err != nil {
			t.Fatal(err)
		}
	}
	built := func(schema Schema) func(t *testing.T, path string) {
		return func(t *testing.T, path string) {
			db, err := openTest(t, path, schema)
			if err != nil {
				t.Fatal(err)
			}
			if err := db.Close(); err != nil {
				t.Fatal(err)
			}
		}
	}
	cases := []struct {
		name    string
		prepare func(t *testing.T, path string)
		open    Schema
		want    error
	}{
		{"unidentified", func(t *testing.T, path string) { raw(t, path, `CREATE TABLE kept (value TEXT)`) }, first, ErrUnexpectedApplicationID},
		{"foreign", func(t *testing.T, path string) {
			raw(t, path, `PRAGMA application_id = 123; CREATE TABLE kept (value TEXT)`)
		}, first, ErrUnexpectedApplicationID},
		{"pre-baseline", func(t *testing.T, path string) {
			raw(t, path, fmt.Sprintf(`PRAGMA application_id = %d; PRAGMA user_version = 27; CREATE TABLE kept (value TEXT)`, testApplicationID))
		}, first, ErrSchemaUnsupported},
		{"newer", built(second), first, ErrSchemaTooNew},
		{"edited", built(first), edited, ErrSchemaInconsistent},
		{"failing", built(first), failing, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "test.db")
			tc.prepare(t, path)
			before := snapshot(t, path)
			db, err := openTest(t, path, tc.open)
			if db != nil || err == nil {
				t.Fatal("refused database opened")
			}
			if tc.want != nil && !errors.Is(err, tc.want) {
				t.Fatalf("Open: %v, want %v", err, tc.want)
			}
			if after := snapshot(t, path); after != before {
				t.Fatalf("database changed:\n%s\n%s", before, after)
			}
		})
	}
}

// snapshot is a database's identity and objects, read without opening it
// through the package.
func snapshot(t *testing.T, path string) string {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var applicationID, version int
	var objects string
	if err := db.QueryRowContext(t.Context(), `SELECT (SELECT application_id FROM pragma_application_id), (SELECT user_version FROM pragma_user_version),
		(SELECT group_concat(name, ',') FROM (SELECT name FROM sqlite_schema ORDER BY name))`).Scan(&applicationID, &version, &objects); err != nil {
		t.Fatal(err)
	}
	return fmt.Sprintf("application %#x, version %d, objects %s", applicationID, version, objects)
}

func TestLoadMigrationsRequiresNumberedNamesWithoutGaps(t *testing.T) {
	for name, files := range map[string]fstest.MapFS{
		"empty":   {"migrations": {Mode: fs.ModeDir}},
		"stray":   {"migrations/0001_baseline.sql": baseline, "migrations/README": {Data: nil}},
		"gap":     {"migrations/0001_baseline.sql": baseline, "migrations/0003_notes.sql": addition},
		"start":   {"migrations/0002_baseline.sql": baseline},
		"unnamed": {"migrations/0001.sql": baseline},
		"case":    {"migrations/0001_Baseline.sql": baseline},
	} {
		t.Run(name, func(t *testing.T) {
			if migrations, err := LoadMigrations(files, "migrations"); err == nil {
				t.Fatalf("LoadMigrations() = %d migrations, want an error", len(migrations))
			}
		})
	}
}
