package store

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestCoreArtifactLifecycleMigrationPreservesIdentitiesAndRuntimeReferences(t *testing.T) {
	for _, state := range []string{"verified", "quarantined", "revoked"} {
		t.Run(state, func(t *testing.T) {
			ctx := testContext(t)
			path := filepath.Join(t.TempDir(), "panel.db")
			legacy, err := sql.Open("sqlite", path)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = legacy.Close() })
			migrations, err := loadMigrations()
			if err != nil {
				t.Fatal(err)
			}
			for _, migration := range migrations {
				if migration.version > 7 {
					break
				}
				if _, err := legacy.ExecContext(ctx, migration.sql); err != nil {
					t.Fatal(err)
				}
				if _, err := legacy.ExecContext(ctx, `INSERT INTO schema_migrations VALUES (?, ?, ?)`, migration.version, migration.name, "2026-09-01T00:00:00Z"); err != nil {
					t.Fatal(err)
				}
			}
			for _, statement := range []string{fmt.Sprintf("PRAGMA application_id = %d", ApplicationID), "PRAGMA user_version = 7", "PRAGMA foreign_keys = ON"} {
				if _, err := legacy.ExecContext(ctx, statement); err != nil {
					t.Fatal(err)
				}
			}
			before := &Store{db: legacy}
			now := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
			bundle := seedTrafficActivationBundle(t, ctx, before, now)
			startup, err := before.GetStartupArtifact(ctx, bundle.StartupArtifactID)
			if err != nil {
				t.Fatal(err)
			}
			core, err := before.GetCoreArtifact(ctx, startup.CoreArtifactID)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := legacy.ExecContext(ctx, `UPDATE core_artifacts SET verification_state = ? WHERE id = ?`, state, core.ID); err != nil {
				t.Fatal(err)
			}
			if err := legacy.Close(); err != nil {
				t.Fatal(err)
			}

			upgraded, err := Open(ctx, path)
			if err != nil {
				t.Fatal(err)
			}
			defer upgraded.Close()
			stored, err := upgraded.GetCoreArtifact(ctx, core.ID)
			if err != nil || !reflect.DeepEqual(stored, core) {
				t.Fatalf("core identity changed during migration: %+v, %v", stored, err)
			}
			if stored, err := upgraded.GetActivationBundle(ctx, bundle.ID); err != nil || stored != bundle {
				t.Fatalf("activation reference changed: %+v, %v", stored, err)
			}
			if err := upgraded.WithTx(ctx, func(tx *sql.Tx) error { return validateRunnableBundle(ctx, tx, bundle.ID) }); err != nil {
				t.Fatalf("legacy lifecycle still blocks runtime: %v", err)
			}
			if retried, err := upgraded.UpsertCoreArtifact(ctx, core); err != nil || !reflect.DeepEqual(retried, core) {
				t.Fatalf("reinstall changed identity: %+v, %v", retried, err)
			}
			var columns int
			if err := upgraded.db.QueryRowContext(ctx, `SELECT count(*) FROM pragma_table_info('core_artifacts') WHERE name = 'verification_state'`).Scan(&columns); err != nil || columns != 0 {
				t.Fatalf("obsolete lifecycle column remains: %d, %v", columns, err)
			}
			bootstrap, err := upgraded.Bootstrap(ctx)
			if err != nil || bootstrap.Hub.DesiredRunning || bootstrap.Hub.TargetGeneration != 0 {
				t.Fatalf("migration changed desired runtime: %+v, %v", bootstrap.Hub, err)
			}
		})
	}
}
