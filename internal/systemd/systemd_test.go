// SPDX-License-Identifier: GPL-3.0-or-later

package systemd

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

type recordedCommand struct {
	name string
	args []string
}

type recordingRunner struct {
	calls []recordedCommand
	run   func(string, []string) (CommandResult, error)
}

func (runner *recordingRunner) Run(_ context.Context, name string, args ...string) (CommandResult, error) {
	runner.calls = append(runner.calls, recordedCommand{name: name, args: append([]string(nil), args...)})
	if runner.run != nil {
		return runner.run(name, args)
	}
	return CommandResult{}, nil
}

type managerFixture struct {
	manager    *Manager
	runner     *recordingRunner
	layout     Layout
	executable string
	settings   string
	data       string
}

func TestUserInstallAndUninstallUseAuditedArguments(t *testing.T) {
	fixture := newManagerFixture(t, 1000)
	result, err := fixture.manager.Install(context.Background(), InstallRequest{
		Scope: ScopeAuto, SettingsPath: fixture.settings, DataDir: fixture.data, Now: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Scope != ScopeUser || !result.Enabled || !result.Started || result.UnitPath != fixture.layout.UserUnitPath {
		t.Fatalf("Install() = %+v", result)
	}
	wantCalls := []recordedCommand{
		{name: "systemctl", args: []string{"--no-ask-password", "--user", "daemon-reload"}},
		{name: "systemctl", args: []string{"--no-ask-password", "--user", "enable", "--now", UnitName}},
	}
	if !reflect.DeepEqual(fixture.runner.calls, wantCalls) {
		t.Fatalf("commands = %#v, want %#v", fixture.runner.calls, wantCalls)
	}
	unit, err := os.ReadFile(fixture.layout.UserUnitPath)
	if err != nil {
		t.Fatal(err)
	}
	text := string(unit)
	for _, value := range []string{
		managedMark,
		`ExecStart="` + escapedUnitPath(fixture.executable) + `" server start --config "` + escapedUnitPath(fixture.settings) + `"`,
		`Environment=SING_BOX_PANEL_SUPERVISOR=systemd`,
		`WorkingDirectory="` + escapedPathDirective(fixture.data) + `"`,
		`ReadWritePaths="` + escapedPathDirective(fixture.data) + `"`,
	} {
		if !strings.Contains(text, value) {
			t.Fatalf("unit does not contain %q:\n%s", value, text)
		}
	}

	fixture.runner.calls = nil
	uninstalled, err := fixture.manager.Uninstall(context.Background(), UninstallRequest{Scope: ScopeUser})
	if err != nil {
		t.Fatal(err)
	}
	if !uninstalled.ConfigRetained || !uninstalled.DataRetained || uninstalled.AccountRetained {
		t.Fatalf("Uninstall() = %+v", uninstalled)
	}
	wantCalls = []recordedCommand{
		{name: "systemctl", args: []string{"--no-ask-password", "--user", "disable", "--now", UnitName}},
		{name: "systemctl", args: []string{"--no-ask-password", "--user", "daemon-reload"}},
	}
	if !reflect.DeepEqual(fixture.runner.calls, wantCalls) {
		t.Fatalf("commands = %#v, want %#v", fixture.runner.calls, wantCalls)
	}
	if _, err := os.Stat(fixture.layout.UserUnitPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("unit still exists: %v", err)
	}
	for _, retained := range []string{fixture.settings, fixture.data} {
		if _, err := os.Stat(retained); err != nil {
			t.Fatalf("retained path %q: %v", retained, err)
		}
	}
}

func TestInstallConflictRequiresForceAndNeverFollowsSymlink(t *testing.T) {
	fixture := newManagerFixture(t, 1000)
	if err := os.MkdirAll(filepath.Dir(fixture.layout.UserUnitPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fixture.layout.UserUnitPath, []byte("unmanaged\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	request := InstallRequest{Scope: ScopeUser, SettingsPath: fixture.settings, DataDir: fixture.data}
	if _, err := fixture.manager.Install(context.Background(), request); !errors.Is(err, ErrConflict) {
		t.Fatalf("Install() error = %v, want conflict", err)
	}
	if len(fixture.runner.calls) != 0 {
		t.Fatalf("commands ran before conflict resolution: %#v", fixture.runner.calls)
	}
	request.Force = true
	if _, err := fixture.manager.Install(context.Background(), request); err != nil {
		t.Fatalf("forced Install(): %v", err)
	}
	if err := os.Chmod(fixture.layout.UserUnitPath, 0o600); err != nil {
		t.Fatal(err)
	}
	request.Force = false
	repaired, err := fixture.manager.Install(context.Background(), request)
	if err != nil {
		t.Fatalf("repair mode Install(): %v", err)
	}
	if len(repaired.InstalledPaths) != 1 || repaired.InstalledPaths[0] != fixture.layout.UserUnitPath {
		t.Fatalf("mode repair paths = %#v", repaired.InstalledPaths)
	}
	info, err := os.Stat(fixture.layout.UserUnitPath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o644 {
		t.Fatalf("repaired unit mode=%v", info.Mode().Perm())
	}

	if _, err := fixture.manager.Uninstall(context.Background(), UninstallRequest{Scope: ScopeUser}); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(filepath.Dir(fixture.layout.UserUnitPath), "target")
	if err := os.WriteFile(target, []byte("do not replace"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, fixture.layout.UserUnitPath); err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.manager.Install(context.Background(), request); !errors.Is(err, ErrConflict) {
		t.Fatalf("forced symlink Install() error = %v, want conflict", err)
	}
	data, err := os.ReadFile(target)
	if err != nil || string(data) != "do not replace" {
		t.Fatalf("symlink target changed: data=%q err=%v", data, err)
	}
}

func TestSystemInstallRequiresRootAndConventionalLayout(t *testing.T) {
	nonRoot := newManagerFixture(t, 1000)
	_, err := nonRoot.manager.Install(context.Background(), InstallRequest{Scope: ScopeSystem})
	if !errors.Is(err, ErrPermission) {
		t.Fatalf("non-root Install() error = %v", err)
	}
	if len(nonRoot.runner.calls) != 0 {
		t.Fatalf("non-root commands = %#v", nonRoot.runner.calls)
	}

	fixture := newManagerFixture(t, 0)
	result, err := fixture.manager.Install(context.Background(), InstallRequest{
		Scope: ScopeSystem, SettingsPath: fixture.settings, DataDir: fixture.data,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Scope != ScopeSystem || result.Started || len(result.InstalledPaths) != 3 {
		t.Fatalf("Install() = %+v", result)
	}
	wantCalls := []recordedCommand{
		{name: "systemd-sysusers", args: []string{fixture.layout.SystemSysusersPath}},
		{name: "systemd-tmpfiles", args: []string{"--create", fixture.layout.SystemTmpfilesPath}},
		{name: "chown", args: []string{"root:" + serviceGroup, fixture.settings}},
		{name: "chown", args: []string{"--recursive", "--no-dereference", serviceUser + ":" + serviceGroup, fixture.data}},
		{name: "systemctl", args: []string{"--no-ask-password", "daemon-reload"}},
		{name: "systemctl", args: []string{"--no-ask-password", "enable", UnitName}},
	}
	if !reflect.DeepEqual(fixture.runner.calls, wantCalls) {
		t.Fatalf("commands = %#v, want %#v", fixture.runner.calls, wantCalls)
	}
	info, err := os.Stat(fixture.settings)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o640 {
		t.Fatalf("settings mode = %o, want 640", got)
	}

	request := InstallRequest{Scope: ScopeSystem, SettingsPath: fixture.settings, DataDir: filepath.Join(fixture.data, "other")}
	if _, err := fixture.manager.Install(context.Background(), request); !errors.Is(err, ErrInvalid) {
		t.Fatalf("non-conventional Install() error = %v", err)
	}
}

func TestStatusParsesExactPropertiesAndReportsMissingUnit(t *testing.T) {
	fixture := newManagerFixture(t, 1000)
	fixture.runner.run = func(name string, args []string) (CommandResult, error) {
		return CommandResult{Stdout: []byte("LoadState=loaded\nActiveState=active\nSubState=running\nUnitFileState=enabled\nMainPID=42\nFragmentPath=/usr/lib/systemd/system/sing-box-panel.service\n")}, nil
	}
	status, err := fixture.manager.Status(context.Background(), ScopeAuto)
	if err != nil {
		t.Fatal(err)
	}
	if status.Scope != ScopeUser || status.LoadState != "loaded" || status.ActiveState != "active" || status.MainPID != 42 || status.UnitPath != "/usr/lib/systemd/system/sing-box-panel.service" {
		t.Fatalf("Status() = %+v", status)
	}
	if status.UnitFileSettingsPath != "" || status.NeedDaemonReload {
		t.Fatalf("unreadable fragment must not yield a settings path: %+v", status)
	}
	want := recordedCommand{name: "systemctl", args: []string{
		"--no-ask-password", "--user", "show", UnitName, "--no-pager",
		"--property=LoadState", "--property=ActiveState", "--property=SubState",
		"--property=UnitFileState", "--property=MainPID", "--property=FragmentPath",
		"--property=DropInPaths", "--property=NeedDaemonReload",
	}}
	if !reflect.DeepEqual(fixture.runner.calls[0], want) {
		t.Fatalf("status command = %#v, want %#v", fixture.runner.calls[0], want)
	}

	fixture.runner.run = func(string, []string) (CommandResult, error) {
		return CommandResult{Stdout: []byte("LoadState=not-found\nActiveState=inactive\nSubState=dead\nUnitFileState=\nMainPID=0\nFragmentPath=\n")}, nil
	}
	if _, err := fixture.manager.Status(context.Background(), ScopeUser); !errors.Is(err, ErrNotInstalled) {
		t.Fatalf("missing Status() error = %v", err)
	}
}

func TestStatusReportsUnitFileSettingsPathAndDaemonReloadState(t *testing.T) {
	fixture := newManagerFixture(t, 1000)
	if _, err := fixture.manager.Install(context.Background(), InstallRequest{
		Scope: ScopeUser, SettingsPath: fixture.settings, DataDir: fixture.data,
	}); err != nil {
		t.Fatal(err)
	}
	properties := "LoadState=loaded\nActiveState=active\nSubState=running\nUnitFileState=enabled\nMainPID=42\nFragmentPath=" + fixture.layout.UserUnitPath + "\nDropInPaths=%s\nNeedDaemonReload=%r\n"
	respond := func(dropIns, reload string) {
		fixture.runner.run = func(string, []string) (CommandResult, error) {
			stdout := strings.ReplaceAll(strings.ReplaceAll(properties, "%s", dropIns), "%r", reload)
			return CommandResult{Stdout: []byte(stdout)}, nil
		}
	}

	respond("", "no")
	status, err := fixture.manager.Status(context.Background(), ScopeUser)
	if err != nil {
		t.Fatal(err)
	}
	if status.UnitFileSettingsPath != fixture.settings || status.NeedDaemonReload {
		t.Fatalf("Status() = %+v, want unit file settings %q", status, fixture.settings)
	}

	// The file on disk was edited after systemd loaded the unit: the path it
	// now names is still reported as the on-disk value, and the stale state is
	// surfaced so callers do not present it as the loaded command line.
	editedSettings := filepath.Join(fixture.data, "edited.json")
	edited := strings.Replace(mustRead(t, fixture.layout.UserUnitPath), escapedUnitPath(fixture.settings), escapedUnitPath(editedSettings), 1)
	if edited == mustRead(t, fixture.layout.UserUnitPath) {
		t.Fatal("test did not rewrite the installed ExecStart line")
	}
	if err := os.WriteFile(fixture.layout.UserUnitPath, []byte(edited), 0o644); err != nil {
		t.Fatal(err)
	}
	respond("", "yes")
	status, err = fixture.manager.Status(context.Background(), ScopeUser)
	if err != nil {
		t.Fatal(err)
	}
	if status.UnitFileSettingsPath != editedSettings || !status.NeedDaemonReload {
		t.Fatalf("stale Status() = %+v", status)
	}

	respond("/etc/systemd/system/sing-box-panel.service.d/override.conf", "no")
	status, err = fixture.manager.Status(context.Background(), ScopeUser)
	if err != nil {
		t.Fatal(err)
	}
	if status.UnitFileSettingsPath != "" {
		t.Fatalf("drop-in override must leave the settings path unknown, got %q", status.UnitFileSettingsPath)
	}
}

func TestParseUnitFileSettingsPathHandlesQuotingAndRefusesAmbiguity(t *testing.T) {
	cases := []struct {
		name string
		unit string
		want string
		ok   bool
	}{
		{name: "plain", unit: "[Unit]\nDescription=x\n[Service]\nExecStart=/usr/local/bin/sing-box-panel server start --config /etc/sing-box-panel/setting.json\n[Install]\nWantedBy=default.target\n", want: "/etc/sing-box-panel/setting.json", ok: true},
		{name: "quoted escapes", unit: "[Service]\nExecStart=\"/opt/bin/panel\" server start --config \"/tmp/paths with space;$$value%%/set\\\"ting\\\\.json\"\n", want: `/tmp/paths with space;$value%/set"ting\.json`, ok: true},
		{name: "equals form and prefix", unit: "[Service]\nExecStart=-/opt/bin/panel server start --config=/etc/a.json\n", want: "/etc/a.json", ok: true},
		{name: "continuation and reset", unit: "[Service]\nExecStart=/x --config /old.json\nExecStart=\nExecStart=/y server \\\n  --config /new.json\n", want: "/new.json", ok: true},
		{name: "single quotes shorthand", unit: "[Service]\nExecStart='/opt/bin/panel' server -c '/etc/b.json'\n", want: "/etc/b.json", ok: true},
		{name: "comments inside continuation", unit: `[Service]
ExecStart=/x --config /real.json \
# --config /comment.json
; --config /another-comment.json
  server
`, want: "/real.json", ok: true},
		{name: "attached shorthand", unit: "[Service]\nExecStart=/x server -c/etc/c.json\n", want: "/etc/c.json", ok: true},
		{name: "repeated flag last value wins", unit: "[Service]\nExecStart=/x server start --config /first.json --config /second.json\n", want: "/second.json", ok: true},
		{name: "double dash ends flags", unit: "[Service]\nExecStart=/x server start --config /kept.json -- --config /ignored.json\n", want: "/kept.json", ok: true},
		{name: "repeated flag with unresolvable value", unit: "[Service]\nExecStart=/x server start --config %h/a.json --config /b.json\n"},
		{name: "two effective commands", unit: "[Service]\nType=oneshot\nExecStart=/x server start --config /a.json\nExecStart=/x server start --config /b.json\n"},
		{name: "exec start outside service section", unit: "[Unit]\nExecStart=/x server start --config /a.json\n"},
		{name: "exec start before any section", unit: "ExecStart=/x server start --config /a.json\n"},
		{name: "unresolved specifier", unit: "[Service]\nExecStart=%h/.local/bin/sing-box-panel server start --config %h/.config/sing-box-panel/setting.json\n"},
		{name: "unresolved variable", unit: "[Service]\nExecStart=/x server start --config ${CONFIG}\n"},
		{name: "relative path", unit: "[Service]\nExecStart=/x server start --config setting.json\n"},
		{name: "missing value", unit: "[Service]\nExecStart=/x server start --config\n"},
		{name: "unterminated quote", unit: "[Service]\nExecStart=/x server start --config \"/etc/a.json\n"},
		{name: "no exec start", unit: "[Service]\nType=simple\n"},
		{name: "commented", unit: "[Service]\n# ExecStart=/x server start --config /etc/a.json\n"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			got, ok := parseUnitFileSettingsPath([]byte(testCase.unit))
			if ok != testCase.ok || got != testCase.want {
				t.Fatalf("parse = %q, %t; want %q, %t", got, ok, testCase.want, testCase.ok)
			}
		})
	}
}

func mustRead(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestLogsPassesSinceAsOneArgumentAndPreservesCommandFailure(t *testing.T) {
	fixture := newManagerFixture(t, 1000)
	fixture.runner.run = func(name string, args []string) (CommandResult, error) {
		if name != "journalctl" {
			t.Fatalf("name = %q", name)
		}
		return CommandResult{Stderr: []byte("journal unavailable\n")}, errors.New("exit status 1")
	}
	since := "2026-08-26 12:00:00; touch /tmp/not-run"
	_, err := fixture.manager.Logs(context.Background(), LogsRequest{Scope: ScopeUser, Lines: 20, Since: since})
	var commandErr *CommandError
	if !errors.As(err, &commandErr) || commandErr.Stderr != "journal unavailable" {
		t.Fatalf("Logs() error = %#v", err)
	}
	call := fixture.runner.calls[0]
	wantArgs := []string{"--no-pager", "--output=short-iso", "--lines=20", "--user-unit=" + UnitName, "--since", since}
	if !reflect.DeepEqual(call.args, wantArgs) {
		t.Fatalf("journal args = %#v, want %#v", call.args, wantArgs)
	}
}

func TestManagerIsLinuxOnly(t *testing.T) {
	fixture := newManagerFixture(t, 1000)
	fixture.manager.goos = "darwin"
	if _, err := fixture.manager.Control(context.Background(), ScopeUser, ActionStart); !errors.Is(err, ErrUnsupportedOS) {
		t.Fatalf("Control() error = %v", err)
	}
	if len(fixture.runner.calls) != 0 {
		t.Fatalf("commands = %#v", fixture.runner.calls)
	}
}

func TestFilesInspectsManagedDestinationsWithoutRunningCommands(t *testing.T) {
	fixture := newManagerFixture(t, 1000)
	if _, err := fixture.manager.Install(context.Background(), InstallRequest{Scope: ScopeUser, SettingsPath: fixture.settings, DataDir: fixture.data}); err != nil {
		t.Fatal(err)
	}
	fixture.runner.calls = nil
	files, err := fixture.manager.Files(context.Background(), ScopeUser)
	if err != nil {
		t.Fatal(err)
	}
	if files.SettingsPath != fixture.settings || len(files.Files) != 1 || !files.Files[0].Managed {
		t.Fatalf("files=%+v", files)
	}
	if len(fixture.runner.calls) != 0 {
		t.Fatal("inspection ran a system command")
	}
	if err := os.WriteFile(fixture.layout.UserUnitPath, []byte("unmanaged"), 0o644); err != nil {
		t.Fatal(err)
	}
	files, err = fixture.manager.Files(context.Background(), ScopeUser)
	if err != nil || files.Files[0].Managed || files.SettingsPath != "" {
		t.Fatalf("unmanaged unit claimed: %+v %v", files, err)
	}
}

func newManagerFixture(t *testing.T, euid int) managerFixture {
	t.Helper()
	root := filepath.Join(t.TempDir(), "paths with space;$value%")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	executable := filepath.Join(root, "bin", "sing-box-panel")
	settings := filepath.Join(root, "etc", "setting.json")
	data := filepath.Join(root, "data")
	for _, directory := range []string{filepath.Dir(executable), filepath.Dir(settings), data} {
		if err := os.MkdirAll(directory, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(executable, []byte("test binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(settings, []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	layout := Layout{
		SystemUnitPath:       filepath.Join(root, "system", UnitName),
		SystemSysusersPath:   filepath.Join(root, "sysusers", "sing-box-panel.conf"),
		SystemTmpfilesPath:   filepath.Join(root, "tmpfiles", "sing-box-panel.conf"),
		SystemExecutablePath: executable,
		SystemSettingsPath:   settings,
		SystemDataDir:        data,
		UserUnitPath:         filepath.Join(root, "user", UnitName),
	}
	runner := &recordingRunner{}
	manager, err := New(Options{
		GOOS: "linux", EUID: func() int { return euid },
		Executable: func() (string, error) { return executable, nil },
		Runner:     runner, Layout: layout,
	})
	if err != nil {
		t.Fatal(err)
	}
	return managerFixture{manager: manager, runner: runner, layout: layout, executable: executable, settings: settings, data: data}
}

func escapedUnitPath(value string) string {
	value = strings.ReplaceAll(value, "\\", "\\\\")
	value = strings.ReplaceAll(value, `"`, `\"`)
	value = strings.ReplaceAll(value, "%", "%%")
	return strings.ReplaceAll(value, "$", "$$")
}

func escapedPathDirective(value string) string {
	value = strings.ReplaceAll(value, "\\", "\\\\")
	value = strings.ReplaceAll(value, `"`, `\"`)
	return strings.ReplaceAll(value, "%", "%%")
}
