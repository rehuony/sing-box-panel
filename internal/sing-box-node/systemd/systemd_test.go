// SPDX-License-Identifier: GPL-3.0-or-later

package systemd

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/rehuony/sing-box-panel/internal/sing-box-node/settings"
	"github.com/rehuony/sing-box-panel/internal/systemd"
)

func TestUnitsGiveOnlyTheSystemServiceReadAccessAndHostCounters(t *testing.T) {
	manager, err := New(systemd.Options{Layout: systemd.Layout{UserUnitPath: "/nonexistent/" + UnitName}})
	if err != nil {
		t.Fatal(err)
	}
	system, err := manager.Render(systemd.ScopeSystem, "/usr/local/bin/sing-box-node", SystemSettingsPath, "/var/lib/sing-box-node")
	if err != nil {
		t.Fatal(err)
	}
	for _, directive := range []string{
		`ExecStart="/usr/local/bin/sing-box-node" server start --config "/etc/sing-box-node/setting.json"`,
		`ReadWritePaths="/var/lib/sing-box-node" "/etc/sing-box-node"`,
		"Environment=SING_BOX_NODE_SUPERVISOR=systemd",
		"User=sing-box-node",
		"CapabilityBoundingSet=CAP_DAC_READ_SEARCH",
		"AmbientCapabilities=CAP_DAC_READ_SEARCH",
		"NoNewPrivileges=true",
		"ProtectSystem=strict",
		"ProtectProc=invisible",
		"Environment=SING_BOX_NODE_HOST_PROC=/run/sing-box-node/host-proc",
		"BindReadOnlyPaths=/proc/stat:/run/sing-box-node/host-proc/stat /proc/meminfo:/run/sing-box-node/host-proc/meminfo /proc/loadavg:/run/sing-box-node/host-proc/loadavg",
	} {
		if !strings.Contains(string(system), "\n"+directive+"\n") {
			t.Errorf("system unit lacks %q", directive)
		}
	}
	user, err := manager.Render(systemd.ScopeUser, "/home/operator/.local/bin/sing-box-node", "/home/operator/.config/sing-box-node/setting.json", "/home/operator/.local/share/sing-box-node")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(user), "\nAmbientCapabilities=\n") || !strings.Contains(string(user), "\nNoNewPrivileges=true\n") {
		t.Fatalf("user unit can gain privileges:\n%s", user)
	}
	// A user manager applies these only in a user namespace, in which the
	// artifact store sees no root-owned directory and refuses to run cores.
	for _, directive := range []string{
		"CapabilityBoundingSet=", "PrivateTmp=", "PrivateDevices=", "ProtectSystem=", "ProtectHome=", "ReadWritePaths=",
		"ProtectHostname=", "ProtectClock=", "ProtectKernelTunables=", "ProtectKernelModules=", "ProtectKernelLogs=", "ProtectControlGroups=",
	} {
		if strings.Contains(string(user), "\n"+directive) {
			t.Errorf("user unit sets %s, which needs a user namespace", directive)
		}
	}
}

type recordingRunner struct {
	calls         [][]string
	passwd, group string
	unitPath      string
}

func (runner *recordingRunner) Run(_ context.Context, name string, args ...string) (systemd.CommandResult, error) {
	runner.calls = append(runner.calls, append([]string{name}, args...))
	switch name {
	case "getent":
		entry := runner.group
		if args[0] == "passwd" {
			entry = runner.passwd
		}
		if len(args) == 1 {
			entry = map[string]string{"passwd": "root:x:0:0:root:/root:/bin/sh\n", "group": "root:x:0:\n"}[args[0]] + entry
		}
		return systemd.CommandResult{Stdout: []byte(entry)}, nil
	case "systemd-sysusers":
		runner.passwd = "sing-box-node:x:991:992:Sing-Box Node service account:/var/lib/sing-box-node:/usr/sbin/nologin\n"
		runner.group = "sing-box-node:x:992:\n"
	case "systemctl":
		if slices.Contains(args, "--property=LoadState") {
			return systemd.CommandResult{Stdout: []byte("LoadState=loaded\nActiveState=active\nSubState=running\nUnitFileState=enabled\nMainPID=12\nFragmentPath=" + runner.unitPath + "\nDropInPaths=\nNeedDaemonReload=no\n")}, nil
		}
	}
	return systemd.CommandResult{}, nil
}

