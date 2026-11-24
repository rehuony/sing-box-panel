// SPDX-License-Identifier: GPL-3.0-or-later

package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"

	"github.com/rehuony/sing-box-panel/internal/cliui"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/settings"
	panelSystemd "github.com/rehuony/sing-box-panel/internal/sing-box-panel/systemd"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/testutil"
	"github.com/rehuony/sing-box-panel/internal/systemd"
)

type fakeSystemdService struct {
	installRequest   systemd.InstallRequest
	uninstallRequest systemd.UninstallRequest
	statusScope      systemd.Scope
	controlScope     systemd.Scope
	controlAction    systemd.Action
	logsRequest      systemd.LogsRequest

	installResult   systemd.InstallResult[settings.Settings]
	uninstallResult systemd.UninstallResult
	statusResult    systemd.Status
	controlResult   systemd.ControlResult[settings.Settings]
	logsResult      systemd.LogsResult
	err             error
	filesResult     systemd.FilesResult
}

func (service *fakeSystemdService) Files(_ context.Context, _ systemd.Scope) (systemd.FilesResult, error) {
	return service.filesResult, service.err
}

func (service *fakeSystemdService) Install(_ context.Context, request systemd.InstallRequest) (systemd.InstallResult[settings.Settings], error) {
	service.installRequest = request
	return service.installResult, service.err
}

func (service *fakeSystemdService) Uninstall(_ context.Context, request systemd.UninstallRequest) (systemd.UninstallResult, error) {
	service.uninstallRequest = request
	return service.uninstallResult, service.err
}

func (service *fakeSystemdService) Status(_ context.Context, scope systemd.Scope) (systemd.Status, error) {
	service.statusScope = scope
	return service.statusResult, service.err
}

func (service *fakeSystemdService) Control(_ context.Context, scope systemd.Scope, action systemd.Action) (systemd.ControlResult[settings.Settings], error) {
	service.controlScope = scope
	service.controlAction = action
	return service.controlResult, service.err
}

func (service *fakeSystemdService) Logs(_ context.Context, request systemd.LogsRequest) (systemd.LogsResult, error) {
	service.logsRequest = request
	return service.logsResult, service.err
}

func TestSystemdInstallLoadsSettingsAndReportsResolvedPaths(t *testing.T) {
	settingsPath := commandSettingsFixture(t)
	service := &fakeSystemdService{installResult: systemd.InstallResult[settings.Settings]{
		Scope: systemd.ScopeUser, Unit: panelSystemd.UnitName,
		UnitPath:       "/home/test/.config/systemd/user/sing-box-panel.service",
		ExecutablePath: "/home/test/.local/bin/sing-box-panel", Enabled: true, Started: true,
	}}
	stdout, stderr, err := executeSystemCommand(t, service,
		"--config", settingsPath, "--output=json", "systemd", "install", "--scope=user", "--force", "--now",
	)
	if err != nil {
		t.Fatal(err)
	}
	if stderr != "" {
		t.Fatalf("stderr = %q", stderr)
	}
	var result systemd.InstallResult[settings.Settings]
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("decode stdout: %v; %s", err, stdout)
	}
	if result.Scope != systemd.ScopeUser || !result.Enabled || !result.Started {
		t.Fatalf("result = %+v", result)
	}
	if service.installRequest.Scope != systemd.ScopeUser || !service.installRequest.Force || !service.installRequest.Now {
		t.Fatalf("request = %+v", service.installRequest)
	}
	wantSettings, _ := filepath.Abs(settingsPath)
	if service.installRequest.StatePath != wantSettings || !filepath.IsAbs(service.installRequest.DataDir) {
		t.Fatalf("request paths = %+v", service.installRequest)
	}
}

