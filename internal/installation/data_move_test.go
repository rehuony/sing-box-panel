// SPDX-License-Identifier: GPL-3.0-or-later

package installation

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rehuony/sing-box-panel/internal/panelprocess"
	"github.com/rehuony/sing-box-panel/internal/settings"
	"github.com/rehuony/sing-box-panel/internal/store"
)

func TestDataMovePreservesFixedCleanupHistory(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("user history fixture requires XDG_STATE_HOME")
	}
	for _, historyIn := range []string{"source", "target"} {
		t.Run(historyIn, func(t *testing.T) {
			path, source := fixture(t)
			target := filepath.Join(filepath.Dir(source), "moved-data")
			stateRoot := source
			if historyIn == "target" {
				stateRoot = target
			}
			t.Setenv("XDG_STATE_HOME", filepath.Join(stateRoot, "state"))
			history := DefaultHistoryPath()
			record := CleanupHistory{SettingsPath: filepath.Join(t.TempDir(), "other.json"), Scope: "user", Outcome: "completed"}
			if err := WriteCleanupHistory(t.Context(), history, record); err != nil {
				t.Fatal(err)
			}
			before, err := os.ReadFile(history)
			if err != nil {
				t.Fatal(err)
			}
			if err := settings.RememberDataLocation(path, source); err != nil {
				t.Fatal(err)
			}
			selectMovedDataDir(t, path, target)
			if _, err := PrepareDataLocation(t.Context(), path); err == nil || !strings.Contains(err.Error(), "cleanup history") {
				t.Fatalf("migration did not protect history: %v", err)
			}
			after, err := os.ReadFile(history)
			if err != nil || !bytes.Equal(before, after) {
				t.Fatalf("independent history moved or changed: %v", err)
			}
			location, err := settings.ReadDataLocation(path)
			if err != nil || location.DataDir != source || location.Move != nil {
				t.Fatalf("migration started despite retained history: %+v %v", location, err)
			}
			if _, err := os.Stat(filepath.Join(source, "panel.db")); err != nil {
				t.Fatal("source database was removed")
			}
			if _, err := os.Lstat(filepath.Join(target, "panel.db")); !os.IsNotExist(err) {
				t.Fatal("migration copied data before validation")
			}
		})
	}
}

func selectMovedDataDir(t *testing.T, path, target string) {
	t.Helper()
	value, err := settings.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	value.DataDir = target
	raw, _ := json.Marshal(value)
	if err := settings.Replace(path, raw); err != nil {
		t.Fatal(err)
	}
}

