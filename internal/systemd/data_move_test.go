// SPDX-License-Identifier: GPL-3.0-or-later

package systemd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rehuony/sing-box-panel/internal/panelprocess"
	"github.com/rehuony/sing-box-panel/internal/settings"
	"github.com/rehuony/sing-box-panel/internal/store"
)

func TestExplicitServiceRestartMovesDataAndUpdatesGeneratedPaths(t *testing.T) {
	for _, customized := range []bool{false, true} {
		t.Run(map[bool]string{false: "generated", true: "customized"}[customized], func(t *testing.T) {
			fixture := newManagerFixture(t, 0)
			db, err := store.Open(t.Context(), filepath.Join(fixture.data, "panel.db"))
			if err != nil {
				t.Fatal(err)
			}
			db.Close()
			value := settings.Defaults()
			value.Auth.Token = strings.Repeat("test", 8)
			value.DataDir = fixture.data
			raw, _ := json.Marshal(value)
			if err := settings.Replace(fixture.settings, raw); err != nil {
				t.Fatal(err)
			}
			if _, err := fixture.manager.Install(t.Context(), InstallRequest{Scope: ScopeSystem, SettingsPath: fixture.settings, DataDir: fixture.data}); err != nil {
				t.Fatal(err)
			}
			target := filepath.Join(filepath.Dir(fixture.data), "moved-data")
			value.DataDir = target
			raw, _ = json.Marshal(value)
			if err := settings.Replace(fixture.settings, raw); err != nil {
				t.Fatal(err)
			}
			lease, err := panelprocess.AcquireLease(fixture.data)
			if err != nil {
				t.Fatal(err)
			}
			defer lease.Close()
			if customized {
				file, err := os.OpenFile(fixture.layout.SystemUnitPath, os.O_APPEND|os.O_WRONLY, 0600)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := file.WriteString("\n# operator customization\n"); err != nil {
					t.Fatal(err)
				}
				file.Close()
			}
			stopped := false
			fixture.runner.calls = nil
			fixture.runner.run = func(name string, args []string) (CommandResult, error) {
				if name == "systemctl" && len(args) > 1 && args[1] == "show" {
					return CommandResult{Stdout: []byte("LoadState=loaded\nActiveState=active\nSubState=running\nUnitFileState=enabled\nMainPID=1\nFragmentPath=" + fixture.layout.SystemUnitPath + "\n")}, nil
				}
				if name == "systemctl" && len(args) > 1 && args[1] == "stop" {
					if _, err := os.Stat(target); !os.IsNotExist(err) {
						t.Fatal("migration touched target before service stop")
					}
					stopped = true
					return CommandResult{}, lease.Close()
				}
				return CommandResult{}, nil
			}
			_, err = fixture.manager.Control(t.Context(), ScopeSystem, ActionRestart)
			if customized {
				if err == nil || stopped {
					t.Fatalf("custom unit changed or service stopped: %v", err)
				}
				if _, err := os.Stat(filepath.Join(fixture.data, "panel.db")); err != nil {
					t.Fatal("customized instance moved", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if !stopped {
				t.Fatal("service was not stopped before migration")
			}
			if _, err := os.Stat(filepath.Join(target, "panel.db")); err != nil {
				t.Fatal(err)
			}
			unit, _ := os.ReadFile(fixture.layout.SystemUnitPath)
			if !strings.Contains(string(unit), escapedPathDirective(target)) {
				t.Fatal("unit retained previous data directory")
			}
			if strings.Contains(string(unit), "StateDirectory=sing-box-panel") {
				t.Fatal("unit would recreate the default directory")
			}
			tmpfiles, _ := os.ReadFile(fixture.layout.SystemTmpfilesPath)
			if !strings.Contains(string(tmpfiles), escapedPathDirective(target)) {
				t.Fatal("tmpfiles would recreate the old directory")
			}
		})
	}
}

func TestSystemServiceRejectsProtectedDataDirectory(t *testing.T) {
	fixture := newManagerFixture(t, 0)
	for _, path := range []string{"/home/panel-data", "/root/panel-data", "/run/user/1000/panel-data"} {
		if err := os.WriteFile(fixture.settings, validTestSettings(path), 0600); err != nil {
			t.Fatal(err)
		}
		_, err := fixture.manager.Install(t.Context(), InstallRequest{Scope: ScopeSystem, SettingsPath: fixture.settings, DataDir: path})
		if err == nil || !strings.Contains(err.Error(), "protected home") {
			t.Fatalf("protected destination accepted: %s %v", path, err)
		}
	}
	if len(fixture.runner.calls) != 0 {
		t.Fatal("invalid paths mutated the service")
	}
}

func TestTemporaryServicePathsRemainVisibleWithPrivateTmp(t *testing.T) {
	unit, err := renderUnit(ScopeUser, "/usr/local/bin/sing-box-panel", "/var/tmp/panel-config/setting.json", "/tmp/panel-data")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(unit), "PrivateTmp=true\nBindPaths=\"/tmp/panel-data\" \"/var/tmp/panel-config\"") {
		t.Fatal("private temporary directories hide selected storage")
	}
}
