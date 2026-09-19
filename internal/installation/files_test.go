// SPDX-License-Identifier: GPL-3.0-or-later

package installation

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/rehuony/sing-box-panel/internal/panelprocess"
	"github.com/rehuony/sing-box-panel/internal/settings"
	"github.com/rehuony/sing-box-panel/internal/store"
)

func fixture(t *testing.T) (string, string) {
	t.Helper()
	dir := t.TempDir()
	dataDir := filepath.Join(dir, "data")
	if err := os.Mkdir(dataDir, 0o700); err != nil {
		t.Fatal(err)
	}
	value := settings.Defaults()
	value.DataDir = dataDir
	value.Auth.Token = strings.Repeat("test", 8)
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "setting.json")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	db, err := store.Open(context.Background(), filepath.Join(dataDir, "panel.db"))
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	return path, dataDir
}

func putFile(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("fixture"), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestInspectAndCleanKeepUnrelatedFiles(t *testing.T) {
	path, dir := fixture(t)
	for _, name := range []string{"artifacts/sha256/core/sing-box", "runtime/configs/current.json", "logs/core/2026-09-19-000.log", "imports/core-upload-test", "notes.txt", "logs/operator-notes.txt"} {
		putFile(t, filepath.Join(dir, name))
	}
	outside := filepath.Join(filepath.Dir(dir), "original-core.tar.gz")
	putFile(t, outside)
	before, err := os.ReadFile(filepath.Join(dir, "panel.db"))
	if err != nil {
		t.Fatal(err)
	}
	report, err := Inspect(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	if report.DatabaseIdentity != "sing-box-panel" {
		t.Fatalf("identity=%s", report.DatabaseIdentity)
	}
	seen := map[string]bool{}
	for _, entry := range report.Entries {
		seen[entry.Path] = true
	}
	if !seen[filepath.Join(dir, "logs/core/2026-09-19-000.log")] || !seen[filepath.Join(dir, "notes.txt")] {
		t.Fatalf("incomplete inventory=%+v", report.Entries)
	}
	after, err := os.ReadFile(filepath.Join(dir, "panel.db"))
	if err != nil || string(before) != string(after) {
		t.Fatal("inspection modified database")
	}
	if _, err := os.Stat(filepath.Join(dir, panelprocess.LeaseFileName)); !os.IsNotExist(err) {
		t.Fatalf("inspection created lock: %v", err)
	}
	result, err := Clean(context.Background(), report)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Removed) == 0 {
		t.Fatal("cleanup removed nothing")
	}
	for _, name := range []string{path, filepath.Join(dir, "panel.db"), filepath.Join(dir, "artifacts"), filepath.Join(dir, "runtime"), filepath.Join(dir, "imports"), filepath.Join(dir, "logs/core"), filepath.Join(dir, panelprocess.LeaseFileName)} {
		if _, err := os.Lstat(name); !os.IsNotExist(err) {
			t.Fatalf("managed path remains: %s %v", name, err)
		}
	}
	for _, name := range []string{outside, filepath.Join(dir, "notes.txt"), filepath.Join(dir, "logs/operator-notes.txt")} {
		if data, err := os.ReadFile(name); err != nil || string(data) != "fixture" {
			t.Fatalf("unrelated file changed: %s %v", name, err)
		}
	}
}

func TestCleanupRefusesRunningOwnerAndForeignDatabase(t *testing.T) {
	path, dir := fixture(t)
	report, err := Inspect(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	lease, err := panelprocess.AcquireLease(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Clean(context.Background(), report); err == nil {
		t.Fatal("cleaned a running instance")
	}
	_ = lease.Close()
	if _, err := os.Stat(path); err != nil {
		t.Fatal("failed cleanup removed settings")
	}
	putFile(t, filepath.Join(dir, "panel.db"))
	report, err = Inspect(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Clean(context.Background(), report); err == nil {
		t.Fatal("cleaned an unrelated database")
	}
	if data, err := os.ReadFile(filepath.Join(dir, "panel.db")); err != nil || string(data) != "fixture" {
		t.Fatal("foreign database changed")
	}
}

func TestCleanupDoesNotFollowSymlinks(t *testing.T) {
	for _, linked := range []string{"settings", "logs", "artifacts", "nested"} {
		t.Run(linked, func(t *testing.T) {
			path, dir := fixture(t)
			outside := filepath.Join(filepath.Dir(dir), "outside")
			putFile(t, filepath.Join(outside, "keep"))
			switch linked {
			case "settings":
				if err := os.Rename(path, path+".real"); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(path+".real", path); err != nil {
					t.Fatal(err)
				}
			case "nested":
				if err := os.Mkdir(filepath.Join(dir, "artifacts"), 0o700); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(outside, filepath.Join(dir, "artifacts/link")); err != nil {
					t.Fatal(err)
				}
			default:
				if err := os.Symlink(outside, filepath.Join(dir, linked)); err != nil {
					t.Fatal(err)
				}
			}
			report, err := Inspect(context.Background(), path)
			if err != nil {
				t.Fatal(err)
			}
			_, err = Clean(context.Background(), report)
			if linked == "nested" && err != nil {
				t.Fatal(err)
			}
			if linked != "nested" && err == nil {
				t.Fatal("unsafe root link accepted")
			}
			if data, err := os.ReadFile(filepath.Join(outside, "keep")); err != nil || string(data) != "fixture" {
				t.Fatal("followed symlink outside managed data")
			}
		})
	}
}

func TestCleanRemovesEmptyDataDirectoryAndRejectsChangedSelection(t *testing.T) {
	path, dir := fixture(t)
	report, err := Inspect(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	other := report
	other.DataDir = filepath.Join(dir, "different")
	if _, err := Clean(context.Background(), other); err == nil {
		t.Fatal("changed instance selection accepted")
	}
	if _, err := Clean(context.Background(), report); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("empty data directory remains: %v", err)
	}
}

func TestCleanupRefusesAnActiveCLIConnection(t *testing.T) {
	path, dir := fixture(t)
	db, err := store.Open(context.Background(), filepath.Join(dir, "panel.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	report, err := Inspect(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Clean(context.Background(), report); err == nil {
		t.Fatal("deleted data while another CLI still had it open")
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal("settings removed while instance was busy")
	}
}

func TestInspectRecognizesUncheckpointedDatabase(t *testing.T) {
	path, dir := fixture(t)
	if err := os.Remove(filepath.Join(dir, "panel.db")); err != nil {
		t.Fatal(err)
	}
	db, err := store.Open(context.Background(), filepath.Join(dir, "panel.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	report, err := Inspect(context.Background(), path)
	if err != nil || report.DatabaseIdentity != "sing-box-panel" {
		t.Fatalf("running database identity=%s err=%v", report.DatabaseIdentity, err)
	}
}

func TestDatabaseOpenIsBlockedDuringCleanup(t *testing.T) {
	_, dir := fixture(t)
	lock, err := store.LockDirectoryForCleanup(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close()
	if db, err := store.Open(context.Background(), filepath.Join(dir, "panel.db")); err == nil {
		_ = db.Close()
		t.Fatal("opened database during exclusive cleanup")
	}
}

func TestCleanupRetainsLinkedSettingsParent(t *testing.T) {
	path, _ := fixture(t)
	parent := filepath.Dir(path)
	alias := filepath.Join(t.TempDir(), "sing-box-panel")
	if err := os.Symlink(parent, alias); err != nil {
		t.Fatal(err)
	}
	report, err := Inspect(context.Background(), filepath.Join(alias, "setting.json"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Clean(context.Background(), report); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Lstat(alias); err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("settings-directory link was removed: %v", err)
	}
}

func TestInspectDoesNotCreateDatabaseSidecars(t *testing.T) {
	path, dir := fixture(t)
	names := func() []string {
		files, err := os.ReadDir(dir)
		if err != nil {
			t.Fatal(err)
		}
		result := []string{}
		for _, file := range files {
			result = append(result, file.Name())
		}
		return result
	}
	before := names()
	if _, err := Inspect(context.Background(), path); err != nil {
		t.Fatal(err)
	}
	if after := names(); !reflect.DeepEqual(before, after) {
		t.Fatalf("inspection changed files: before=%v after=%v", before, after)
	}
}

func TestInspectLeavesIncompleteOrLinkedWALUntouched(t *testing.T) {
	for _, kind := range []string{"missing shared memory", "linked shared memory", "linked WAL"} {
		t.Run(kind, func(t *testing.T) {
			path, dir := fixture(t)
			outside := filepath.Join(t.TempDir(), "keep")
			putFile(t, outside)
			wal := filepath.Join(dir, "panel.db-wal")
			shm := filepath.Join(dir, "panel.db-shm")
			if kind == "linked WAL" {
				if err := os.Symlink(outside, wal); err != nil {
					t.Fatal(err)
				}
				putFile(t, shm)
			} else {
				putFile(t, wal)
				if kind == "linked shared memory" {
					if err := os.Symlink(outside, shm); err != nil {
						t.Fatal(err)
					}
				}
			}
			report, err := Inspect(context.Background(), path)
			if err != nil || report.DatabaseIdentity != "unknown" {
				t.Fatalf("identity=%s err=%v", report.DatabaseIdentity, err)
			}
			if err := ValidateCleanup(report); err == nil {
				t.Fatal("unverified WAL accepted for cleanup")
			}
			if kind == "missing shared memory" {
				if _, err := os.Lstat(shm); !os.IsNotExist(err) {
					t.Fatalf("inspection created shared memory: %v", err)
				}
			}
			for _, file := range []string{outside, wal} {
				if data, err := os.ReadFile(file); err != nil || string(data) != "fixture" {
					t.Fatalf("inspection modified %s: %v", file, err)
				}
			}
		})
	}
}
