// SPDX-License-Identifier: GPL-3.0-or-later

package cli

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rehuony/sing-box-panel/internal/application"
	"github.com/rehuony/sing-box-panel/internal/buildinfo"
	"github.com/rehuony/sing-box-panel/internal/selfupdate"
	"github.com/rehuony/sing-box-panel/internal/server"
	"github.com/rehuony/sing-box-panel/internal/settings"
	panelSystemd "github.com/rehuony/sing-box-panel/internal/systemd"
)

func unavailableSettingsFixtures(t *testing.T) map[string]string {
	t.Helper()
	directory := t.TempDir()
	paths := map[string]string{"missing": filepath.Join(directory, "missing.json")}
	for name, content := range map[string]string{"malformed": "{", "invalid": "{}", "unreadable": "{}"} {
		path := filepath.Join(directory, name+".json")
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		if name == "unreadable" {
			if os.Geteuid() == 0 {
				continue // Root can read a file even without read permission.
			}
			if err := os.Chmod(path, 0); err != nil {
				t.Fatal(err)
			}
		}
		paths[name] = path
	}
	return paths
}

func TestCommandsWithoutSettingsDependencies(t *testing.T) {
	commands := [][]string{
		{}, {"--help"}, {"help"}, {"version"},
		{"completion", "bash"}, {"completion", "zsh"}, {"completion", "fish"},
		{"update"},
	}
	for _, action := range []string{"uninstall", "status", "start", "stop", "restart", "logs"} {
		commands = append(commands, []string{"systemd", action, "--scope=user"})
	}
	for _, command := range NewRootCommand(Dependencies{}).Commands() {
		if command.HasAvailableSubCommands() {
			commands = append(commands, []string{command.Name()})
		}
	}
	for _, path := range visibleLeafCapabilities {
		commands = append(commands, append(strings.Fields(path), "--help"))
	}
	for name, path := range unavailableSettingsFixtures(t) {
		t.Run(name, func(t *testing.T) {
			for _, args := range commands {
				t.Run(strings.Join(args, " "), func(t *testing.T) {
					var stdout, stderr bytes.Buffer
					service := &fakeSystemdService{statusResult: panelSystemd.Status{UnitFileSettingsPath: path}}
					root := NewRootCommand(Dependencies{
						Stdin: strings.NewReader("{}"), Stdout: &stdout, Stderr: &stderr,
						Build: buildinfo.Info{Version: "v1.2.3"}, Systemd: service,
						OpenApplication: func(context.Context, string) (*application.Application, error) {
							t.Fatal("command unexpectedly opened application settings and storage")
							return nil, nil
						},
						RunServer: func(context.Context, string) error {
							t.Fatal("command unexpectedly started the server")
							return nil
						},
						Update: func(_ context.Context, version string) (selfupdate.Result, error) {
							return selfupdate.Result{PreviousVersion: version, Version: version}, nil
						},
					})
					root.SetArgs(append([]string{"--config", path}, args...))
					if err := root.ExecuteContext(t.Context()); err != nil || stderr.Len() != 0 || stdout.Len() == 0 {
						t.Fatalf("command failed with unavailable settings: err=%v stdout=%q stderr=%q", err, stdout.String(), stderr.String())
					}
					if service.statusScope != "" && !strings.Contains(stdout.String(), "unreadable or invalid") {
						t.Fatalf("status did not report unavailable installed settings: %s", stdout.String())
					}
				})
			}
		})
	}
}

