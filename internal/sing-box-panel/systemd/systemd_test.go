// SPDX-License-Identifier: GPL-3.0-or-later

package systemd

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"

	panelsettings "github.com/rehuony/sing-box-panel/internal/sing-box-panel/settings"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/testutil"
	"github.com/rehuony/sing-box-panel/internal/systemd"
)

type recordedCommand struct {
	name string
	args []string
}

type recordingRunner struct {
	calls []recordedCommand
	run   func(string, []string) (systemd.CommandResult, error)
}

func (runner *recordingRunner) Run(_ context.Context, name string, args ...string) (systemd.CommandResult, error) {
	runner.calls = append(runner.calls, recordedCommand{name: name, args: append([]string(nil), args...)})
	if runner.run != nil {
		return runner.run(name, args)
	}
	if name == "systemctl" && slices.Contains(args, "--property=LoadState") {
		return systemd.CommandResult{Stdout: []byte("LoadState=not-found\nActiveState=inactive\nSubState=dead\nUnitFileState=\nMainPID=0\nFragmentPath=\n")}, nil
	}
	return systemd.CommandResult{}, nil
}

type managerFixture struct {
	manager    *Manager
	options    systemd.Options
	runner     *recordingRunner
	layout     systemd.Layout
	executable string
	settings   string
	data       string
}

