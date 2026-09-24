// SPDX-License-Identifier: GPL-3.0-or-later

package systemd

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/rehuony/sing-box-panel/internal/settings"
)

func TestUninstallChecksManagerWhenFilesAreMissing(t *testing.T) {
	for _, scenario := range []string{"absent", "cached stopped", "active", "manager unavailable", "foreign fragment", "unknown fragment", "stop fails", "still running"} {
		t.Run(scenario, func(t *testing.T) {
			f := newManagerFixture(t, 1000)
			queries, stops := 0, 0
			f.runner.run = func(_ string, args []string) (CommandResult, error) {
				if slices.Contains(args, "--property=LoadState") {
					queries++
					if scenario == "manager unavailable" {
						return CommandResult{}, errors.New("manager unavailable")
					}
					load, active, pid, fragment := "loaded", "active", "123", f.layout.UserUnitPath
					switch scenario {
					case "absent":
						load, active, pid, fragment = "not-found", "inactive", "0", ""
					case "cached stopped":
						active, pid = "inactive", "0"
					case "foreign fragment":
						fragment = filepath.Join(t.TempDir(), UnitName)
					case "unknown fragment":
						load, fragment = "error", ""
					case "active":
						if stops > 0 {
							active, pid = "inactive", "0"
						}
					}
					return CommandResult{Stdout: []byte("LoadState=" + load + "\nActiveState=" + active + "\nSubState=dead\nUnitFileState=disabled\nMainPID=" + pid + "\nFragmentPath=" + fragment + "\n")}, nil
				}
				if slices.Contains(args, "stop") {
					stops++
					if scenario == "stop fails" {
						return CommandResult{}, errors.New("cannot stop service")
					}
				}
				return CommandResult{}, nil
			}
			result, err := f.manager.Uninstall(t.Context(), UninstallRequest{Scope: ScopeUser})
			success := scenario == "absent" || scenario == "cached stopped" || scenario == "active"
			if queries == 0 || (err == nil) != success {
				t.Fatalf("uninstall: %+v, error=%v, queries=%d", result, err, queries)
			}
			if result.Stopped != success || len(result.RemovedPaths) != 0 {
				t.Fatalf("incorrect completion claims: %+v", result)
			}
			shouldStop := scenario == "active" || scenario == "stop fails" || scenario == "still running"
			if (stops == 1) != shouldStop {
				t.Fatalf("stop calls=%d", stops)
			}
			if scenario == "active" && queries != 2 {
				t.Fatal("did not verify service termination")
			}
			for _, call := range f.runner.calls {
				if slices.Contains(call.args, "disable") || slices.Contains(call.args, "enable") {
					t.Fatalf("modified nonexistent enablement: %+v", call)
				}
				if slices.Contains(call.args, "daemon-reload") && !success {
					t.Fatal("continued after failure")
				}
			}
			if scenario == "absent" && len(f.runner.calls) != 1 {
				t.Fatalf("no-op performed mutations: %+v", f.runner.calls)
			}
		})
	}
}

func TestControlDoesNotDependOnUnrelatedInventoryDirectories(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root bypasses directory read permissions")
	}
	for _, action := range []Action{ActionStart, ActionRestart} {
		for _, restore := range []bool{false, true} {
			name := string(action)
			if restore {
				name += "/restore"
			}
			t.Run(name, func(t *testing.T) {
				f := newManagerFixture(t, 1000)
				if _, err := f.manager.Install(t.Context(), InstallRequest{Scope: ScopeUser, SettingsPath: f.settings}); err != nil {
					t.Fatal(err)
				}
				unrelated := filepath.Join(filepath.Dir(f.layout.UserUnitPath), "unrelated.target.wants")
				if err := os.Mkdir(unrelated, 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.Chmod(unrelated, 0); err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = os.Chmod(unrelated, 0700) })
				f.runner.calls = nil
				files, err := f.manager.Files(t.Context(), ScopeUser)
				if !os.IsPermission(err) || len(files.Files) == 0 || !files.Files[0].Managed || len(f.runner.calls) != 0 {
					t.Fatalf("inventory must retain partial results and inspection errors: %+v %v", files, err)
				}
				if restore {
					for _, path := range []string{f.settings, f.data} {
						if err := os.Remove(path); err != nil {
							t.Fatal(err)
						}
					}
				}
				f.runner.run = func(_ string, args []string) (CommandResult, error) {
					if slices.Contains(args, "--property=LoadState") {
						return CommandResult{Stdout: []byte("LoadState=loaded\nActiveState=inactive\nSubState=dead\nUnitFileState=enabled\nMainPID=0\nFragmentPath=" + f.layout.UserUnitPath + "\n")}, nil
					}
					return CommandResult{}, nil
				}
				if _, err := f.manager.Control(t.Context(), ScopeUser, action); err != nil {
					t.Fatalf("unrelated directory blocked %s: %v", action, err)
				}
				last := f.runner.calls[len(f.runner.calls)-1]
				if !slices.Contains(last.args, string(action)) {
					t.Fatalf("did not run requested action: %+v", last)
				}
				config, err := settings.Load(f.settings)
				if err != nil || config.DataDir != f.data {
					t.Fatalf("lost installed settings: %+v %v", config, err)
				}
			})
		}
	}
}

