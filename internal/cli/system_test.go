// SPDX-License-Identifier: GPL-3.0-or-later

package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"

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

func TestSystemInstallLoadsSettingsAndReportsResolvedPaths(t *testing.T) {
	settingsPath := commandSettingsFixture(t)
	service := &fakeSystemdService{installResult: panelSystemd.InstallResult{
		Scope: panelSystemd.ScopeUser, Unit: panelSystemd.UnitName,
		UnitPath:       "/home/test/.config/systemd/user/sing-box-panel.service",
		ExecutablePath: "/home/test/.local/bin/sing-box-panel", Enabled: true, Started: true,
	}}
	stdout, stderr, err := executeSystemCommand(t, service,
		"--config", settingsPath, "--output=json", "system", "install", "--scope=user", "--force", "--now",
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

func TestSystemStatusControlAndLogsPreserveOutputContracts(t *testing.T) {
	service := &fakeSystemdService{statusResult: panelSystemd.Status{
		Scope: panelSystemd.ScopeUser, Unit: panelSystemd.UnitName,
		LoadState: "loaded", ActiveState: "active", SubState: "running", UnitFileState: "enabled", MainPID: 321,
	}}
	stdout, stderr, err := executeSystemCommand(t, service, "--output=json", "system", "status", "--scope=user")
	if err != nil || stderr != "" {
		t.Fatalf("status error=%v stderr=%q", err, stderr)
	}
	var status panelSystemd.Status
	if err := json.Unmarshal([]byte(stdout), &status); err != nil {
		t.Fatal(err)
	}
	if status.MainPID != 321 || service.statusScope != panelSystemd.ScopeUser {
		t.Fatalf("status=%+v scope=%q", status, service.statusScope)
	}

	service.controlResult = panelSystemd.ControlResult{Scope: panelSystemd.ScopeSystem, Unit: panelSystemd.UnitName, Action: panelSystemd.ActionRestart}
	stdout, stderr, err = executeSystemCommand(t, service, "system", "restart", "--scope=system")
	if err != nil || stderr != "" || !strings.Contains(stdout, "restart system") {
		t.Fatalf("restart stdout=%q stderr=%q error=%v", stdout, stderr, err)
	}
	if service.controlScope != panelSystemd.ScopeSystem || service.controlAction != panelSystemd.ActionRestart {
		t.Fatalf("control scope=%q action=%q", service.controlScope, service.controlAction)
	}

	service.logsResult = panelSystemd.LogsResult{Scope: panelSystemd.ScopeUser, Unit: panelSystemd.UnitName, Lines: 7, Since: "today", Text: "entry one\nentry two"}
	stdout, stderr, err = executeSystemCommand(t, service, "system", "logs", "--scope=user", "--lines=7", "--since=today")
	if err != nil || stderr != "" || stdout != "entry one\nentry two\n" {
		t.Fatalf("logs stdout=%q stderr=%q error=%v", stdout, stderr, err)
	}
	if service.logsRequest.Lines != 7 || service.logsRequest.Since != "today" {
		t.Fatalf("logs request = %+v", service.logsRequest)
	}
}

func TestSystemErrorsHaveStableExitClasses(t *testing.T) {
	service := &fakeSystemdService{err: panelSystemd.ErrNotInstalled}
	_, _, err := executeSystemCommand(t, service, "system", "status", "--scope=user")
	if ExitCode(err) != 6 {
		t.Fatalf("not installed exit=%d error=%v", ExitCode(err), err)
	}

	settingsPath := commandSettingsFixture(t)
	service.err = panelSystemd.ErrPermission
	_, _, err = executeSystemCommand(t, service, "--config", settingsPath, "system", "install", "--scope=system")
	if ExitCode(err) != 5 {
		t.Fatalf("permission exit=%d error=%v", ExitCode(err), err)
	}

	service.err = errors.New("system bus unavailable")
	_, _, err = executeSystemCommand(t, service, "system", "start", "--scope=system")
	if ExitCode(err) != 6 {
		t.Fatalf("systemctl failure exit=%d error=%v", ExitCode(err), err)
	}

	service.err = nil
	_, _, err = executeSystemCommand(t, service, "system", "logs", "--scope=container")
	if ExitCode(err) != 2 {
		t.Fatalf("invalid scope exit=%d error=%v", ExitCode(err), err)
	}
}

func executeSystemCommand(t *testing.T, service panelSystemd.Service, args ...string) (string, string, error) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	command := NewRootCommand(Dependencies{
		Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr, Systemd: service,
	})
	command.SetArgs(args)
	err := command.ExecuteContext(context.Background())
	return stdout.String(), stderr.String(), err
}