func TestCommandsRequiringSettingsRejectUnavailableSettings(t *testing.T) {
	for name, path := range unavailableSettingsFixtures(t) {
		t.Run(name, func(t *testing.T) {
			commands := [][]string{
				{"config", "check"}, {"core", "list"},
				{"server", "status"}, {"server", "stop"}, {"systemd", "install"},
				{"system", "prune", "--yes"},
			}
			if name != "missing" {
				commands = append(commands, []string{"system", "df"}, []string{"system", "prune"}, []string{"server", "start"})
			}
			for _, args := range commands {
				t.Run(strings.Join(args, " "), func(t *testing.T) {
					var stdout, stderr bytes.Buffer
					service := &fakeSystemdService{}
					root := NewRootCommand(Dependencies{
						Stdout: &stdout, Stderr: &stderr, Systemd: service, OpenApplication: application.Open,
						RunServer: func(context.Context, string) error {
							t.Fatal("server started with invalid settings")
							return nil
						},
					})
					root.SetArgs(append([]string{"--config", path}, args...))
					err := root.ExecuteContext(t.Context())
					var classified *Error
					if ExitCode(err) != 3 || !errors.As(err, &classified) || stdout.Len() != 0 {
						t.Fatalf("command accepted unavailable settings: err=%v stdout=%q", err, stdout.String())
					}
					if service.installRequest.Scope != "" || service.uninstallRequest.Scope != "" || service.controlScope != "" {
						t.Fatal("command changed service state before validating settings")
					}
				})
			}
		})
	}
}

func TestInstanceCommandsIgnoreInvalidRuntimeSettings(t *testing.T) {
	path := commandSettingsFixture(t)
	configuration, err := settings.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	saveConfigurationFixture(t, path, "{}")
	valid, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	invalid := strings.Replace(string(valid), `"sample_retention_days":90`, `"sample_retention_days":0`, 1)
	if err := os.WriteFile(path, []byte(invalid), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := settings.Load(path); err == nil || !strings.Contains(err.Error(), "sample_retention_days") {
		t.Fatalf("fixture did not reproduce the reported validation error: %v", err)
	}
	for _, args := range [][]string{
		{"system", "df"}, {"system", "prune"}, {"systemd", "install"}, {"systemd", "status"},
		{"server", "status"}, {"server", "stop"},
		{"config", "show"}, {"core", "list"},
		{"channel", "list"}, {"source", "list"}, {"token", "list"},
		{"task", "list"}, {"log", "list"}, {"metrics", "show"}, {"metrics", "history"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			service := &fakeSystemdService{statusResult: panelSystemd.Status{UnitFileSettingsPath: path}}
			root := NewRootCommand(Dependencies{Stdout: &stdout, Stderr: &stderr, Systemd: service, OpenApplication: application.Open})
			root.SetArgs(append([]string{"--config", path, "--output=json"}, args...))
			if err := root.ExecuteContext(t.Context()); err != nil || stderr.Len() != 0 {
				t.Fatalf("unrelated runtime setting blocked command: %v; stderr=%q", err, stderr.String())
			}
			if args[0] == "system" || args[0] == "server" || args[0] == "systemd" && args[1] == "status" {
				if !strings.Contains(stdout.String(), configuration.DataDir) {
					t.Fatalf("command did not resolve the selected data directory: %s", stdout.String())
				}
			}
			if args[0] == "systemd" && args[1] == "install" && (service.installRequest.DataDir != configuration.DataDir || service.installRequest.Now) {
				t.Fatalf("wrong installation request: %+v", service.installRequest)
			}
		})
	}
	for _, args := range [][]string{{"config", "check"}, {"systemd", "install", "--now"}} {
		service := &fakeSystemdService{}
		_, _, err := executeSystemCommand(t, service, append([]string{"--config", path}, args...)...)
		if ExitCode(err) != 3 || !strings.Contains(err.Error(), "sample_retention_days") || service.installRequest.Scope != "" {
			t.Fatalf("runtime validation was skipped: args=%v err=%v", args, err)
		}
	}
	if err := server.Run(t.Context(), path, buildinfo.Info{}, nil); err == nil || !strings.Contains(err.Error(), "sample_retention_days") {
		t.Fatalf("server startup skipped full runtime validation: %v", err)
	}
	after, err := os.ReadFile(path)
	if err != nil || string(after) != invalid {
		t.Fatal("commands changed invalid settings")
	}
}
