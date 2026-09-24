// SPDX-License-Identifier: GPL-3.0-or-later

package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/rehuony/sing-box-panel/internal/settings"
	panelSystemd "github.com/rehuony/sing-box-panel/internal/systemd"
)

type fakeSystemdService struct {
	installRequest   panelSystemd.InstallRequest
	uninstallRequest panelSystemd.UninstallRequest
	statusScope      panelSystemd.Scope
	controlScope     panelSystemd.Scope
	controlAction    panelSystemd.Action
	logsRequest      panelSystemd.LogsRequest

	installResult   panelSystemd.InstallResult
	uninstallResult panelSystemd.UninstallResult
	statusResult    panelSystemd.Status
	controlResult   panelSystemd.ControlResult
	logsResult      panelSystemd.LogsResult
	err             error
	filesResult     panelSystemd.FilesResult
}

func (service *fakeSystemdService) Files(_ context.Context, _ panelSystemd.Scope) (panelSystemd.FilesResult, error) {
	return service.filesResult, service.err
}

func (service *fakeSystemdService) Install(_ context.Context, request panelSystemd.InstallRequest) (panelSystemd.InstallResult, error) {
	service.installRequest = request
	return service.installResult, service.err
}

func (service *fakeSystemdService) Uninstall(_ context.Context, request panelSystemd.UninstallRequest) (panelSystemd.UninstallResult, error) {
	service.uninstallRequest = request
	return service.uninstallResult, service.err
}

func (service *fakeSystemdService) Status(_ context.Context, scope panelSystemd.Scope) (panelSystemd.Status, error) {
	service.statusScope = scope
	return service.statusResult, service.err
}

func (service *fakeSystemdService) Control(_ context.Context, scope panelSystemd.Scope, action panelSystemd.Action) (panelSystemd.ControlResult, error) {
	service.controlScope = scope
	service.controlAction = action
	return service.controlResult, service.err
}

func (service *fakeSystemdService) Logs(_ context.Context, request panelSystemd.LogsRequest) (panelSystemd.LogsResult, error) {
	service.logsRequest = request
	return service.logsResult, service.err
}

