package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
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

func TestRemoveExportBindingsMigration(t *testing.T) {
	for _, version := range []int{11, 12} {
		for _, fail := range []bool{false, true} {
			t.Run(fmt.Sprintf("version-%d/fail-%t", version, fail), func(t *testing.T) {
				ctx := t.Context()
				db := openTestStore(t, ctx)
				var before []SubscriptionChannel
				for _, id := range []string{"channel-first", "channel-second", "channel-unbound"} {
					channel, err := db.CreateSubscriptionChannel(ctx, SubscriptionChannel{
						ID: id, Name: id, Format: SubscriptionFormatMihomo, Enabled: id != "channel-second",
						Config: json.RawMessage(`{"exclude_tags":["private"],"exclude_types":["http"]}`),
					})
					if err != nil {
						t.Fatal(err)
					}
					before = append(before, channel)
					if id != "channel-unbound" {
						if _, err := db.db.ExecContext(ctx, `UPDATE subscription_channels SET config_json = json_set(config_json, '$.export_token_ids', json('["token-old"]')) WHERE id = ?`, id); err != nil {
							t.Fatal(err)
						}
					}
				}
				if version == 11 {
					if _, err := db.db.ExecContext(ctx, "DROP TABLE traffic_months"); err != nil {
						t.Fatal(err)
					}
				}
				if fail {
					if _, err := db.db.ExecContext(ctx, `CREATE TRIGGER reject_cleanup BEFORE UPDATE OF config_json ON subscription_channels
						WHEN OLD.id = 'channel-second' BEGIN SELECT RAISE(ABORT, 'reject cleanup'); END`); err != nil {
						t.Fatal(err)
					}
				}
				if _, err := db.db.ExecContext(ctx, fmt.Sprintf("PRAGMA user_version = %d", version)); err != nil {
					t.Fatal(err)
				}
				path := db.Path()
				if err := db.Close(); err != nil {
					t.Fatal(err)
				}
				upgraded, err := Open(ctx, path)
				if fail {
					if err == nil {
						upgraded.Close()
						t.Fatal("failed cleanup was accepted")
					}
					raw, err := sql.Open("sqlite", path)
					if err != nil {
						t.Fatal(err)
					}
					defer raw.Close()
					assertPragmaInt(t, ctx, raw, 0, "user_version", version)
					var count int
					if err := raw.QueryRowContext(ctx, `SELECT count(*) FROM subscription_channels WHERE json_type(config_json, '$.export_token_ids') IS NOT NULL`).Scan(&count); err != nil || count != 2 {
						t.Fatalf("partial cleanup persisted: %d %v", count, err)
					}
					if version == 11 {
						if err := raw.QueryRowContext(ctx, `SELECT count(*) FROM sqlite_schema WHERE name = 'traffic_months'`).Scan(&count); err != nil || count != 0 {
							t.Fatal("earlier migration was not rolled back", count, err)
						}
					}
					return
				}
				if err != nil {
					t.Fatal(err)
				}
				defer upgraded.Close()
				// A second initialization must not change the upgraded data.
				for range 2 {
					for _, want := range before {
						got, err := upgraded.GetSubscriptionChannel(ctx, want.ID)
						if err != nil || !reflect.DeepEqual(got, want) {
							t.Fatalf("channel changed beyond bindings: got %+v want %+v error %v", got, want, err)
						}
					}
					if info, err := upgraded.SchemaInfo(ctx); err != nil || info.Version != 13 {
						t.Fatal(info, err)
					}
					if err := upgraded.initializeSchema(ctx); err != nil {
						t.Fatal(err)
					}
				}
			})
		}
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
