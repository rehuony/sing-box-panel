// SPDX-License-Identifier: GPL-3.0-or-later

package settings

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadOrInitializeConcurrentStarts(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root defaults use the system data directory")
	}
	root := t.TempDir()
	t.Setenv("XDG_DATA_HOME", filepath.Join(root, "data"))
	path := filepath.Join(root, "config", "setting.json")
	type result struct {
		value   Settings
		created bool
		err     error
	}
	const callers = 16
	results := make(chan result, callers)
	start := make(chan struct{})
	for range callers {
		go func() {
			<-start
			value, created, err := LoadOrInitialize(path)
			results <- result{value, created, err}
		}()
	}
	close(start)
	created := 0
	var token string
	for range callers {
		got := <-results
		if got.err != nil {
			t.Errorf("LoadOrInitialize() error = %v", got.err)
			continue
		}
		if got.created {
			created++
		}
		if token == "" {
			token = got.value.Auth.Token
		}
		if got.value.Auth.Token != token {
			t.Error("concurrent starts loaded different tokens")
		}
	}
	if created != 1 || token == "" {
		t.Fatalf("created = %d, want one creation with a token", created)
	}
	value, err := Load(path)
	if err != nil || value.Auth.Token != token {
		t.Fatalf("stored settings differ from the startup settings: %v", err)
	}
	for path, wantMode := range map[string]os.FileMode{
		path: 0o600, filepath.Dir(path): 0o700, value.DataDir: 0o700,
	} {
		info, err := os.Stat(path)
		if err != nil || info.Mode().Perm() != wantMode {
			t.Fatalf("permissions for %s: info=%v error=%v", path, info, err)
		}
	}
}

func TestLoadOrInitializePreservesInvalidFiles(t *testing.T) {
	for _, name := range []string{"malformed", "invalid", "dangling link", "directory", "unreadable"} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "setting.json")
			content := []byte("{}")
			var err error
			switch name {
			case "dangling link":
				err = os.Symlink("missing.json", path)
			case "directory":
				err = os.Mkdir(path, 0o700)
			default:
				if name == "malformed" {
					content = []byte("{")
				}
				err = os.WriteFile(path, content, 0o600)
			}
			if err != nil {
				t.Fatal(err)
			}
			if name == "unreadable" {
				if os.Geteuid() == 0 {
					t.Skip("root can read files without read permission")
				}
				if err := os.Chmod(path, 0); err != nil {
					t.Fatal(err)
				}
			}
			before, err := os.Lstat(path)
			if err != nil {
				t.Fatal(err)
			}
			if _, created, err := LoadOrInitialize(path); err == nil || created {
				t.Fatalf("created = %v, error = %v; want rejection", created, err)
			}
			after, err := os.Lstat(path)
			if err != nil || !os.SameFile(before, after) || before.Mode() != after.Mode() {
				t.Fatalf("selected file was changed: %v", err)
			}
			if name == "malformed" || name == "invalid" {
				data, err := os.ReadFile(path)
				if err != nil || !bytes.Equal(data, content) {
					t.Fatalf("existing settings were modified: %v", err)
				}
			}
		})
	}
}

func TestAtomicSettingsPublication(t *testing.T) {
	path := filepath.Join(t.TempDir(), "setting.json")
	if err := atomicWrite(path, []byte("first"), 0o600, false); err != nil {
		t.Fatal(err)
	}
	if err := atomicWrite(path, []byte("second"), 0o600, false); !errors.Is(err, os.ErrExist) {
		t.Fatalf("second publication error = %v, want ErrExist", err)
	}
	if data, err := os.ReadFile(path); err != nil || string(data) != "first" {
		t.Fatalf("original file changed: %q, %v", data, err)
	}
	if err := atomicWrite(path, []byte("forced"), 0o600, true); err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(path); err != nil || string(data) != "forced" {
		t.Fatalf("explicit replacement failed: %q, %v", data, err)
	}
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil || len(entries) != 1 {
		t.Fatalf("temporary settings files were left behind: %v", err)
	}
}