func TestSystemdUninstallAccountOutput(t *testing.T) {
	for _, scenario := range []string{"removed", "retained", "partial failure", "json"} {
		t.Run(scenario, func(t *testing.T) {
			service := &fakeSystemdService{uninstallResult: systemd.UninstallResult{
				Scope: systemd.ScopeSystem, Unit: panelSystemd.UnitName,
				ConfigRetained: true, DataRetained: true, AccountRemoved: true, GroupRemoved: true,
			}}
			args := []string{"systemd", "uninstall", "--scope=system"}
			switch scenario {
			case "retained":
				args = append(args, "--keep-user")
				service.uninstallResult.AccountRemoved, service.uninstallResult.GroupRemoved = false, false
				service.uninstallResult.AccountRetained, service.uninstallResult.GroupRetained = true, true
				service.uninstallResult.AccountNote = "account and ownership retained by --keep-user"
			case "partial failure":
				service.err = errors.New("groupdel failed")
				service.uninstallResult.GroupRemoved, service.uninstallResult.GroupRetained = false, true
			case "json":
				args = append(args, "--output=json")
			}
			stdout, _, err := executeSystemCommand(t, service, args...)
			if (err != nil) != (scenario == "partial failure") || service.uninstallRequest.KeepUser != (scenario == "retained") {
				t.Fatalf("request=%+v, error=%v", service.uninstallRequest, err)
			}
			if scenario == "json" {
				var result systemd.UninstallResult
				if err := json.Unmarshal([]byte(stdout), &result); err != nil || !result.AccountRemoved || !result.GroupRemoved || !result.DataRetained {
					t.Fatalf("result=%+v, error=%v", result, err)
				}
				return
			}
			for _, want := range map[string][]string{
				"removed":         {"  Service user       [removed]", "  Service group      [removed]"},
				"retained":        {"  Service user       [retained]", "  Service group      [retained]", "--keep-user"},
				"partial failure": {"Uninstall interrupted", "  Service user       [removed]", "  Service group      [retained]"},
			}[scenario] {
				if !strings.Contains(stdout, want) {
					t.Fatalf("output missing %q: %s", want, stdout)
				}
			}
		})
	}
}