func TestUserInstallAndUninstallUseAuditedArguments(t *testing.T) {
	fixture := newManagerFixture(t, 1000)
	result, err := fixture.manager.Install(context.Background(), systemd.InstallRequest{
		Scope: systemd.ScopeAuto, StatePath: fixture.settings, DataDir: fixture.data, Now: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Scope != systemd.ScopeUser || !result.Enabled || !result.Started || result.UnitPath != fixture.layout.UserUnitPath {
		t.Fatalf("Install() = %+v", result)
	}
	wantCalls := []recordedCommand{
		{name: "systemctl", args: []string{"--no-ask-password", "--user", "show", "--property=Version", "--value"}},
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
		testManagedMark,
		`ExecStart="` + escapedUnitPath(fixture.executable) + `" server start --config "` + escapedUnitPath(fixture.settings) + `"`,
		`Environment=SING_BOX_PANEL_SUPERVISOR=systemd`,
		`WorkingDirectory=` + strings.ReplaceAll(fixture.data, "%", "%%"),
		`ReadWritePaths="` + escapedPathDirective(fixture.data) + `" "` + escapedPathDirective(filepath.Dir(fixture.settings)) + `"`,
	} {
		if !strings.Contains(text, value) {
			t.Fatalf("unit does not contain %q:\n%s", value, text)
		}
	}

	fixture.runner.calls = nil
	uninstalled, err := fixture.manager.Uninstall(context.Background(), systemd.UninstallRequest{Scope: systemd.ScopeUser})
	if err != nil {
		t.Fatal(err)
	}
	if !uninstalled.ConfigRetained || !uninstalled.DataRetained || uninstalled.AccountRetained {
		t.Fatalf("Uninstall() = %+v", uninstalled)
	}
	wantCalls = []recordedCommand{
		{name: "systemctl", args: testStatusArgs(true)},
		{name: "systemctl", args: []string{"--no-ask-password", "--user", "disable", "--no-reload", UnitName}},
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
	request := systemd.InstallRequest{Scope: systemd.ScopeUser, StatePath: fixture.settings, DataDir: fixture.data}
	if _, err := fixture.manager.Install(context.Background(), request); !errors.Is(err, systemd.ErrConflict) {
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

	if _, err := fixture.manager.Uninstall(context.Background(), systemd.UninstallRequest{Scope: systemd.ScopeUser}); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(filepath.Dir(fixture.layout.UserUnitPath), "target")
	if err := os.WriteFile(target, []byte("do not replace"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, fixture.layout.UserUnitPath); err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.manager.Install(context.Background(), request); !errors.Is(err, systemd.ErrConflict) {
		t.Fatalf("forced symlink Install() error = %v, want conflict", err)
	}
	data, err := os.ReadFile(target)
	if err != nil || string(data) != "do not replace" {
		t.Fatalf("symlink target changed: data=%q err=%v", data, err)
	}
}

func TestSystemInstallRequiresRootAndConventionalLayout(t *testing.T) {
	nonRoot := newManagerFixture(t, 1000)
	_, err := nonRoot.manager.Install(context.Background(), systemd.InstallRequest{Scope: systemd.ScopeSystem})
	if !errors.Is(err, systemd.ErrPermission) {
		t.Fatalf("non-root Install() error = %v", err)
	}
	if len(nonRoot.runner.calls) != 0 {
		t.Fatalf("non-root commands = %#v", nonRoot.runner.calls)
	}

	fixture := newManagerFixture(t, 0)
	result, err := fixture.manager.Install(context.Background(), systemd.InstallRequest{
		Scope: systemd.ScopeSystem, StatePath: fixture.settings, DataDir: fixture.data,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Scope != systemd.ScopeSystem || result.Started || len(result.InstalledPaths) != 3 {
		t.Fatalf("Install() = %+v", result)
	}
	wantCalls := []recordedCommand{
		{name: "systemctl", args: []string{"--no-ask-password", "show", "--property=Version", "--value"}},
		{name: "getent", args: []string{"passwd"}},
		{name: "getent", args: []string{"group"}},
		{name: "getent", args: []string{"passwd", Name}},
		{name: "getent", args: []string{"group", Name}},
		{name: "getent", args: []string{"passwd"}},
		{name: "getent", args: []string{"group"}},
		{name: "getent", args: []string{"passwd", Name}},
		{name: "getent", args: []string{"group", Name}},
		{name: "systemd-sysusers", args: []string{fixture.layout.SystemSysusersPath}},
		{name: "systemd-tmpfiles", args: []string{"--create", fixture.layout.SystemTmpfilesPath}},
		{name: "chown", args: []string{Name + ":" + Name, filepath.Dir(fixture.settings)}},
		{name: "chown", args: []string{Name + ":" + Name, fixture.settings}},
		{name: "chown", args: []string{"--recursive", "--no-dereference", Name + ":" + Name, fixture.data}},
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
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("settings mode = %o, want 600", got)
	}

	request := systemd.InstallRequest{Scope: systemd.ScopeSystem, StatePath: fixture.settings, DataDir: filepath.Join(fixture.data, "other")}
	if _, err := fixture.manager.Install(context.Background(), request); !errors.Is(err, systemd.ErrInvalid) {
		t.Fatalf("non-conventional Install() error = %v", err)
	}
}

func TestStatusParsesExactPropertiesAndReportsMissingUnit(t *testing.T) {
	fixture := newManagerFixture(t, 1000)
	fixture.runner.run = func(name string, args []string) (systemd.CommandResult, error) {
		return systemd.CommandResult{Stdout: []byte("LoadState=loaded\nActiveState=active\nSubState=running\nUnitFileState=enabled\nMainPID=42\nFragmentPath=/usr/lib/systemd/system/sing-box-panel.service\n")}, nil
	}
	status, err := fixture.manager.Status(context.Background(), systemd.ScopeAuto)
	if err != nil {
		t.Fatal(err)
	}
	if status.Scope != systemd.ScopeUser || status.LoadState != "loaded" || status.ActiveState != "active" || status.MainPID != 42 || status.UnitPath != "/usr/lib/systemd/system/sing-box-panel.service" {
		t.Fatalf("Status() = %+v", status)
	}
	if status.UnitFileStatePath != "" || status.NeedDaemonReload {
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

	fixture.runner.run = func(string, []string) (systemd.CommandResult, error) {
		return systemd.CommandResult{Stdout: []byte("LoadState=not-found\nActiveState=inactive\nSubState=dead\nUnitFileState=\nMainPID=0\nFragmentPath=\n")}, nil
	}
	if _, err := fixture.manager.Status(context.Background(), systemd.ScopeUser); !errors.Is(err, systemd.ErrNotInstalled) {
		t.Fatalf("missing systemd.Status() error = %v", err)
	}
}

func TestStatusReportsUnitFileStatePathAndDaemonReloadState(t *testing.T) {
	fixture := newManagerFixture(t, 1000)
	if _, err := fixture.manager.Install(context.Background(), systemd.InstallRequest{
		Scope: systemd.ScopeUser, StatePath: fixture.settings, DataDir: fixture.data,
	}); err != nil {
		t.Fatal(err)
	}
	properties := "LoadState=loaded\nActiveState=active\nSubState=running\nUnitFileState=enabled\nMainPID=42\nFragmentPath=" + fixture.layout.UserUnitPath + "\nDropInPaths=%s\nNeedDaemonReload=%r\n"
	respond := func(dropIns, reload string) {
		fixture.runner.run = func(string, []string) (systemd.CommandResult, error) {
			stdout := strings.ReplaceAll(strings.ReplaceAll(properties, "%s", dropIns), "%r", reload)
			return systemd.CommandResult{Stdout: []byte(stdout)}, nil
		}
	}

	respond("", "no")
	status, err := fixture.manager.Status(context.Background(), systemd.ScopeUser)
	if err != nil {
		t.Fatal(err)
	}
	if status.UnitFileStatePath != fixture.settings || status.NeedDaemonReload {
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
	status, err = fixture.manager.Status(context.Background(), systemd.ScopeUser)
	if err != nil {
		t.Fatal(err)
	}
	if status.UnitFileStatePath != editedSettings || !status.NeedDaemonReload {
		t.Fatalf("stale systemd.Status() = %+v", status)
	}

	respond("/etc/systemd/system/sing-box-panel.service.d/override.conf", "no")
	status, err = fixture.manager.Status(context.Background(), systemd.ScopeUser)
	if err != nil {
		t.Fatal(err)
	}
	if status.UnitFileStatePath != "" {
		t.Fatalf("drop-in override must leave the settings path unknown, got %q", status.UnitFileStatePath)
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
	fixture.runner.run = func(name string, args []string) (systemd.CommandResult, error) {
		if name != "journalctl" {
			t.Fatalf("name = %q", name)
		}
		return systemd.CommandResult{Stderr: []byte("journal unavailable\n")}, errors.New("exit status 1")
	}
	since := "2026-08-26 12:00:00; touch /tmp/not-run"
	_, err := fixture.manager.Logs(context.Background(), systemd.LogsRequest{Scope: systemd.ScopeUser, Lines: 20, Since: since})
	var commandErr *systemd.CommandError
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
	fixture.options.GOOS = "darwin"
	fixture.rebuild(t)
	if _, err := fixture.manager.Control(context.Background(), systemd.ScopeUser, systemd.ActionStart); !errors.Is(err, systemd.ErrUnsupportedOS) {
		t.Fatalf("Control() error = %v", err)
	}
	if len(fixture.runner.calls) != 0 {
		t.Fatalf("commands = %#v", fixture.runner.calls)
	}
}

func TestFilesInspectsManagedDestinationsWithoutRunningCommands(t *testing.T) {
	fixture := newManagerFixture(t, 1000)
	if _, err := fixture.manager.Install(context.Background(), systemd.InstallRequest{Scope: systemd.ScopeUser, StatePath: fixture.settings, DataDir: fixture.data}); err != nil {
		t.Fatal(err)
	}
	fixture.runner.calls = nil
	files, err := fixture.manager.Files(context.Background(), systemd.ScopeUser)
	if err != nil {
		t.Fatal(err)
	}
	if files.StatePath != fixture.settings || len(files.Files) != 1 || !files.Files[0].Managed {
		t.Fatalf("files=%+v", files)
	}
	if len(fixture.runner.calls) != 0 {
		t.Fatal("inspection ran a system command")
	}
	if err := os.WriteFile(fixture.layout.UserUnitPath, []byte("unmanaged"), 0o644); err != nil {
		t.Fatal(err)
	}
	files, err = fixture.manager.Files(context.Background(), systemd.ScopeUser)
	if err != nil || files.Files[0].Managed || files.StatePath != "" {
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
	if err := os.WriteFile(settings, validTestSettings(data), 0o600); err != nil {
		t.Fatal(err)
	}
	layout := systemd.Layout{
		SystemUnitPath:       filepath.Join(root, "system", UnitName),
		SystemSysusersPath:   filepath.Join(root, "sysusers", "sing-box-panel.conf"),
		SystemTmpfilesPath:   filepath.Join(root, "tmpfiles", "sing-box-panel.conf"),
		SystemExecutablePath: executable,
		SystemStatePath:      settings,
		SystemDataDir:        data,
		UserUnitPath:         filepath.Join(root, "user", UnitName),
	}
	runner := &recordingRunner{}
	procRoot := t.TempDir()
	task := filepath.Join(procRoot, "1", "task", "1")
	if err := os.MkdirAll(task, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(task, "status"), []byte("Uid:\t0 0 0 0\nGid:\t0 0 0 0\nGroups:\t0\n"), 0600); err != nil {
		t.Fatal(err)
	}
	fixture := managerFixture{
		options: systemd.Options{
			GOOS: "linux", EUID: func() int { return euid },
			Executable: func() (string, error) { return executable, nil },
			Runner:     runner, Layout: layout, LookPath: func(name string) (string, error) { return name, nil },
			ProcRoot: procRoot,
		},
		runner: runner, layout: layout, executable: executable, settings: settings, data: data,
	}
	fixture.rebuild(t)
	return fixture
}

// rebuild creates the fixture's manager from its current options.
func (f *managerFixture) rebuild(t *testing.T) {
	t.Helper()
	manager, err := New(f.options)
	if err != nil {
		t.Fatal(err)
	}
	f.manager = manager
}

// renderUnit renders the panel's unit for scope.
func renderUnit(scope systemd.Scope, executable, settingsPath, dataDir string) ([]byte, error) {
	manager, err := New(systemd.Options{Layout: systemd.Layout{UserUnitPath: "/nonexistent/user/" + UnitName}})
	if err != nil {
		return nil, err
	}
	return manager.Render(scope, executable, settingsPath, dataDir)
}

const testManagedMark = "# Managed by sing-box-panel system install."

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

func validTestSettings(data string) []byte {
	config := panelsettings.Defaults()
	config.DataDir = data
	config.Auth.Email = testutil.AdminEmail
	config.Auth.PasswordHash = testutil.PasswordHash
	raw, _ := json.Marshal(config)
	return raw
}
func testStatusArgs(user bool) []string {
	args := []string{"--no-ask-password"}
	if user {
		args = append(args, "--user")
	}
	return append(args, "show", UnitName, "--no-pager", "--property=LoadState", "--property=ActiveState", "--property=SubState", "--property=UnitFileState", "--property=MainPID", "--property=FragmentPath", "--property=DropInPaths", "--property=NeedDaemonReload")
}

func fileExists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}

// The system unit installs a release the panel staged before it starts the
// panel again on the status the panel exits with for that.
func TestSystemUnitInstallsAStagedUpdateBeforeThePanel(t *testing.T) {
	manager, err := New(systemd.Options{Layout: systemd.Layout{UserUnitPath: "/nonexistent/user/" + UnitName}})
	if err != nil {
		t.Fatal(err)
	}
	unit, err := manager.Render(systemd.ScopeSystem, "/usr/local/bin/sing-box-panel", SystemSettingsPath, "/var/lib/sing-box-panel")
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(string(unit), "\n")
	for _, want := range []string{
		`ExecStartPre=-+"/usr/local/bin/sing-box-panel" update --apply-staged --data-dir "/var/lib/sing-box-panel"`,
		"SuccessExitStatus=" + strconv.Itoa(systemd.RestartExitStatus),
		"RestartForceExitStatus=" + strconv.Itoa(systemd.RestartExitStatus),
	} {
		if !slices.Contains(lines, want) {
			t.Errorf("the system unit lacks %q:\n%s", want, unit)
		}
	}
}
