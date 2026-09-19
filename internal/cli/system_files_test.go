// SPDX-License-Identifier: GPL-3.0-or-later

package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rehuony/sing-box-panel/internal/settings"
	panelSystemd "github.com/rehuony/sing-box-panel/internal/systemd"
)

func TestSystemFilesUsesDefaultAndExplicitConfigPaths(t *testing.T) {
	defaultPath := commandSettingsFixture(t)
	if os.Geteuid() != 0 {
		configHome := t.TempDir()
		t.Setenv("XDG_CONFIG_HOME", configHome)
		defaultPath = settings.DefaultPath()
		if err := os.MkdirAll(filepath.Dir(defaultPath), 0o700); err != nil {
			t.Fatal(err)
		}
		data, err := os.ReadFile(commandSettingsFixture(t))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(defaultPath, data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	explicitPath := commandSettingsFixture(t)
	tests := []struct {
		name string
		args []string
		path string
	}{
		{"short", []string{"system", "files", "-c", explicitPath}, explicitPath},
		{"long", []string{"--config", explicitPath, "system", "files"}, explicitPath},
		{"last flag", []string{"-c", defaultPath, "system", "files", "--config", explicitPath}, explicitPath},
	}
	if os.Geteuid() != 0 {
		tests = append(tests, struct {
			name string
			args []string
			path string
		}{"default", []string{"system", "files"}, defaultPath})
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			root := NewRootCommand(Dependencies{Stdout: &stdout, Stderr: &stderr, Systemd: &fakeSystemdService{}})
			root.SetArgs(append(test.args, "--output", "json"))
			if err := root.ExecuteContext(context.Background()); err != nil {
				t.Fatal(err)
			}
			var result instanceFilesReport
			if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
				t.Fatal(err)
			}
			if result.SettingsPath != test.path {
				t.Fatalf("loaded %q want %q", result.SettingsPath, test.path)
			}
			value, err := settings.Load(test.path)
			if err != nil || result.DataDir != value.DataDir {
				t.Fatalf("default/explicit file not loaded: %+v %v", result, err)
			}
		})
	}
}

func TestSystemCleanDefaultsToPreviewAndRejectsEmptyConfig(t *testing.T) {
	path := commandSettingsFixture(t)
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	root := NewRootCommand(Dependencies{Stdout: &stdout, Stderr: &stderr, Systemd: &fakeSystemdService{}})
	root.SetArgs([]string{"system", "clean", "--config", path})
	if err := root.ExecuteContext(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout.String(), "Preview only") {
		t.Fatalf("output=%s", stdout.String())
	}
	after, err := os.ReadFile(path)
	if err != nil || string(before) != string(after) {
		t.Fatal("preview removed or changed settings")
	}
	root = NewRootCommand(Dependencies{Stdout: &stdout, Stderr: &stderr, Systemd: &fakeSystemdService{}})
	root.SetArgs([]string{"system", "files", "--config="})
	if err := root.ExecuteContext(context.Background()); err == nil {
		t.Fatal("explicit empty config silently used default")
	}
}

func TestCleanupDistinguishesMatchingSharedAndUnrelatedServices(t *testing.T) {
	path := commandSettingsFixture(t)
	value, err := settings.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"matching", "shared", "unrelated", "unknown"} {
		t.Run(kind, func(t *testing.T) {
			service := &fakeSystemdService{
				filesResult:  panelSystemd.FilesResult{Scope: panelSystemd.ScopeUser, SettingsPath: path, Files: []panelSystemd.FileStatus{{Path: "fixture.service", State: "managed", Managed: true}}},
				statusResult: panelSystemd.Status{Scope: panelSystemd.ScopeUser, UnitFileSettingsPath: path},
			}
			report, err := inspectInstanceFiles(context.Background(), path, panelSystemd.ScopeUser, service)
			if err != nil {
				t.Fatal(err)
			}
			if kind != "matching" {
				report.ServiceMatches = false
				report.Service.SettingsPath = "/different/setting.json"
			}
			if kind == "unrelated" {
				report.ServiceDataDir = filepath.Join(value.DataDir, "other")
			}
			if kind == "unknown" {
				report.ServiceDataKnown = false
			}
			_, err = stopInstanceForCleanup(context.Background(), report, service)
			if (kind == "shared" || kind == "unknown") && err == nil {
				t.Fatal("ambiguous shared service accepted")
			}
			if (kind == "matching" || kind == "unrelated") && err != nil {
				t.Fatal(err)
			}
			if kind == "matching" && service.uninstallRequest.Scope != panelSystemd.ScopeUser {
				t.Fatal("matching service not uninstalled")
			}
			if kind != "matching" && service.uninstallRequest.Scope != "" {
				t.Fatal("unrelated service was changed")
			}
		})
	}
}