func TestSystemdStatusLabelsOnDiskSourcesAndNeverClaimsLiveSettings(t *testing.T) {
	settingsPath := commandSettingsFixture(t)
	absoluteSettings, _ := filepath.Abs(settingsPath)
	unit := systemd.Status{
		Scope: systemd.ScopeUser, Unit: panelSystemd.UnitName, UnitPath: "/home/test/.config/systemd/user/sing-box-panel.service",
		LoadState: "loaded", ActiveState: "active", SubState: "running", UnitFileState: "enabled", MainPID: 321,
	}
	decode := func(t *testing.T, stdout string) systemStatusReport {
		t.Helper()
		var report systemStatusReport
		if err := json.Unmarshal([]byte(stdout), &report); err != nil {
			t.Fatalf("decode %s: %v", stdout, err)
		}
		if strings.Contains(stdout, "test-token") {
			t.Fatal("status output leaks settings content")
		}
		if report.LiveSettingsState != "unknown" {
			t.Fatalf("live settings must stay unknown: %+v", report)
		}
		return report
	}

	// Unit file does not state one unambiguous path: unit state stays,
	// storage locations are unknown.
	service := &fakeSystemdService{statusResult: unit}
	stdout, stderr, err := executeSystemCommand(t, service, "--config", settingsPath, "--output=json", "systemd", "status", "--scope=user")
	if err != nil || stderr != "" {
		t.Fatalf("status error=%v stderr=%q", err, stderr)
	}
	report := decode(t, stdout)
	if report.Service.MainPID != 321 || report.Service.ActiveState != "active" || service.statusScope != systemd.ScopeUser {
		t.Fatalf("report=%+v scope=%q", report, service.statusScope)
	}
	if report.CLISettingsPath != absoluteSettings || report.UnitFile.Path != unit.UnitPath || report.UnitFile.Source != "unit file on disk" ||
		report.UnitFile.Stale || report.UnitFile.SettingsPath != "" || report.UnitFile.SettingsState != "unknown" || report.UnitFile.MatchesCLISettings != nil ||
		report.SettingsFile.Source != "settings file on disk" || report.SettingsFile.State != "unknown" || report.SettingsFile.Path != "" ||
		report.SettingsFile.DataDir != "" || report.SettingsFile.DatabasePath != "" {
		t.Fatalf("unknown unit settings report=%+v", report)
	}
	if report.Configuration.Name != "config.json" || report.Configuration.Table != "node_configurations" || report.Configuration.DatabasePath != "" {
		t.Fatalf("configuration location=%+v", report.Configuration)
	}
	// Aligned fields: compare with runs of spaces collapsed.
	spaces := regexp.MustCompile(" +")
	text, _, err := executeSystemCommand(t, service, "--config", settingsPath, "systemd", "status", "--scope=user")
	if err != nil {
		t.Fatal(err)
	}
	text = spaces.ReplaceAllString(text, " ")
	for _, want := range []string{
		"Settings in unit file unknown", "Live settings unknown", "on disk; systemd reports no daemon reload needed",
		"each node's config.json stored in sqlite table node_configurations of the service database",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("status text %q lacks %q", text, want)
		}
	}

	// Unit file names the same file the CLI selected and it loads.
	unit.UnitFileStatePath = absoluteSettings
	service = &fakeSystemdService{statusResult: unit}
	stdout, _, err = executeSystemCommand(t, service, "--config", settingsPath, "--output=json", "systemd", "status", "--scope=user")
	if err != nil {
		t.Fatal(err)
	}
	report = decode(t, stdout)
	loaded, _ := settings.Load(settingsPath)
	if report.UnitFile.SettingsPath != absoluteSettings || report.UnitFile.SettingsState != "parsed" ||
		report.UnitFile.MatchesCLISettings == nil || !*report.UnitFile.MatchesCLISettings ||
		report.SettingsFile.Path != absoluteSettings || report.SettingsFile.State != "loaded" || report.SettingsFile.DataDir != loaded.DataDir ||
		report.SettingsFile.DatabasePath != filepath.Join(loaded.DataDir, "panel.db") || report.Configuration.DatabasePath != report.SettingsFile.DatabasePath {
		t.Fatalf("loaded report=%+v", report)
	}
	text, _, err = executeSystemCommand(t, service, "--config", settingsPath, "systemd", "status", "--scope=user")
	if err != nil {
		t.Fatal(err)
	}
	text = spaces.ReplaceAllString(text, " ")
	for _, want := range []string{
		"PID 321", unit.UnitPath, "(unit file on disk; same as --config)", "Live settings unknown",
		loaded.DataDir + " (settings file on disk)",
		"each node's config.json stored in sqlite table node_configurations of " + filepath.Join(loaded.DataDir, "panel.db"),
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("status text %q lacks %q", text, want)
		}
	}
	for _, forbidden := range []string{"effective", "actual", "verified"} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("status text %q claims %q", text, forbidden)
		}
	}

	// Stale unit: the on-disk path is still labeled as on-disk and the
	// daemon-reload requirement is surfaced instead of calling it loaded.
	unit.NeedDaemonReload = true
	service = &fakeSystemdService{statusResult: unit}
	stdout, _, err = executeSystemCommand(t, service, "--config", settingsPath, "--output=json", "systemd", "status", "--scope=user")
	if err != nil {
		t.Fatal(err)
	}
	report = decode(t, stdout)
	if !report.UnitFile.Stale || !report.Service.NeedDaemonReload || report.UnitFile.SettingsPath != absoluteSettings {
		t.Fatalf("stale report=%+v", report)
	}
	text, _, err = executeSystemCommand(t, service, "--config", settingsPath, "systemd", "status", "--scope=user")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text, "on disk; systemd reports daemon reload needed; restart to apply command changes") || strings.Contains(text, "no daemon reload needed") {
		t.Fatalf("stale status text %q", text)
	}
	unit.NeedDaemonReload = false

	// Unit file names a different, unreadable settings file: unit state and
	// both paths are kept, storage stays unknown, no parse detail is shown.
	unit.UnitFileStatePath = filepath.Join(t.TempDir(), "missing.json")
	service = &fakeSystemdService{statusResult: unit}
	stdout, _, err = executeSystemCommand(t, service, "--config", settingsPath, "--output=json", "systemd", "status", "--scope=user")
	if err != nil {
		t.Fatal(err)
	}
	report = decode(t, stdout)
	if report.Service.MainPID != 321 || report.UnitFile.SettingsPath != unit.UnitFileStatePath || report.UnitFile.MatchesCLISettings == nil || *report.UnitFile.MatchesCLISettings ||
		report.SettingsFile.Path != unit.UnitFileStatePath || report.SettingsFile.State != "unavailable" || report.SettingsFile.DataDir != "" ||
		strings.Contains(stdout, "no such file") {
		t.Fatalf("unavailable report=%+v stdout=%s", report, stdout)
	}
}

