package store

import (
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"testing"
)

func TestOpenRejectsUnsupportedFormatsWithoutConvertingData(t *testing.T) {
	cases := []struct {
		name        string
		id, version int
		want        error
	}{
		{"unidentified", 0, 0, ErrUnexpectedApplicationID},
		{"foreign", 123, CurrentSchemaVersion, ErrUnexpectedApplicationID},
		{"future", ApplicationID, CurrentSchemaVersion + 1, ErrSchemaTooNew},
	}
	for version := 0; version < 11; version++ {
		cases = append(cases, struct {
			name        string
			id, version int
			want        error
		}{fmt.Sprintf("old-%d", version), ApplicationID, version, ErrSchemaUnsupported})
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := t.Context()
			path := filepath.Join(t.TempDir(), "panel.db")
			db, err := sql.Open("sqlite", path)
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			_, err = db.ExecContext(ctx, fmt.Sprintf("PRAGMA application_id=%d; PRAGMA user_version=%d; CREATE TABLE preserved(value TEXT); INSERT INTO preserved VALUES('untouched');", tc.id, tc.version))
			if err != nil {
				t.Fatal(err)
			}
			opened, err := Open(ctx, path)
			if opened != nil {
				opened.Close()
				t.Fatal("unsupported database opened")
			}
			if !errors.Is(err, tc.want) {
				t.Fatalf("Open: %v, want %v", err, tc.want)
			}
			var content string
			if err := db.QueryRowContext(ctx, "SELECT value FROM preserved").Scan(&content); err != nil || content != "untouched" {
				t.Fatalf("data changed: %q %v", content, err)
			}
			var count int
			if err := db.QueryRowContext(ctx, "SELECT count(*) FROM sqlite_schema WHERE name NOT LIKE 'sqlite_%'").Scan(&count); err != nil || count != 1 {
				t.Fatalf("schema changed: %d %v", count, err)
			}
			assertPragmaInt(t, ctx, db, 0, "user_version", tc.version)
			assertPragmaInt(t, ctx, db, 0, "application_id", tc.id)
		})
	}
}

func TestFreshSchemaContainsOnlyCurrentStorage(t *testing.T) {
	db := openTestStore(t, t.Context())
	var obsolete int
	if err := db.db.QueryRowContext(t.Context(), `SELECT count(*) FROM sqlite_schema WHERE name IN ('schema_migrations', 'panel_settings', 'tasks')`).Scan(&obsolete); err != nil || obsolete != 0 {
		t.Fatalf("obsolete storage objects: %d %v", obsolete, err)
	}
	var violations int
	rows, err := db.db.QueryContext(t.Context(), "PRAGMA foreign_key_check")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		violations++
	}
	if err := rows.Err(); err != nil || violations != 0 {
		t.Fatalf("foreign key violations: %d %v", violations, err)
	}
}

func TestFailedVersion11UpgradeRollsBackSchemaAndVersion(t *testing.T) {
	path := filepath.Join(t.TempDir(), "panel.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	// An incomplete version-11 schema cannot supply the migration's source data.
	if _, err := db.ExecContext(t.Context(), fmt.Sprintf("PRAGMA application_id=%d; PRAGMA user_version=11; CREATE TABLE preserved(value TEXT); INSERT INTO preserved VALUES('untouched')", ApplicationID)); err != nil {
		t.Fatal(err)
	}
	opened, err := Open(t.Context(), path)
	if err == nil {
		opened.Close()
		t.Fatal("incomplete migration succeeded")
	}
	assertPragmaInt(t, t.Context(), db, 0, "user_version", 11)
	var count int
	if err := db.QueryRowContext(t.Context(), "SELECT count(*) FROM sqlite_schema WHERE name='traffic_months'").Scan(&count); err != nil || count != 0 {
		t.Fatalf("partial schema persisted: %d %v", count, err)
	}
	var value string
	if err := db.QueryRowContext(t.Context(), "SELECT value FROM preserved").Scan(&value); err != nil || value != "untouched" {
		t.Fatal("existing data changed", err)
	}
}