// systemFixture is a system-scope node installation below a temporary root.
type systemFixture struct {
	manager                           *Manager
	runner                            *recordingRunner
	layout                            systemd.Layout
	executable, settingsPath, dataDir string
}

func newSystemFixture(t *testing.T) systemFixture {
	t.Helper()
	root := t.TempDir()
	fixture := systemFixture{
		executable:   filepath.Join(root, "bin", Name),
		settingsPath: filepath.Join(root, "etc", Name, "setting.json"),
		dataDir:      filepath.Join(root, "data"),
	}
	if err := os.MkdirAll(filepath.Dir(fixture.executable), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fixture.executable, []byte("test binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	fixture.layout = systemd.Layout{
		SystemUnitPath: filepath.Join(root, "system", UnitName), SystemSysusersPath: filepath.Join(root, "sysusers", Name+".conf"),
		SystemTmpfilesPath: filepath.Join(root, "tmpfiles", Name+".conf"), SystemExecutablePath: fixture.executable,
		SystemStatePath: fixture.settingsPath, SystemDataDir: fixture.dataDir, UserUnitPath: filepath.Join(root, "user", UnitName),
	}
	fixture.runner = &recordingRunner{unitPath: fixture.layout.SystemUnitPath}
	manager, err := New(systemd.Options{
		GOOS: "linux", EUID: func() int { return 0 }, Executable: func() (string, error) { return fixture.executable, nil },
		Runner: fixture.runner, Layout: fixture.layout, LookPath: func(name string) (string, error) { return name, nil }, ProcRoot: t.TempDir(),
	})
	if err != nil {
		t.Fatal(err)
	}
	fixture.manager = manager
	return fixture
}

func TestSystemInstallCreatesSettingsStartsTheNodeAndGivesItsAccountTheFiles(t *testing.T) {
	fixture := newSystemFixture(t)
	manager, runner, layout := fixture.manager, fixture.runner, fixture.layout
	executable, settingsPath, dataDir := fixture.executable, fixture.settingsPath, fixture.dataDir
	request := systemd.InstallRequest{Scope: systemd.ScopeSystem, StatePath: settingsPath, Now: true}
	// The update helper units an earlier release installed go with this one.
	unitDir := filepath.Dir(layout.SystemUnitPath)
	if err := os.MkdirAll(unitDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, unit := range retiredUnits {
		if err := os.WriteFile(filepath.Join(unitDir, unit), []byte("# Managed by sing-box-node system install.\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	// The console is always on, so a new node starts at once and waits to be
	// enrolled; installing creates its settings with a console password.
	result, err := manager.Install(t.Context(), request)
	if err != nil || !result.Started || result.DataDir != dataDir || result.Initial == nil || result.Initial.InitialPassword == "" {
		t.Fatalf("install = %+v, %v", result, err)
	}
	if configured, err := settings.LoadDataDir(settingsPath); err != nil || configured != dataDir {
		t.Fatalf("settings data_dir = %q, %v", configured, err)
	}
	want := [][]string{
		{"chown", "--recursive", "--no-dereference", "sing-box-node:sing-box-node", dataDir},
		{"chown", "sing-box-node:sing-box-node", settingsPath},
		{"systemctl", "--no-ask-password", "enable", "--now", UnitName},
		{"systemctl", "--no-ask-password", "disable", "--now", Name + "-update.path"},
		{"systemctl", "--no-ask-password", "stop", Name + "-update.service"},
	}
	// The unit installs a staged release with the installed executable before
	// it starts the node again on the restart status.
	content, err := os.ReadFile(layout.SystemUnitPath)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(string(content), "\n")
	for _, line := range []string{
		`ExecStartPre=-+"` + executable + `" update --apply-staged --data-dir "` + dataDir + `"`,
		"SuccessExitStatus=" + strconv.Itoa(systemd.RestartExitStatus),
		"RestartForceExitStatus=" + strconv.Itoa(systemd.RestartExitStatus),
	} {
		if !slices.Contains(lines, line) {
			t.Errorf("the unit lacks %q:\n%s", line, content)
		}
	}
	for _, unit := range retiredUnits {
		if _, err := os.Lstat(filepath.Join(unitDir, unit)); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("the retired %s remains: %v", unit, err)
		}
	}
	for _, call := range want {
		if !slices.ContainsFunc(runner.calls, func(got []string) bool { return slices.Equal(got, call) }) {
			t.Errorf("install did not run %v: %v", call, runner.calls)
		}
	}
	status, err := manager.Status(t.Context(), systemd.ScopeSystem)
	if err != nil || status.UnitFileStatePath != settingsPath {
		t.Fatalf("status = %+v, %v", status, err)
	}
}

// A unit an earlier release installed starts the node with arguments this
// release no longer accepts; starting it is refused with the command that
// rewrites it, and installing replaces it without --force.
func TestUnitOfAnEarlierReleaseIsRefusedUntilInstallationRewritesIt(t *testing.T) {
	fixture := newSystemFixture(t)
	earlier := "# Managed by sing-box-node system install.\n[Service]\nExecStart=\"" + fixture.executable + "\" --data-dir \"" + fixture.dataDir + "\" run\n"
	if err := os.MkdirAll(filepath.Dir(fixture.layout.SystemUnitPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fixture.layout.SystemUnitPath, []byte(earlier), 0o644); err != nil {
		t.Fatal(err)
	}
	if status, err := fixture.manager.Status(t.Context(), systemd.ScopeSystem); err != nil || !status.UnitOutdated {
		t.Fatalf("status = %+v, %v", status, err)
	}
	_, err := fixture.manager.Control(t.Context(), systemd.ScopeSystem, systemd.ActionStart)
	if !errors.Is(err, systemd.ErrConflict) || !strings.Contains(err.Error(), "Next: sudo sing-box-node systemd install --scope=system") {
		t.Fatalf("start of an outdated unit = %v", err)
	}
	if slices.ContainsFunc(fixture.runner.calls, func(call []string) bool { return slices.Contains(call, "start") }) {
		t.Fatalf("an outdated unit was started: %v", fixture.runner.calls)
	}
	if _, err := fixture.manager.Install(t.Context(), systemd.InstallRequest{Scope: systemd.ScopeSystem, StatePath: fixture.settingsPath}); err != nil {
		t.Fatalf("install over the earlier unit: %v", err)
	}
	if status, err := fixture.manager.Status(t.Context(), systemd.ScopeSystem); err != nil || status.UnitOutdated || status.UnitFileStatePath != fixture.settingsPath {
		t.Fatalf("status after install = %+v, %v", status, err)
	}
	// A unit without the install step never installs a staged update.
	installed, err := os.ReadFile(fixture.layout.SystemUnitPath)
	if err != nil {
		t.Fatal(err)
	}
	var kept []string
	for _, line := range strings.Split(string(installed), "\n") {
		if !strings.HasPrefix(line, "ExecStartPre=") {
			kept = append(kept, line)
		}
	}
	if err := os.WriteFile(fixture.layout.SystemUnitPath, []byte(strings.Join(kept, "\n")), 0o644); err != nil {
		t.Fatal(err)
	}
	if status, err := fixture.manager.Status(t.Context(), systemd.ScopeSystem); err != nil || !status.UnitOutdated {
		t.Fatalf("status without the install step = %+v, %v", status, err)
	}
}

func TestGeneratedUnitsWithSystemdAnalyze(t *testing.T) {
	analyze, err := exec.LookPath("systemd-analyze")
	if runtime.GOOS != "linux" || err != nil {
		t.Skip("requires the Linux systemd parser, a CI and test dependency only")
	}
	manager, err := New(systemd.Options{})
	if err != nil {
		t.Fatal(err)
	}
	for _, scope := range []systemd.Scope{systemd.ScopeSystem, systemd.ScopeUser} {
		for _, suffix := range []string{"plain", `space and $cash%literal`, `quote"and\backslash`} {
			t.Run(string(scope)+"/"+suffix, func(t *testing.T) {
				dir := t.TempDir()
				data := filepath.Join(dir, suffix)
				unit, err := manager.Render(scope, "/usr/bin/true", filepath.Join(data, "setting.json"), data)
				if err != nil {
					t.Fatal(err)
				}
				path := filepath.Join(dir, UnitName)
				if err := os.WriteFile(path, unit, 0o600); err != nil {
					t.Fatal(err)
				}
				if output, err := exec.CommandContext(t.Context(), analyze, "verify", "--man=no", path).CombinedOutput(); err != nil {
					t.Fatalf("systemd rejected generated unit: %v\n%s\n%s", err, output, unit)
				}
			})
		}
	}
}
