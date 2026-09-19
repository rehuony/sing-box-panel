// SPDX-License-Identifier: GPL-3.0-or-later

package installation

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"slices"
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

func TestInspectWithoutSettingsDoesNotGuessDataDirectory(t *testing.T) {
	path, dir := fixture(t)
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	for _, selected := range []string{path, filepath.Join(t.TempDir(), "sing-box-panel", "setting.json")} {
		report, err := Inspect(t.Context(), selected)
		if err != nil {
			t.Fatal(err)
		}
		if report.SettingsPath != selected || report.DataDir != "" || report.DatabaseIdentity != "unknown" {
			t.Fatalf("missing settings produced an inferred data location: %+v", report)
		}
		var settingsFound, executableFound bool
		for _, entry := range report.Entries {
			switch entry.Role {
			case "panel bootstrap settings":
				settingsFound = entry.Path == selected && entry.State == "missing"
			case "panel executable":
				executableFound = entry.State != "missing"
			case "instance data directory":
				t.Fatalf("invented a data directory: %+v", entry)
			}
		}
		if !settingsFound || !executableFound {
			t.Fatalf("known paths missing from report: %+v", report)
		}
		if _, err := Clean(t.Context(), report); err == nil {
			t.Fatal("cleanup accepted missing settings")
		}
		if _, err := os.Lstat(selected); !os.IsNotExist(err) {
			t.Fatalf("inspection or cleanup recreated settings: %v", err)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "panel.db")); err != nil {
		t.Fatalf("unlocated data was changed: %v", err)
	}
}

func TestInspectStillRejectsInvalidExistingSettings(t *testing.T) {
	for _, contents := range []string{"{", "{}", `{"data_dir":""}`, `{"data_dir":42}`} {
		path := filepath.Join(t.TempDir(), "setting.json")
		if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := Inspect(t.Context(), path); err == nil {
			t.Fatalf("invalid existing settings were treated as absent: %s", contents)
		}
	}
	path := filepath.Join(t.TempDir(), "setting.json")
	if err := os.Symlink(filepath.Join(t.TempDir(), "absent.json"), path); err != nil {
		t.Fatal(err)
	}
	if _, err := Inspect(t.Context(), path); err == nil {
		t.Fatal("dangling settings symlink was treated as an absent settings file")
	}
}