func TestInstallInitializesOnlyAfterPreflight(t *testing.T) {
	for _, failure := range []string{"", "command", "manager", "conflict", "canceled"} {
		t.Run(failure, func(t *testing.T) {
			f := newManagerFixture(t, 0)
			if err := os.Remove(f.settings); err != nil {
				t.Fatal(err)
			}
			if err := os.Remove(f.data); err != nil {
				t.Fatal(err)
			}
			switch failure {
			case "command":
				f.manager.lookPath = func(string) (string, error) { return "", os.ErrNotExist }
			case "manager":
				f.runner.run = func(string, []string) (CommandResult, error) {
					return CommandResult{}, errors.New("manager unavailable")
				}
			case "conflict":
				if err := os.MkdirAll(filepath.Dir(f.layout.SystemUnitPath), 0755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(f.layout.SystemUnitPath, []byte("custom"), 0644); err != nil {
					t.Fatal(err)
				}
			}
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			if failure == "canceled" {
				cancel()
			}
			result, err := f.manager.Install(ctx, InstallRequest{Scope: ScopeSystem, SettingsPath: f.settings, Now: true})
			if failure != "" {
				if err == nil {
					t.Fatal("preflight failure accepted")
				}
				for _, path := range []string{f.settings, f.settings + ".lock", f.data} {
					if _, err := os.Lstat(path); !os.IsNotExist(err) {
						t.Fatalf("created before preflight: %s", path)
					}
				}
				return
			}
			if err != nil || !result.SettingsCreated {
				t.Fatalf("fresh install: %+v %v", result, err)
			}
			config, err := settings.Load(f.settings)
			if err != nil || config.DataDir != f.data {
				t.Fatalf("settings: %+v %v", config, err)
			}
			before, _ := os.ReadFile(f.settings)
			if _, err := f.manager.Install(t.Context(), InstallRequest{Scope: ScopeSystem, SettingsPath: f.settings}); err != nil {
				t.Fatal(err)
			}
			after, _ := os.ReadFile(f.settings)
			if !bytes.Equal(before, after) {
				t.Fatal("reinstall changed settings/token")
			}
			if _, err := os.Stat(filepath.Join(f.data, "panel.db")); !os.IsNotExist(err) {
				t.Fatal("installer created database")
			}
		})
	}
}

func TestUninstallStoppedBrokenUnitAndPartialFailure(t *testing.T) {
	for _, stage := range []string{"broken", "stop", "disable", "reload"} {
		t.Run(stage, func(t *testing.T) {
			f := newManagerFixture(t, 1000)
			if _, err := f.manager.Install(t.Context(), InstallRequest{Scope: ScopeUser, SettingsPath: f.settings}); err != nil {
				t.Fatal(err)
			}
			f.runner.calls = nil
			f.runner.run = func(name string, args []string) (CommandResult, error) {
				joined := strings.Join(args, " ")
				if strings.Contains(joined, "--property=LoadState") {
					active, pid := "inactive", "0"
					if stage == "stop" {
						active, pid = "active", "123"
					}
					return CommandResult{Stdout: []byte("LoadState=bad-setting\nActiveState=" + active + "\nSubState=dead\nUnitFileState=enabled\nMainPID=" + pid + "\nFragmentPath=" + f.layout.UserUnitPath + "\n")}, nil
				}
				if strings.Contains(joined, " "+stage+" ") || stage == "reload" && strings.Contains(joined, "daemon-reload") {
					return CommandResult{}, errors.New("injected " + stage)
				}
				return CommandResult{}, nil
			}
			result, err := f.manager.Uninstall(t.Context(), UninstallRequest{Scope: ScopeUser})
			if stage == "broken" {
				if err != nil {
					t.Fatal(err)
				}
			} else if err == nil {
				t.Fatal("failure swallowed")
			}
			if stage == "broken" || stage == "reload" {
				if len(result.RemovedPaths) != 1 {
					t.Fatalf("lost completed removals: %+v", result)
				}
			} else if len(result.RemovedPaths) != 0 {
				t.Fatal("removed files before stop/disable succeeded")
			}
		})
	}
}

func TestGeneratedUnitsWithSystemdAnalyze(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("requires Linux systemd parser")
	}
	analyze, err := exec.LookPath("systemd-analyze")
	if err != nil {
		t.Skip("systemd-analyze is a CI/test dependency only")
	}
	for _, scope := range []Scope{ScopeSystem, ScopeUser} {
		for _, suffix := range []string{"plain", `space and $cash%literal`, `quote"and\backslash`} {
			t.Run(string(scope)+"/"+suffix, func(t *testing.T) {
				dir := t.TempDir()
				data := filepath.Join(dir, suffix)
				unit, err := renderUnit(scope, "/usr/bin/true", filepath.Join(dir, "setting.json"), data)
				if err != nil {
					t.Fatal(err)
				}
				path := filepath.Join(dir, UnitName)
				if err := os.WriteFile(path, unit, 0600); err != nil {
					t.Fatal(err)
				}
				output, err := exec.CommandContext(t.Context(), analyze, "verify", "--man=no", path).CombinedOutput()
				if err != nil {
					t.Fatalf("systemd rejected generated unit: %v\n%s\n%s", err, output, unit)
				}
			})
		}
	}
}

