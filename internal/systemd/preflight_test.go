// SPDX-License-Identifier: GPL-3.0-or-later

package systemd

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestInstallChecksDirectoryToBeUsedOrCreated(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root bypasses directory write permissions")
	}
	for _, scenario := range []string{"existing writable", "existing read-only", "missing"} {
		t.Run(scenario, func(t *testing.T) {
			f := newManagerFixture(t, 1000)
			parent := filepath.Join(t.TempDir(), "shared-parent")
			data := filepath.Join(parent, "user-data")
			if err := os.MkdirAll(data, 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(f.settings, validTestSettings(data), 0600); err != nil {
				t.Fatal(err)
			}
			switch scenario {
			case "existing read-only":
				if err := os.Chmod(data, 0500); err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = os.Chmod(data, 0700) })
			case "missing":
				if err := os.Remove(data); err != nil {
					t.Fatal(err)
				}
			}
			if err := os.Chmod(parent, 0555); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = os.Chmod(parent, 0700) })
			_, err := f.manager.Install(t.Context(), InstallRequest{Scope: ScopeUser, SettingsPath: f.settings})
			if scenario == "existing writable" {
				if err != nil {
					t.Fatalf("writable data directory requires no parent write access: %v", err)
				}
			} else {
				if err == nil || len(f.runner.calls) != 0 {
					t.Fatalf("unusable data directory passed preflight: %v, calls=%+v", err, f.runner.calls)
				}
				if _, err := os.Lstat(f.layout.UserUnitPath); !os.IsNotExist(err) {
					t.Fatalf("preflight wrote a unit: %v", err)
				}
			}
		})
	}
}

func TestInstallRejectsSidecarConflictsBeforeMutation(t *testing.T) {
	for _, scope := range []Scope{ScopeSystem, ScopeUser} {
		for _, configExists := range []bool{true, false} {
			for _, suffix := range []string{".lock", ".pending", ".location"} {
				for _, kind := range []string{"directory", "symlink"} {
					name := string(scope) + suffix + "/" + kind
					if !configExists {
						name += "/missing-config"
					}
					t.Run(name, func(t *testing.T) {
						uid := 1000
						if scope == ScopeSystem {
							uid = 0
						}
						f := newManagerFixture(t, uid)
						before, err := os.ReadFile(f.settings)
						if err != nil {
							t.Fatal(err)
						}
						if !configExists {
							if err := os.Remove(f.settings); err != nil {
								t.Fatal(err)
							}
						}
						if err := os.Remove(f.data); err != nil {
							t.Fatal(err)
						}
						conflict := f.settings + suffix
						if kind == "directory" {
							err = os.Mkdir(conflict, 0700)
						} else {
							err = os.Symlink(f.settings, conflict)
						}
						if err != nil {
							t.Fatal(err)
						}
						_, err = f.manager.Install(t.Context(), InstallRequest{Scope: scope, SettingsPath: f.settings, DataDir: f.data})
						if err == nil || len(f.runner.calls) != 0 {
							t.Fatalf("conflict detected after commands: %v, calls=%+v", err, f.runner.calls)
						}
						for _, path := range append(f.manager.managedPaths(scope), f.data) {
							if _, err := os.Lstat(path); !os.IsNotExist(err) {
								t.Fatalf("created %s before rejecting conflict: %v", path, err)
							}
						}
						after, err := os.ReadFile(f.settings)
						if configExists && (err != nil || !bytes.Equal(before, after)) {
							t.Fatal("changed existing settings")
						}
						if !configExists && !os.IsNotExist(err) {
							t.Fatal("created settings despite conflict")
						}
					})
				}
			}
		}
	}
}
