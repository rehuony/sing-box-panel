// SPDX-License-Identifier: GPL-3.0-or-later

package installation

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/rehuony/sing-box-panel/internal/panelprocess"
	"github.com/rehuony/sing-box-panel/internal/store"
)

func TestCleanupProtectsNestedOwners(t *testing.T) {
	for _, owner := range []string{"runtime", "database", "both"} {
		t.Run(owner, func(t *testing.T) {
			path, dataDir := fixture(t)
			nested := filepath.Join(dataDir, "other-instance")
			keep := filepath.Join(nested, "keep.txt")
			putFile(t, keep)
			var lease *panelprocess.Lease
			var database *store.Store
			var err error
			if owner != "database" {
				lease, err = panelprocess.AcquireLease(nested)
				if err != nil {
					t.Fatal(err)
				}
				defer lease.Close()
			}
			if owner != "runtime" {
				database, err = store.Open(t.Context(), filepath.Join(nested, "panel.db"))
				if err != nil {
					t.Fatal(err)
				}
				defer database.Close()
			}
			report, err := Inspect(t.Context(), path)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := Clean(t.Context(), report); err == nil {
				t.Fatal("cleanup accepted an occupied nested instance")
			}
			for _, retained := range []string{path, filepath.Join(dataDir, "panel.db"), keep} {
				if _, err := os.Stat(retained); err != nil {
					t.Fatalf("removed protected data %s: %v", retained, err)
				}
			}
			if err := errors.Join(lease.Close(), database.Close()); err != nil {
				t.Fatal(err)
			}
			// A stale nested lease remains removable once its owner releases it.
			if _, err := Clean(t.Context(), report); err != nil {
				t.Fatalf("cleanup could not retry after owner exited: %v", err)
			}
			if _, err := os.Stat(dataDir); !os.IsNotExist(err) {
				t.Fatalf("retry left data behind: %v", err)
			}
		})
	}
}

func TestCleanupKeepsNestedSettingsAndDatabaseOnFailure(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root bypasses fixture permissions")
	}
	for _, name := range []string{"nested/setting.json", "nested/deep/sing-box-panel/setting.json"} {
		t.Run(name, func(t *testing.T) {
			path, dataDir := fixture(t)
			selected := filepath.Join(dataDir, name)
			if err := os.MkdirAll(filepath.Dir(selected), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.Rename(path, selected); err != nil {
				t.Fatal(err)
			}
			before := make(map[string][]byte)
			for _, file := range []string{selected, filepath.Join(dataDir, "panel.db")} {
				var err error
				before[file], err = os.ReadFile(file)
				if err != nil {
					t.Fatal(err)
				}
			}
			removed := filepath.Join(filepath.Dir(selected), "a-remove")
			putFile(t, removed)
			blocked := filepath.Join(filepath.Dir(selected), "z-locked")
			putFile(t, filepath.Join(blocked, "keep"))
			if err := os.Chmod(blocked, 0o500); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = os.Chmod(blocked, 0o700) })
			report, err := Inspect(t.Context(), selected)
			if err != nil {
				t.Fatal(err)
			}
			result, err := Clean(t.Context(), report)
			if err == nil || !slices.Contains(result.Removed, removed) || slices.Contains(result.Removed, selected) {
				t.Fatalf("incorrect partial result: %+v err=%v", result, err)
			}
			for file, contents := range before {
				actual, err := os.ReadFile(file)
				if err != nil || !bytes.Equal(actual, contents) {
					t.Fatalf("durable file changed on failure: %s: %v", file, err)
				}
			}
			if err := os.Chmod(blocked, 0o700); err != nil {
				t.Fatal(err)
			}
			if _, err := Clean(t.Context(), report); err != nil {
				t.Fatalf("cleanup could not retry after permissions were fixed: %v", err)
			}
			if _, err := os.Lstat(dataDir); !os.IsNotExist(err) {
				t.Fatalf("retry left data behind: %v", err)
			}
		})
	}
}

func TestCleanupDirectoryKeepsLocksUntilRemoval(t *testing.T) {
	_, dataDir := fixture(t)
	nested := filepath.Join(dataDir, "nested")
	putFile(t, filepath.Join(nested, panelprocess.LeaseFileName))
	root, err := os.OpenRoot(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	info, err := root.Lstat("nested")
	if err != nil {
		t.Fatal(err)
	}
	directory, err := openCleanupDirectory(root, "nested", "nested", info)
	if err != nil {
		t.Fatal(err)
	}
	defer directory.Close()
	if database, err := store.Open(t.Context(), filepath.Join(nested, "panel.db")); err == nil {
		_ = database.Close()
		t.Fatal("database owner entered during cleanup")
	}
	if lease, err := panelprocess.AcquireLease(nested); err == nil {
		_ = lease.Close()
		t.Fatal("runtime owner entered during cleanup")
	}
	// The locked root continues to protect storage even after its lease pathname
	// is removed. A new database cannot open before directory removal completes.
	if err := directory.root.Remove(panelprocess.LeaseFileName); err != nil {
		t.Fatal(err)
	}
	if database, err := store.Open(t.Context(), filepath.Join(nested, "panel.db")); err == nil {
		_ = database.Close()
		t.Fatal("database owner entered after lease unlink")
	}
	if err := directory.remove(); err != nil {
		t.Fatal(err)
	}
}