func TestSystemdInstallLoadsSettingsAndReportsResolvedPaths(t *testing.T) {
	settingsPath := commandSettingsFixture(t)
	service := &fakeSystemdService{installResult: panelSystemd.InstallResult{
		Scope: panelSystemd.ScopeUser, Unit: panelSystemd.UnitName,
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
	var result panelSystemd.InstallResult
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("decode stdout: %v; %s", err, stdout)
	}
	if result.Scope != panelSystemd.ScopeUser || !result.Enabled || !result.Started {
		t.Fatalf("result = %+v", result)
	}
	if service.installRequest.Scope != panelSystemd.ScopeUser || !service.installRequest.Force || !service.installRequest.Now {
		t.Fatalf("request = %+v", service.installRequest)
	}
	wantSettings, _ := filepath.Abs(settingsPath)
	if service.installRequest.SettingsPath != wantSettings || !filepath.IsAbs(service.installRequest.DataDir) {
		t.Fatalf("request paths = %+v", service.installRequest)
	}
}

func TestSystemdStatusLabelsOnDiskSourcesAndNeverClaimsLiveSettings(t *testing.T) {
	settingsPath := commandSettingsFixture(t)
	absoluteSettings, _ := filepath.Abs(settingsPath)
	unit := panelSystemd.Status{
		Scope: panelSystemd.ScopeUser, Unit: panelSystemd.UnitName, UnitPath: "/home/test/.config/systemd/user/sing-box-panel.service",
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
	if report.Service.MainPID != 321 || report.Service.ActiveState != "active" || service.statusScope != panelSystemd.ScopeUser {
		t.Fatalf("report=%+v scope=%q", report, service.statusScope)
	}
	if report.CLISettingsPath != absoluteSettings || report.UnitFile.Path != unit.UnitPath || report.UnitFile.Source != "unit file on disk" ||
		report.UnitFile.Stale || report.UnitFile.SettingsPath != "" || report.UnitFile.SettingsState != "unknown" || report.UnitFile.MatchesCLISettings != nil ||
		report.SettingsFile.Source != "settings file on disk" || report.SettingsFile.State != "unknown" || report.SettingsFile.Path != "" ||
		report.SettingsFile.DataDir != "" || report.SettingsFile.DatabasePath != "" {
		t.Fatalf("unknown unit settings report=%+v", report)
	}
	if report.Configuration.Name != "config.json" || report.Configuration.Table != "configuration_file" || report.Configuration.DatabasePath != "" {
		t.Fatalf("configuration location=%+v", report.Configuration)
	}
	text, _, err := executeSystemCommand(t, service, "--config", settingsPath, "systemd", "status", "--scope=user")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"settings in unit file\tunknown", "live settings\tunknown", "on disk; systemd reports no daemon reload needed"} {
		if !strings.Contains(text, want) {
			t.Fatalf("status text %q lacks %q", text, want)
		}
	}

	// Unit file names the same file the CLI selected and it loads.
	unit.UnitFileSettingsPath = absoluteSettings
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
	for _, want := range []string{
		"pid=321", unit.UnitPath, "(unit file on disk; same as --config)", "live settings\tunknown",
		loaded.DataDir + " (settings file on disk)", "config.json stored in sqlite table configuration_file",
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
	unit.UnitFileSettingsPath = filepath.Join(t.TempDir(), "missing.json")
	service = &fakeSystemdService{statusResult: unit}
	stdout, _, err = executeSystemCommand(t, service, "--config", settingsPath, "--output=json", "systemd", "status", "--scope=user")
	if err != nil {
		t.Fatal(err)
	}
	report = decode(t, stdout)
	if report.Service.MainPID != 321 || report.UnitFile.SettingsPath != unit.UnitFileSettingsPath || report.UnitFile.MatchesCLISettings == nil || *report.UnitFile.MatchesCLISettings ||
		report.SettingsFile.Path != unit.UnitFileSettingsPath || report.SettingsFile.State != "unavailable" || report.SettingsFile.DataDir != "" ||
		strings.Contains(stdout, "no such file") {
		t.Fatalf("unavailable report=%+v stdout=%s", report, stdout)
	}
}

func TestSystemdControlAndLogsPreserveOutputContracts(t *testing.T) {
	service := &fakeSystemdService{controlResult: panelSystemd.ControlResult{
		Scope: panelSystemd.ScopeSystem, Unit: panelSystemd.UnitName, Action: panelSystemd.ActionRestart,
	}}
	stdout, stderr, err := executeSystemCommand(t, service, "systemd", "restart", "--scope=system")
	if err != nil || stderr != "" || !strings.Contains(stdout, "restart system") {
		t.Fatalf("restart stdout=%q stderr=%q error=%v", stdout, stderr, err)
	}
	if service.controlScope != panelSystemd.ScopeSystem || service.controlAction != panelSystemd.ActionRestart {
		t.Fatalf("control scope=%q action=%q", service.controlScope, service.controlAction)
	}

	service.logsResult = panelSystemd.LogsResult{Scope: panelSystemd.ScopeUser, Unit: panelSystemd.UnitName, Lines: 7, Since: "today", Text: "entry one\nentry two"}
	stdout, stderr, err = executeSystemCommand(t, service, "systemd", "logs", "--scope=user", "--lines=7", "--since=today")
	if err != nil || stderr != "" || stdout != "entry one\nentry two\n" {
		t.Fatalf("logs stdout=%q stderr=%q error=%v", stdout, stderr, err)
	}
	if service.logsRequest.Lines != 7 || service.logsRequest.Since != "today" {
		t.Fatalf("logs request = %+v", service.logsRequest)
	}
}

func TestSystemdErrorsHaveStableExitClasses(t *testing.T) {
	service := &fakeSystemdService{err: panelSystemd.ErrNotInstalled}
	_, _, err := executeSystemCommand(t, service, "systemd", "status", "--scope=user")
	if ExitCode(err) != 6 {
		t.Fatalf("not installed exit=%d error=%v", ExitCode(err), err)
	}

	settingsPath := commandSettingsFixture(t)
	service.err = panelSystemd.ErrPermission
	_, _, err = executeSystemCommand(t, service, "--config", settingsPath, "systemd", "install", "--scope=system")
	if ExitCode(err) != 5 {
		t.Fatalf("permission exit=%d error=%v", ExitCode(err), err)
	}

	service.err = errors.New("system bus unavailable")
	_, _, err = executeSystemCommand(t, service, "systemd", "start", "--scope=system")
	if ExitCode(err) != 6 {
		t.Fatalf("systemctl failure exit=%d error=%v", ExitCode(err), err)
	}

	service.err = nil
	_, _, err = executeSystemCommand(t, service, "systemd", "logs", "--scope=container")
	if ExitCode(err) != 2 {
		t.Fatalf("invalid scope exit=%d error=%v", ExitCode(err), err)
	}
}

func executeSystemCommand(t *testing.T, service panelSystemd.Service, args ...string) (string, string, error) {
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