func TestInspectAndCleanRemoveAllDataAndKeepOutsideFiles(t *testing.T) {
	path, dir := fixture(t)
	for _, name := range []string{"artifacts/sha256/core/sing-box", "runtime/configs/current.json", "logs/core/2026-09-19-000.log", "imports/core-upload-test", "notes.txt", "logs/operator-notes.txt", "legacy/nested/unknown.bin"} {
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
	if !seen[filepath.Join(dir, "logs/core/2026-09-19-000.log")] || !seen[filepath.Join(dir, "legacy/nested/unknown.bin")] {
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
	for _, name := range []string{path, filepath.Join(dir, "panel.db"), filepath.Join(dir, "artifacts"), filepath.Join(dir, "runtime"), filepath.Join(dir, "imports"), filepath.Join(dir, "logs/core"), filepath.Join(dir, panelprocess.LeaseFileName), filepath.Join(dir, "notes.txt"), filepath.Join(dir, "legacy"), dir} {
		if _, err := os.Lstat(name); !os.IsNotExist(err) {
			t.Fatalf("managed path remains: %s %v", name, err)
		}
	}
	for _, name := range []string{outside} {
		if data, err := os.ReadFile(name); err != nil || string(data) != "fixture" {
			t.Fatalf("unrelated file changed: %s %v", name, err)
		}
	}
}

func TestCleanupRefusesRunningOwner(t *testing.T) {
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

}

func TestCleanupRemovesLegacyAndUnknownContents(t *testing.T) {
	for _, kind := range []string{"unknown database", "legacy database", "database directory", "database link", "no database"} {
		t.Run(kind, func(t *testing.T) {
			path, dir := fixture(t)
			database := filepath.Join(dir, "panel.db")
			outside := filepath.Join(t.TempDir(), "outside.txt")
			putFile(t, outside)
			switch kind {
			case "unknown database":
				putFile(t, database)
			case "legacy database":
				data, err := os.ReadFile(database)
				if err != nil {
					t.Fatal(err)
				}
				binary.BigEndian.PutUint32(data[68:72], uint32(store.ApplicationID-1))
				if err := os.WriteFile(database, data, 0o600); err != nil {
					t.Fatal(err)
				}
			default:
				if err := os.Remove(database); err != nil {
					t.Fatal(err)
				}
				if kind == "database directory" {
					putFile(t, filepath.Join(database, "old.db"))
				}
				if kind == "database link" {
					if err := os.Symlink(outside, database); err != nil {
						t.Fatal(err)
					}
				}
			}
			putFile(t, filepath.Join(dir, "legacy/nested/residue.bin"))
			putFile(t, filepath.Join(dir, "operator.txt"))
			if err := os.Mkdir(filepath.Join(dir, "empty"), 0o700); err != nil {
				t.Fatal(err)
			}
			link := filepath.Join(dir, "dangling")
			if err := os.Symlink(filepath.Join(t.TempDir(), "absent"), link); err != nil {
				t.Fatal(err)
			}
			report, err := Inspect(t.Context(), path)
			if err != nil {
				t.Fatal(err)
			}
			seen := make(map[string]Entry)
			for _, entry := range report.Entries {
				seen[entry.Path] = entry
				if pathWithin(dir, entry.Path) && entry.Cleanup != "remove" && entry.Cleanup != "remove_link" {
					t.Fatalf("data not included in complete cleanup: %+v", entry)
				}
			}
			if seen[link].State != "symlink" || seen[filepath.Join(dir, "empty")].State != "directory" || seen[filepath.Join(dir, "legacy/nested/residue.bin")].State != "file" {
				t.Fatalf("real contents missing from report: %+v", report)
			}
			// Cleanup must re-enumerate its directory, not trust report paths.
			report.Entries = append(report.Entries, Entry{Path: outside, Cleanup: "remove"})
			result, err := Clean(t.Context(), report)
			if err != nil {
				t.Fatal(err)
			}
			if !slices.Contains(result.Removed, dir) || !slices.Contains(result.Removed, link) {
				t.Fatalf("incomplete cleanup result: %+v", result)
			}
			if _, err := os.Lstat(dir); !os.IsNotExist(err) {
				t.Fatalf("data directory remains: %v", err)
			}
			if data, err := os.ReadFile(outside); err != nil || string(data) != "fixture" {
				t.Fatalf("outside file changed: %v", err)
			}
		})
	}
}

func TestCleanupProtectsSharedPathsAndExecutable(t *testing.T) {
	path, dir := fixture(t)
	report, err := Inspect(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	home, _ := os.UserHomeDir()
	configHome, _ := os.UserConfigDir()
	cacheHome, _ := os.UserCacheDir()
	for _, shared := range []string{"/", "/var/lib", "/etc", home, configHome, cacheHome, os.TempDir(), filepath.Dir(home)} {
		if shared == "" {
			continue
		}
		selected := report
		selected.DataDir = shared
		if err := ValidateCleanup(selected); err == nil {
			t.Errorf("accepted shared path %q", shared)
		}
	}
	alias := filepath.Join(t.TempDir(), "alias")
	if err := os.Symlink(filepath.Dir(home), alias); err != nil {
		t.Fatal(err)
	}
	selected := report
	selected.DataDir = filepath.Join(alias, filepath.Base(home))
	if err := ValidateCleanup(selected); err == nil {
		t.Fatal("accepted a home directory through a parent alias")
	}
	selected = report
	selected.Entries = slices.Clone(report.Entries)
	selected.Entries[0].Path = filepath.Join(dir, "nested/panel")
	if err := ValidateCleanup(selected); err == nil {
		t.Fatal("accepted an executable anywhere inside the data directory")
	}
}

func TestCleanupRemovesSettingsInsideDataDirectory(t *testing.T) {
	for _, name := range []string{"setting.json", "nested/sing-box-panel/setting.json"} {
		t.Run(name, func(t *testing.T) {
			path, dir := fixture(t)
			selected := filepath.Join(dir, name)
			if err := os.MkdirAll(filepath.Dir(selected), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.Rename(path, selected); err != nil {
				t.Fatal(err)
			}
			report, err := Inspect(t.Context(), selected)
			if err != nil {
				t.Fatal(err)
			}
			result, err := Clean(t.Context(), report)
			if err != nil || !slices.Contains(result.Removed, selected) || !slices.Contains(result.Removed, dir) {
				t.Fatalf("nested settings cleanup: result=%+v err=%v", result, err)
			}
		})
	}
}

func TestInspectRejectsLinkedDataRoot(t *testing.T) {
	path, dir := fixture(t)
	alias := filepath.Join(t.TempDir(), "linked-data")
	if err := os.Symlink(dir, alias); err != nil {
		t.Fatal(err)
	}
	value, err := settings.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	value.DataDir = alias
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Inspect(t.Context(), path); err == nil {
		t.Fatal("accepted a linked data root")
	}
	if _, err := os.Stat(filepath.Join(dir, "panel.db")); err != nil {
		t.Fatal("inspection changed the target")
	}
}

func TestCleanupReportsCompletedRemovalsOnFailure(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root bypasses fixture permissions")
	}
	path, dir := fixture(t)
	removed := filepath.Join(dir, "a-remove")
	putFile(t, removed)
	locked := filepath.Join(dir, "z-locked")
	putFile(t, filepath.Join(locked, "keep"))
	if err := os.Chmod(locked, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(locked, 0o700) })
	report, err := Inspect(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	result, err := Clean(t.Context(), report)
	if err == nil || !slices.Contains(result.Removed, removed) || slices.Contains(result.Removed, locked) {
		t.Fatalf("partial cleanup result=%+v err=%v", result, err)
	}
	for _, retained := range []string{path, filepath.Join(dir, "panel.db"), filepath.Join(locked, "keep")} {
		if _, err := os.Stat(retained); err != nil {
			t.Fatalf("removed a durable or failed path: %s: %v", retained, err)
		}
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
			if linked != "settings" && err != nil {
				t.Fatal(err)
			}
			if linked == "settings" && err == nil {
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
			if err := ValidateCleanup(report); err != nil {
				t.Fatalf("diagnostic WAL identity blocked directory cleanup: %v", err)
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
