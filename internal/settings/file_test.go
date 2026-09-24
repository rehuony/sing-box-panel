// SPDX-License-Identifier: GPL-3.0-or-later

package settings

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReplacePreservesNonRegularDestinations(t *testing.T) {
	for _, kind := range []string{"directory", "symlink", "dangling symlink"} {
		t.Run(kind, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "setting.json")
			target := filepath.Join(dir, "target.json")
			value := Defaults()
			value.DataDir = filepath.Join(dir, "data")
			value.Auth.Token = "fixture-token"
			input, err := json.Marshal(value)
			if err != nil {
				t.Fatal(err)
			}
			if kind == "directory" {
				err = os.Mkdir(path, 0700)
			} else {
				if kind == "symlink" {
					if err := os.WriteFile(target, []byte("unchanged"), 0600); err != nil {
						t.Fatal(err)
					}
				}
				err = os.Symlink(target, path)
			}
			if err != nil {
				t.Fatal(err)
			}
			if err := Replace(path, input); err == nil {
				t.Fatal("non-regular destination replaced")
			}
			info, err := os.Lstat(path)
			if err != nil {
				t.Fatal(err)
			}
			if kind == "directory" {
				if !info.IsDir() {
					t.Fatal("directory replaced")
				}
			} else if info.Mode()&os.ModeSymlink == 0 {
				t.Fatal("link replaced")
			}
			if kind == "symlink" {
				data, err := os.ReadFile(target)
				if err != nil || string(data) != "unchanged" {
					t.Fatal("link target changed")
				}
			} else if _, err := os.Stat(target); !errors.Is(err, os.ErrNotExist) {
				t.Fatal("link target created")
			}
		})
	}
}

func TestInitializeFilePreservesStorageAndRecovery(t *testing.T) {
	for _, scenario := range []string{"storage file", "symlink", "pending settings", "pending migration"} {
		t.Run(scenario, func(t *testing.T) {
			if scenario == "storage file" && os.Geteuid() == 0 {
				t.Skip("requires user-scoped XDG defaults; do not write to root's real data path")
			}
			root := t.TempDir()
			t.Setenv("XDG_DATA_HOME", filepath.Join(root, "data-home"))
			path := filepath.Join(root, "setting.json")
			value := Defaults()
			value.DataDir = filepath.Join(root, "original-data")
			value.Auth.Token = "keep-token"
			before, _ := json.Marshal(value)
			if err := os.WriteFile(path, before, 0600); err != nil {
				t.Fatal(err)
			}
			switch scenario {
			case "storage file":
				if err := os.MkdirAll(filepath.Dir(Defaults().DataDir), 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(Defaults().DataDir, []byte("sentinel"), 0600); err != nil {
					t.Fatal(err)
				}
			case "symlink":
				if err := os.Rename(path, path+".target"); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(path+".target", path); err != nil {
					t.Fatal(err)
				}
			case "pending settings":
				if err := os.WriteFile(path+".pending", []byte("pending"), 0600); err != nil {
					t.Fatal(err)
				}
			case "pending migration":
				if err := WriteDataLocation(path, DataLocation{DataDir: value.DataDir, Established: true, Move: &DataMove{ID: "pending", Target: Defaults().DataDir}}); err != nil {
					t.Fatal(err)
				}
			}
			_, err := InitializeFile(context.Background(), path, true)
			if scenario == "storage file" {
				if err != nil {
					t.Fatal("file initialization accessed storage", err)
				}
				raw, err := os.ReadFile(Defaults().DataDir)
				if err != nil || string(raw) != "sentinel" {
					t.Fatal("storage was changed", err)
				}
				active, err := LoadDataDir(path)
				if err != nil || active != value.DataDir {
					t.Fatal("forced initialization lost the previous data directory", err)
				}
				return
			}
			if err == nil {
				t.Fatal("unsafe initialization accepted")
			}
			after, _ := os.ReadFile(path)
			if !bytes.Equal(before, after) {
				t.Fatal("unsafe initialization changed settings")
			}
		})
	}
}

func TestReadBoundsSettings(t *testing.T) {
	path := filepath.Join(t.TempDir(), "setting.json")
	if err := os.WriteFile(path, bytes.Repeat([]byte(" "), MaximumBytes+1), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Read(path); err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("oversized read = %v", err)
	}
}

func TestPrivateSourceDefaultsAndCustomValue(t *testing.T) {
	value := Defaults()
	if len(value.Subscription.PrivateSourceCIDRs) != 0 {
		t.Fatal("private source allowlist should default to empty")
	}
	value.Auth.Token = "fixture-token"
	value.Subscription.PrivateSourceCIDRs = []string{"10.0.0.0/8"}
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "setting.json")
	if err := Replace(path, data); err != nil {
		t.Fatal(err)
	}
	loaded, created, err := LoadOrInitialize(path)
	if err != nil || created || len(loaded.Subscription.PrivateSourceCIDRs) != 1 || loaded.Subscription.PrivateSourceCIDRs[0] != "10.0.0.0/8" {
		t.Fatalf("private source allowlist not preserved: created=%t, error=%v", created, err)
	}
}

func TestPendingUpdatePreventsInitialization(t *testing.T) {
	path := filepath.Join(t.TempDir(), "setting.json")
	if err := os.WriteFile(path+".pending", []byte("recovery needed"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, created, err := LoadOrInitialize(path); created || !errors.Is(err, ErrPending) {
		t.Fatalf("initialized over recovery: created=%t error=%v", created, err)
	}
	if _, err := Initialize(path, true); !errors.Is(err, ErrPending) {
		t.Fatalf("force bypassed recovery: %v", err)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("settings created: %v", err)
	}
}

func TestLegacyFileGetsPanelDefaultsWithoutLosingExplicitZeroRadius(t *testing.T) {
	value := Defaults()
	value.Auth.Token = "fixture-token"
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatal(err)
	}
	delete(fields, "panel")
	legacy, _ := json.Marshal(fields)
	loaded, err := Parse(filepath.Join(t.TempDir(), "setting.json"), legacy)
	if err != nil || loaded.Panel != DefaultPanel() {
		t.Fatalf("legacy defaults: %+v %v", loaded.Panel, err)
	}
	value.Panel.Appearance.Radius = 0
	raw, _ = json.Marshal(value)
	loaded, err = Parse(filepath.Join(t.TempDir(), "setting.json"), raw)
	if err != nil || loaded.Panel.Appearance.Radius != 0 {
		t.Fatalf("zero radius changed: %+v %v", loaded.Panel, err)
	}
}