func TestSystemdControlAndLogsPreserveOutputContracts(t *testing.T) {
	service := &fakeSystemdService{controlResult: systemd.ControlResult[settings.Settings]{
		Scope: systemd.ScopeSystem, Unit: panelSystemd.UnitName, Action: systemd.ActionRestart,
	}}
	stdout, stderr, err := executeSystemCommand(t, service, "systemd", "restart", "--scope=system")
	if err != nil || stderr != "" || !strings.Contains(stdout, "Restarted system sing-box-panel.service") {
		t.Fatalf("restart stdout=%q stderr=%q error=%v", stdout, stderr, err)
	}
	if service.controlScope != systemd.ScopeSystem || service.controlAction != systemd.ActionRestart {
		t.Fatalf("control scope=%q action=%q", service.controlScope, service.controlAction)
	}

	service.logsResult = systemd.LogsResult{Scope: systemd.ScopeUser, Unit: panelSystemd.UnitName, Lines: 7, Since: "today", Text: "entry one\nentry two"}
	stdout, stderr, err = executeSystemCommand(t, service, "systemd", "logs", "--scope=user", "--lines=7", "--since=today")
	if err != nil || stderr != "" || stdout != "entry one\nentry two\n" {
		t.Fatalf("logs stdout=%q stderr=%q error=%v", stdout, stderr, err)
	}
	if service.logsRequest.Lines != 7 || service.logsRequest.Since != "today" {
		t.Fatalf("logs request = %+v", service.logsRequest)
	}
}

func TestSystemdErrorsHaveStableExitClasses(t *testing.T) {
	service := &fakeSystemdService{err: systemd.ErrNotInstalled}
	_, _, err := executeSystemCommand(t, service, "systemd", "status", "--scope=user")
	if cliui.ExitCode(err) != 6 {
		t.Fatalf("not installed exit=%d error=%v", cliui.ExitCode(err), err)
	}

	settingsPath := commandSettingsFixture(t)
	service.err = systemd.ErrPermission
	_, _, err = executeSystemCommand(t, service, "--config", settingsPath, "systemd", "install", "--scope=system")
	if cliui.ExitCode(err) != 5 {
		t.Fatalf("permission exit=%d error=%v", cliui.ExitCode(err), err)
	}

	service.err = errors.New("system bus unavailable")
	_, _, err = executeSystemCommand(t, service, "systemd", "start", "--scope=system")
	if cliui.ExitCode(err) != 6 {
		t.Fatalf("systemctl failure exit=%d error=%v", cliui.ExitCode(err), err)
	}

	service.err = nil
	_, _, err = executeSystemCommand(t, service, "systemd", "logs", "--scope=container")
	if cliui.ExitCode(err) != 2 {
		t.Fatalf("invalid scope exit=%d error=%v", cliui.ExitCode(err), err)
	}
}

func executeSystemCommand(t *testing.T, service systemdService, args ...string) (string, string, error) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	command := NewRootCommand(Dependencies{CleanupHistoryPath: testHistoryPath(t),
		Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr, Systemd: service,
	})
	command.SetArgs(args)
	err := command.ExecuteContext(context.Background())
	return stdout.String(), stderr.String(), err
}

var testHistoryPaths sync.Map

func testHistoryPath(t *testing.T) string {
	t.Helper()
	if path, exists := testHistoryPaths.Load(t); exists {
		return path.(string)
	}
	path := filepath.Join(t.TempDir(), "state", "cleanup-history.json")
	testHistoryPaths.Store(t, path)
	t.Cleanup(func() { testHistoryPaths.Delete(t) })
	return path
}

func TestSystemdInitialCredentialsSurviveActivationFailure(t *testing.T) {
	for _, action := range []string{"install", "start", "restart"} {
		for _, format := range []string{"text", "json", "jsonl"} {
			t.Run(action+"/"+format, func(t *testing.T) {
				path := commandSettingsFixture(t)
				value, err := settings.Load(path)
				if err != nil {
					t.Fatal(err)
				}
				value.InitialPassword = testutil.AdminPassword
				service := &fakeSystemdService{
					installResult: systemd.InstallResult[settings.Settings]{Initial: &value},
					controlResult: systemd.ControlResult[settings.Settings]{Initial: &value},
					err:           errors.New("service activation failed"),
				}
				stdout, stderr, err := executeSystemCommand(t, service, "--config", path, "--output", format, "systemd", action, "--scope=user")
				if err == nil || stdout != "" {
					t.Fatal("activation failure claimed success")
				}
				initialPasswordFromOutput(t, stderr, value.Auth.PasswordHash)
			})
		}
	}
}