func TestStartRestoresInstalledCustomResources(t *testing.T) {
	f := newManagerFixture(t, 1000)
	if _, err := f.manager.Install(t.Context(), InstallRequest{Scope: ScopeUser, SettingsPath: f.settings}); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(f.settings); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(f.data); err != nil {
		t.Fatal(err)
	}
	f.runner.run = func(_ string, args []string) (CommandResult, error) {
		if strings.Contains(strings.Join(args, " "), "--property=LoadState") {
			return CommandResult{Stdout: []byte("LoadState=loaded\nActiveState=inactive\nSubState=dead\nUnitFileState=enabled\nMainPID=0\nFragmentPath=" + f.layout.UserUnitPath + "\n")}, nil
		}
		return CommandResult{}, nil
	}
	if _, err := f.manager.Control(t.Context(), ScopeUser, ActionStart); err != nil {
		t.Fatal(err)
	}
	config, err := settings.Load(f.settings)
	if err != nil || config.DataDir != f.data {
		t.Fatalf("restored wrong instance: %+v %v", config, err)
	}
	if _, err := os.Stat(f.data); err != nil {
		t.Fatal(err)
	}
}

func TestRelatedFilesAndOrphanAuxiliariesAreVisible(t *testing.T) {
	f := newManagerFixture(t, 0)
	if _, err := f.manager.Install(t.Context(), InstallRequest{Scope: ScopeSystem, SettingsPath: f.settings}); err != nil {
		t.Fatal(err)
	}
	unit := f.layout.SystemUnitPath
	links := filepath.Join(filepath.Dir(unit), "multi-user.target.wants")
	dropins := unit + ".d"
	for _, dir := range []string{links, dropins} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
	}
	link := filepath.Join(links, UnitName)
	if err := os.Symlink(unit, link); err != nil {
		t.Fatal(err)
	}
	custom := filepath.Join(dropins, "custom.conf")
	if err := os.WriteFile(custom, []byte("[Service]\nRestartSec=10\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(unit); err != nil {
		t.Fatal(err)
	}
	f.runner.calls = nil
	files, err := f.manager.Files(t.Context(), ScopeSystem)
	if err != nil {
		t.Fatal(err)
	}
	if len(f.runner.calls) != 0 || files.SettingsPath != f.settings {
		t.Fatalf("orphan discovery: %+v", files)
	}
	found := map[string]bool{}
	for _, file := range files.Files {
		if file.Retained {
			found[file.Path] = true
		}
	}
	if !found[link] || !found[custom] {
		t.Fatalf("related files missing: %+v", files)
	}
	result, err := f.manager.Uninstall(t.Context(), UninstallRequest{Scope: ScopeSystem})
	if err != nil || len(result.RemovedPaths) != 2 {
		t.Fatalf("orphan uninstall: %+v %v", result, err)
	}
	if _, err := os.Stat(custom); err != nil {
		t.Fatal("custom drop-in was removed")
	}
}