func TestDataDirectoryMovePreservesDataAndRebasesReferences(t *testing.T) {
	path, source := fixture(t)
	target := filepath.Join(filepath.Dir(source), "moved-data")
	for _, name := range []string{"artifacts/core/sing-box", "imports/core-upload-test", "logs/core/example.log", "runtime/configs/current.json"} {
		putFile(t, filepath.Join(source, name))
	}
	if err := os.Chmod(filepath.Join(source, "artifacts/core/sing-box"), 0700); err != nil {
		t.Fatal(err)
	}
	db, err := store.Open(t.Context(), filepath.Join(source, "panel.db"))
	if err != nil {
		t.Fatal(err)
	}
	err = db.WithTx(t.Context(), func(tx *sql.Tx) error {
		_, err := tx.ExecContext(t.Context(), `INSERT INTO core_artifacts(id,exact_version,operating_system,architecture,source_kind,user_source,archive_sha256,binary_sha256,binary_path,reported_version,created_at)
		VALUES('core','1.14.0','linux','arm64','user_verified','fixture',?,?,?,'1.14.0','2026-09-19T00:00:00Z')`, strings.Repeat("a", 64), strings.Repeat("b", 64), filepath.Join(source, "artifacts/core/sing-box"))
		if err != nil {
			return err
		}

		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	selectMovedDataDir(t, path, target)
	if active, err := settings.LoadDataDir(path); err != nil || active != source {
		t.Fatalf("stop/status lost old instance: %s %v", active, err)
	}
	if _, err := PrepareDataLocation(t.Context(), path); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(source); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("old data remains: %v", err)
	}
	if active, err := settings.LoadDataDir(path); err != nil || active != target {
		t.Fatalf("location did not commit: %s %v", active, err)
	}
	for _, name := range []string{"artifacts/core/sing-box", "imports/core-upload-test", "logs/core/example.log", "runtime/configs/current.json"} {
		raw, err := os.ReadFile(filepath.Join(target, name))
		if err != nil || string(raw) != "fixture" {
			t.Fatalf("lost %s: %v", name, err)
		}
	}
	db, err = store.Open(t.Context(), filepath.Join(target, "panel.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	physicalTarget, err := filepath.EvalSymlinks(target)
	if err != nil {
		t.Fatal(err)
	}
	core, err := db.GetCoreArtifact(t.Context(), "core")
	if err != nil || core.BinaryPath != filepath.Join(physicalTarget, "artifacts/core/sing-box") {
		t.Fatalf("core path: %s %v", core.BinaryPath, err)
	}

}

func TestDataMoveRefusesBusyOrConflictingStorage(t *testing.T) {
	for _, scenario := range []string{"panel", "database", "nonempty", "nested", "database symlink"} {
		t.Run(scenario, func(t *testing.T) {
			path, source := fixture(t)
			target := filepath.Join(filepath.Dir(source), "target")
			switch scenario {
			case "panel":
				lease, err := panelprocess.AcquireLease(source)
				if err != nil {
					t.Fatal(err)
				}
				defer lease.Close()
			case "database":
				db, err := store.Open(t.Context(), filepath.Join(source, "panel.db"))
				if err != nil {
					t.Fatal(err)
				}
				defer db.Close()
			case "nonempty":
				putFile(t, filepath.Join(target, "unrelated"))
			case "nested":
				target = filepath.Join(source, "target")
			case "database symlink":
				original := filepath.Join(source, "panel.db")
				external := filepath.Join(filepath.Dir(source), "external.db")
				if err := os.Rename(original, external); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(external, original); err != nil {
					t.Fatal(err)
				}
			}
			selectMovedDataDir(t, path, target)
			if _, err := PrepareDataLocation(t.Context(), path); err == nil {
				t.Fatal("unsafe migration accepted")
			}
			if _, err := os.Stat(filepath.Join(source, "panel.db")); err != nil {
				t.Fatal("source lost", err)
			}
			location, err := settings.ReadDataLocation(path)
			if err != nil || location.Move != nil || location.DataDir != source {
				t.Fatalf("failed preflight changed location: %+v %v", location, err)
			}
		})
	}
}

func TestDataMoveRecoversInterruptedCopyAndCleanup(t *testing.T) {
	for _, phase := range []string{"copy", "cleanup", "source removed", "destination damaged"} {
		t.Run(phase, func(t *testing.T) {
			path, source := fixture(t)
			target := filepath.Join(filepath.Dir(source), "target")
			putFile(t, filepath.Join(source, "notes.txt"))
			selectMovedDataDir(t, path, target)
			location, _ := settings.ReadDataLocation(path)
			location.Move = &settings.DataMove{ID: "interrupted-move", Target: target, Ready: phase != "copy"}
			if err := settings.WriteDataLocation(path, location); err != nil {
				t.Fatal(err)
			}
			if err := os.Mkdir(target, 0700); err != nil {
				t.Fatal(err)
			}
			owner := dataMoveOwner{ID: location.Move.ID, SettingsPath: path, Source: source, Target: target}
			if err := writeDataMoveOwner(source, owner); err != nil {
				t.Fatal(err)
			}
			if err := writeDataMoveOwner(target, owner); err != nil {
				t.Fatal(err)
			}
			if phase == "copy" {
				putFile(t, filepath.Join(target, "panel.db"))
			} else {
				if err := copyDataMoveTree(t.Context(), source, target); err != nil {
					t.Fatal(err)
				}
				db, err := store.OpenForMigration(t.Context(), filepath.Join(source, "panel.db"))
				if err != nil {
					t.Fatal(err)
				}
				if err := db.SnapshotDataFile(t.Context(), filepath.Join(target, "panel.db")); err != nil {
					t.Fatal(err)
				}
				db.Close()
				moved, err := store.OpenForMigration(t.Context(), filepath.Join(target, "panel.db"))
				if err != nil {
					t.Fatal(err)
				}
				if err := moved.RebaseDataPaths(t.Context(), source, target, owner.ID); err != nil {
					t.Fatal(err)
				}
				moved.Close()
				if phase == "source removed" {
					if err := os.RemoveAll(source); err != nil {
						t.Fatal(err)
					}
				}
				if phase == "destination damaged" {
					if err := os.WriteFile(filepath.Join(target, "notes.txt"), []byte("corrupt"), 0600); err != nil {
						t.Fatal(err)
					}
				}
			}
			if db, err := store.Open(t.Context(), filepath.Join(target, "panel.db")); err == nil {
				db.Close()
				t.Fatal("incomplete target opened")
			}
			_, err := PrepareDataLocation(t.Context(), path)
			if phase == "destination damaged" {
				if err == nil {
					t.Fatal("damaged destination accepted")
				}
				raw, readErr := os.ReadFile(filepath.Join(source, "notes.txt"))
				if readErr != nil || string(raw) != "fixture" {
					t.Fatal("original data lost", readErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if _, err := os.Stat(source); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("source retained: %v", err)
			}
			db, err := store.Open(t.Context(), filepath.Join(target, "panel.db"))
			if err != nil {
				t.Fatal(err)
			}
			db.Close()
		})
	}
}

func TestMissingEstablishedDataIsNotReplacedByEmptyInstance(t *testing.T) {
	path, source := fixture(t)
	selectMovedDataDir(t, path, filepath.Join(filepath.Dir(source), "target"))
	if err := os.RemoveAll(source); err != nil {
		t.Fatal(err)
	}
	if _, err := PrepareDataLocation(t.Context(), path); err == nil {
		t.Fatal("lost original silently replaced by an empty instance")
	}
}

func TestSettingsRecoveryPrecedesDataDirectoryMove(t *testing.T) {
	path, source := fixture(t)
	target := filepath.Join(filepath.Dir(source), "target")
	selectMovedDataDir(t, path, target)
	before, err := settings.Read(path)
	if err != nil {
		t.Fatal(err)
	}
	value, err := settings.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	value.Panel.Language = "en"
	after, _ := json.Marshal(value)
	journal, _ := json.Marshal(map[string]any{"id": "pending-settings", "before": before, "after": after})
	if err := settings.WriteAtomic(path+".pending", journal); err != nil {
		t.Fatal(err)
	}
	db, err := store.Open(t.Context(), filepath.Join(source, "panel.db"))
	if err != nil {
		t.Fatal(err)
	}
	if err := db.CommitPanelSettingsFile(t.Context(), path, "pending-settings", nil, func() error { return settings.ReplaceLocked(path, after) }); err != nil {
		t.Fatal(err)
	}
	db.Close()
	if _, err := PrepareDataLocation(t.Context(), path); err != nil {
		t.Fatal(err)
	}
	loaded, err := settings.Load(path)
	if err != nil || loaded.Panel.Language != "en" || loaded.DataDir != target {
		t.Fatalf("settings recovery/move diverged: %+v %v", loaded, err)
	}
}

func TestRolledBackSettingsDoNotMoveData(t *testing.T) {
	path, source := fixture(t)
	if _, err := PrepareDataLocation(t.Context(), path); err != nil {
		t.Fatal(err)
	}
	before, _ := settings.Read(path)
	target := filepath.Join(filepath.Dir(source), "target")
	selectMovedDataDir(t, path, target)
	after, _ := settings.Read(path)
	journal, _ := json.Marshal(map[string]any{"id": "uncommitted", "before": before, "after": after})
	if err := settings.WriteAtomic(path+".pending", journal); err != nil {
		t.Fatal(err)
	}
	value, err := PrepareDataLocation(t.Context(), path)
	if err != nil || value.DataDir != source {
		t.Fatalf("rollback attempted a migration: %s %v", value.DataDir, err)
	}
	if _, err := os.Stat(target); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("rolled-back destination was created: %v", err)
	}
}

func TestDataLocationCanChangeBeforeDatabaseInitialization(t *testing.T) {
	path, source := fixture(t)
	if err := os.RemoveAll(source); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(source, 0700); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(filepath.Dir(source), "target")
	selectMovedDataDir(t, path, target)
	value, err := PrepareDataLocation(t.Context(), path)
	if err != nil || value.DataDir != target {
		t.Fatalf("unused location could not change: %s %v", value.DataDir, err)
	}
}
