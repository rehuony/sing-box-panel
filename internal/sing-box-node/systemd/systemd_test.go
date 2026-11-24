// SPDX-License-Identifier: GPL-3.0-or-later

package systemd

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
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

func TestSystemInstallCreatesSettingsStartsTheNodeAndGivesItsAccountTheFiles(t *testing.T) {
	root := t.TempDir()
	executable := filepath.Join(root, "bin", Name)
	settingsPath := filepath.Join(root, "etc", Name, "setting.json")
	dataDir := filepath.Join(root, "data")
	if err := os.MkdirAll(filepath.Dir(executable), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(executable, []byte("test binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	layout := systemd.Layout{
		SystemUnitPath: filepath.Join(root, "system", UnitName), SystemSysusersPath: filepath.Join(root, "sysusers", Name+".conf"),
		SystemTmpfilesPath: filepath.Join(root, "tmpfiles", Name+".conf"), SystemExecutablePath: executable,
		SystemStatePath: settingsPath, SystemDataDir: dataDir, UserUnitPath: filepath.Join(root, "user", UnitName),
	}
	runner := &recordingRunner{unitPath: layout.SystemUnitPath}
	manager, err := New(systemd.Options{
		GOOS: "linux", EUID: func() int { return 0 }, Executable: func() (string, error) { return executable, nil },
		Runner: runner, Layout: layout, LookPath: func(name string) (string, error) { return name, nil }, ProcRoot: t.TempDir(),
	})
	if err != nil {
		t.Fatal(err)
	}
	request := systemd.InstallRequest{Scope: systemd.ScopeSystem, StatePath: settingsPath, Now: true}

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
		{"systemctl", "--no-ask-password", "enable", "--now", UpdatePathUnit},
	}
	// The root update helper watches the node's request and applies it with
	// the installed executable.
	for unit, line := range map[string]string{
		UpdatePathUnit:    "PathExists=" + filepath.Join(dataDir, "updates", "request"),
		UpdateServiceUnit: `ExecStart="` + executable + `" update --apply-staged --data-dir "` + dataDir + `"`,
	} {
		content, err := os.ReadFile(filepath.Join(filepath.Dir(layout.SystemUnitPath), unit))
		if err != nil || !slices.Contains(strings.Split(string(content), "\n"), line) {
			t.Errorf("%s lacks %q: %v\n%s", unit, line, err, content)
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
				if scope != systemd.ScopeSystem {
					return
				}
				paths := []string{}
				for _, companion := range Definition().Companions {
					rendered, err := manager.RenderCompanion(companion, "/usr/bin/true", data)
					if err != nil {
						t.Fatal(err)
					}
					path := filepath.Join(dir, companion.Name)
					if err := os.WriteFile(path, rendered, 0o600); err != nil {
						t.Fatal(err)
					}
					paths = append(paths, path)
				}
				if output, err := exec.CommandContext(t.Context(), analyze, append([]string{"verify", "--man=no"}, paths...)...).CombinedOutput(); err != nil {
					t.Fatalf("systemd rejected the update helper units: %v\n%s", err, output)
				}
			})
		}
	}
}
