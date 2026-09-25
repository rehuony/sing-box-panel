// SPDX-License-Identifier: GPL-3.0-or-later

package systemd

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/rehuony/sing-box-panel/internal/settings"
	"github.com/rehuony/sing-box-panel/internal/store"
)

const testPasswdEntry = "sing-box-panel:x:991:992:Sing-Box Panel service account:/var/lib/sing-box-panel:/usr/sbin/nologin\n"
const testGroupEntry = "sing-box-panel:x:992:\n"

type accountFixture struct {
	managerFixture
	passwd, group string
	autoGroup     bool
	failCommand   string
	active        bool
}

func newAccountFixture(t *testing.T) *accountFixture {
	t.Helper()
	f := &accountFixture{managerFixture: newManagerFixture(t, 0)}
	f.runner.run = func(name string, args []string) (CommandResult, error) {
		if name == f.failCommand {
			return CommandResult{}, errors.New("injected command failure")
		}
		switch name {
		case "getent":
			if len(args) == 2 {
				entry := f.group
				if args[0] == "passwd" {
					entry = f.passwd
				}
				return CommandResult{Stdout: []byte(entry)}, nil
			}
			if args[0] == "passwd" {
				return CommandResult{Stdout: []byte("root:x:0:0:root:/root:/bin/sh\n" + f.passwd)}, nil
			}
			return CommandResult{Stdout: []byte("root:x:0:\n" + f.group)}, nil
		case "systemd-sysusers":
			f.passwd, f.group = testPasswdEntry, testGroupEntry
		case "userdel":
			if f.active {
				return CommandResult{}, errors.New("user still has processes")
			}
			f.passwd = ""
			if f.autoGroup {
				f.group = ""
			}
		case "groupdel":
			f.group = ""
		case "systemctl":
			if slices.Contains(args, "--property=LoadState") {
				state, pid := "inactive", "0"
				if f.active {
					state, pid = "active", "123"
				}
				return CommandResult{Stdout: []byte("LoadState=loaded\nActiveState=" + state + "\nSubState=dead\nUnitFileState=disabled\nMainPID=" + pid + "\nFragmentPath=" + f.layout.SystemUnitPath + "\n")}, nil
			}
			if slices.Contains(args, "stop") {
				f.active = false
			}
		}
		return CommandResult{}, nil
	}
	return f
}

func (f *accountFixture) install(t *testing.T) {
	t.Helper()
	if _, err := f.manager.Install(t.Context(), InstallRequest{Scope: ScopeSystem, SettingsPath: f.settings}); err != nil {
		t.Fatal(err)
	}
	f.runner.calls = nil
}

func TestSystemAccountInstallUninstallReinstall(t *testing.T) {
	for _, autoGroup := range []bool{false, true} {
		t.Run(map[bool]string{false: "explicit group deletion", true: "userdel removes group"}[autoGroup], func(t *testing.T) {
			f := newAccountFixture(t)
			f.autoGroup = autoGroup
			f.install(t)
			f.active = true
			before, err := os.ReadFile(f.settings)
			if err != nil {
				t.Fatal(err)
			}
			payload := filepath.Join(f.data, "panel.db")
			if err := os.WriteFile(payload, []byte("retained data"), 0600); err != nil {
				t.Fatal(err)
			}
			result, err := f.manager.Uninstall(t.Context(), UninstallRequest{Scope: ScopeSystem})
			if err != nil || !result.AccountRemoved || !result.GroupRemoved || result.AccountRetained || result.GroupRetained || !result.ConfigRetained || !result.DataRetained {
				t.Fatalf("uninstall: %+v, %v", result, err)
			}
			stop, protect, userdel, groupdel := -1, -1, -1, -1
			for index, call := range f.runner.calls {
				switch {
				case call.name == "systemctl" && slices.Contains(call.args, "stop"):
					stop = index
				case call.name == "chown":
					protect = index
					if !slices.Contains(call.args, "-P") || !slices.Contains(call.args, "--no-dereference") || !slices.Contains(call.args, "--") {
						t.Fatalf("unsafe ownership change: %+v", call)
					}
				case call.name == "userdel":
					userdel = index
					if !slices.Equal(call.args, []string{"--", serviceUser}) {
						t.Fatalf("unsafe user removal: %+v", call)
					}
				case call.name == "groupdel":
					groupdel = index
				}
			}
			if stop < 0 || protect <= stop || userdel <= protect || (!autoGroup && groupdel <= userdel) || (autoGroup && groupdel != -1) {
				t.Fatalf("incorrect lifecycle ordering: %+v", f.runner.calls)
			}
			after, _ := os.ReadFile(f.settings)
			if string(before) != string(after) {
				t.Fatal("retained settings changed")
			}
			if data, err := os.ReadFile(payload); err != nil || string(data) != "retained data" {
				t.Fatalf("lost retained data: %q, %v", data, err)
			}
			f.install(t)
			if f.passwd == "" || f.group == "" {
				t.Fatal("reinstall did not restore account")
			}
		})
	}
}

func TestSystemAccountRejectsUnrelatedAndSharedIdentities(t *testing.T) {
	cases := []struct{ name, passwd, group string }{
		{"root UID", strings.Replace(testPasswdEntry, ":991:", ":0:", 1), testGroupEntry},
		{"login account", strings.Replace(testPasswdEntry, "/usr/sbin/nologin", "/bin/bash", 1), testGroupEntry},
		{"different home", strings.Replace(testPasswdEntry, "/var/lib/sing-box-panel", "/home/operator", 1), testGroupEntry},
		{"different primary group", strings.Replace(testPasswdEntry, ":992:", ":993:", 1), testGroupEntry},
		{"shared UID", testPasswdEntry + "other:x:991:994:Other:/home/other:/bin/sh\n", testGroupEntry},
		{"shared primary GID", testPasswdEntry + "other:x:994:992:Other:/home/other:/bin/sh\n", testGroupEntry},
		{"supplementary members", testPasswdEntry, "sing-box-panel:x:992:other\n"},
		{"GID alias", testPasswdEntry, testGroupEntry + "other:x:992:\n"},
		{"missing group", testPasswdEntry, ""},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			f := newAccountFixture(t)
			f.passwd, f.group = tt.passwd, tt.group
			_, err := f.manager.Install(t.Context(), InstallRequest{Scope: ScopeSystem, SettingsPath: f.settings, Force: true})
			if !errors.Is(err, ErrConflict) {
				t.Fatalf("install error = %v", err)
			}
			if fileExists(f.layout.SystemUnitPath) {
				t.Fatal("wrote service files before validating identity")
			}
			f.passwd, f.group = "", ""
			f.install(t)
			f.passwd, f.group = tt.passwd, tt.group
			_, err = f.manager.Uninstall(t.Context(), UninstallRequest{Scope: ScopeSystem, Force: true})
			if !errors.Is(err, ErrConflict) {
				t.Fatalf("uninstall error = %v", err)
			}
			for _, call := range f.runner.calls {
				if call.name == "chown" || call.name == "userdel" || call.name == "groupdel" {
					t.Fatalf("mutated conflicting identity: %+v", call)
				}
			}
		})
	}
}

func TestSystemAccountRemovalFailuresRemainRetryable(t *testing.T) {
	for _, command := range []string{"getent", "chown", "userdel", "groupdel"} {
		t.Run(command, func(t *testing.T) {
			f := newAccountFixture(t)
			f.install(t)
			f.failCommand = command
			result, err := f.manager.Uninstall(t.Context(), UninstallRequest{Scope: ScopeSystem})
			if err == nil || len(result.RemovedPaths) != 0 || !fileExists(f.layout.SystemSysusersPath) {
				t.Fatalf("lost retry evidence: %+v, %v", result, err)
			}
			if command == "groupdel" && (!result.AccountRemoved || result.AccountRetained || !result.GroupRetained) {
				t.Fatalf("incorrect partial account result: %+v", result)
			}
			if (command == "chown" || command == "userdel") && f.passwd == "" {
				t.Fatal("account removed before retained data was protected")
			}
			f.failCommand = ""
			result, err = f.manager.Uninstall(t.Context(), UninstallRequest{Scope: ScopeSystem})
			if err != nil || result.AccountRetained || result.GroupRetained {
				t.Fatalf("retry: %+v, %v", result, err)
			}
		})
	}
}

func TestAccountRemovalReportsUnverifiedDeletionAsUnknown(t *testing.T) {
	for _, command := range []string{"userdel", "groupdel"} {
		t.Run(command, func(t *testing.T) {
			f := newAccountFixture(t)
			f.install(t)
			base := f.runner.run
			f.runner.run = func(name string, args []string) (CommandResult, error) {
				result, err := base(name, args)
				if name == command {
					f.failCommand = "getent"
				}
				return result, err
			}
			result, err := f.manager.Uninstall(t.Context(), UninstallRequest{Scope: ScopeSystem})
			if err == nil || result.AccountInspected || result.AccountRetained || result.GroupRetained || result.GroupRemoved || result.AccountRemoved != (command == "groupdel") {
				t.Fatalf("unverified deletion reported as confirmed: %+v, %v", result, err)
			}
			if !fileExists(f.layout.SystemSysusersPath) {
				t.Fatal("lost retry evidence")
			}
		})
	}
}

func TestSystemAccountRemovalAbortsBeforeMutation(t *testing.T) {
	for _, scenario := range []string{"missing userdel", "stop fails", "changed UID", "replaced unit"} {
		t.Run(scenario, func(t *testing.T) {
			f := newAccountFixture(t)
			f.install(t)
			f.active = true
			if scenario == "missing userdel" {
				f.manager.lookPath = func(name string) (string, error) {
					if name == "userdel" {
						return "", os.ErrNotExist
					}
					return name, nil
				}
			}
			base := f.runner.run
			f.runner.run = func(name string, args []string) (CommandResult, error) {
				if name == "systemctl" && slices.Contains(args, "stop") {
					switch scenario {
					case "stop fails":
						return CommandResult{}, errors.New("cannot stop")
					case "changed UID":
						f.passwd = strings.Replace(testPasswdEntry, ":991:", ":995:", 1)
					case "replaced unit":
						path := filepath.Join(t.TempDir(), "replacement.service")
						if err := os.WriteFile(path, []byte("other installation"), 0644); err != nil {
							t.Fatal(err)
						}
						if err := os.Rename(path, f.layout.SystemUnitPath); err != nil {
							t.Fatal(err)
						}
					}
				}
				return base(name, args)
			}
			result, err := f.manager.Uninstall(t.Context(), UninstallRequest{Scope: ScopeSystem, Force: true})
			if err == nil || !result.AccountRetained || len(result.RemovedPaths) > 0 {
				t.Fatalf("uninstall: %+v, %v", result, err)
			}
			for _, call := range f.runner.calls {
				if call.name == "chown" || call.name == "userdel" || call.name == "groupdel" {
					t.Fatalf("unexpected account mutation: %+v", call)
				}
			}
		})
	}
}

func TestSystemAccountRejectsInconsistentNSSLookups(t *testing.T) {
	f := newAccountFixture(t)
	base := f.runner.run
	f.runner.run = func(name string, args []string) (CommandResult, error) {
		if name == "getent" && slices.Equal(args, []string{"passwd", serviceUser}) {
			return CommandResult{Stdout: []byte(testPasswdEntry)}, nil
		}
		return base(name, args)
	}
	_, err := f.manager.Install(t.Context(), InstallRequest{Scope: ScopeSystem, SettingsPath: f.settings})
	if !errors.Is(err, ErrConflict) || fileExists(f.layout.SystemUnitPath) {
		t.Fatalf("adopted identity omitted by enumeration: %v", err)
	}
}

func TestSystemAccountRemovalRequiresManagedDeclaration(t *testing.T) {
	for _, scenario := range []string{"keep user", "missing declaration", "custom declaration"} {
		t.Run(scenario, func(t *testing.T) {
			f := newAccountFixture(t)
			f.install(t)
			if scenario == "missing declaration" {
				if err := os.Remove(f.layout.SystemSysusersPath); err != nil {
					t.Fatal(err)
				}
			} else if scenario == "custom declaration" {
				if err := os.WriteFile(f.layout.SystemSysusersPath, []byte(managedMark+"\n# customized\n"), 0644); err != nil {
					t.Fatal(err)
				}
			}
			result, err := f.manager.Uninstall(t.Context(), UninstallRequest{Scope: ScopeSystem, KeepUser: scenario == "keep user", Force: true})
			if (err != nil) != (scenario == "custom declaration") || !result.AccountRetained || !result.GroupRetained {
				t.Fatalf("uninstall: %+v, %v", result, err)
			}
			if err == nil && result.AccountNote == "" {
				t.Fatal("retention was not explained")
			}
			for _, call := range f.runner.calls {
				if call.name == "chown" || call.name == "userdel" || call.name == "groupdel" {
					t.Fatalf("unexpected account mutation: %+v", call)
				}
			}
		})
	}
}

func TestRetainedAccountPathsIncludePendingRelocation(t *testing.T) {
	f := newAccountFixture(t)
	f.install(t)
	target := filepath.Join(filepath.Dir(f.data), "relocated")
	if err := os.Mkdir(target, 0700); err != nil {
		t.Fatal(err)
	}
	if err := settings.WriteDataLocation(f.settings, settings.DataLocation{DataDir: f.data, Move: &settings.DataMove{ID: "test", Target: target}}); err != nil {
		t.Fatal(err)
	}
	paths, err := f.manager.retainedAccountPaths()
	if err != nil || !slices.Contains(paths, target) || !slices.Contains(paths, f.data) || !slices.Contains(paths, filepath.Dir(f.settings)) {
		t.Fatalf("retained paths: %v, %v", paths, err)
	}
}

func TestAccountRemovalRejectsUnsafeRetainedPaths(t *testing.T) {
	for _, scenario := range []string{"shared root", "directory symlink", "invalid settings", "invalid location"} {
		t.Run(scenario, func(t *testing.T) {
			f := newAccountFixture(t)
			f.install(t)
			switch scenario {
			case "shared root":
				if err := os.WriteFile(f.settings, validTestSettings("/etc"), 0600); err != nil {
					t.Fatal(err)
				}
			case "directory symlink":
				if err := os.Remove(f.data); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(t.TempDir(), f.data); err != nil {
					t.Fatal(err)
				}
			case "invalid settings":
				if err := os.WriteFile(f.settings, []byte("invalid"), 0600); err != nil {
					t.Fatal(err)
				}
			case "invalid location":
				if err := os.WriteFile(f.settings+".location", []byte("invalid"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			_, err := f.manager.Uninstall(t.Context(), UninstallRequest{Scope: ScopeSystem})
			if err == nil {
				t.Fatal("uninstalled with unsafe retained paths")
			}
			for _, call := range f.runner.calls {
				if call.name == "chown" || call.name == "userdel" || call.name == "groupdel" {
					t.Fatalf("mutated unsafe retained paths: %+v", call)
				}
			}
		})
	}
}

func TestSystemUnitGrantsOnlyFilesystemReadCapability(t *testing.T) {
	for _, scope := range []Scope{ScopeSystem, ScopeUser} {
		unit, err := renderUnit(scope, "/usr/local/bin/sing-box-panel", "/etc/sing-box-panel/setting.json", "/var/lib/sing-box-panel")
		if err != nil {
			t.Fatal(err)
		}
		capability := ""
		if scope == ScopeSystem {
			capability = "CAP_DAC_READ_SEARCH"
		}
		for _, directive := range []string{"CapabilityBoundingSet=", "AmbientCapabilities="} {
			if !strings.Contains(string(unit), "\n"+directive+capability+"\n") {
				t.Fatalf("%s: unexpected %s", scope, directive)
			}
		}
		for _, protection := range []string{"NoNewPrivileges=true", "ProtectSystem=strict", "ProtectHome=true"} {
			if scope == ScopeSystem && !strings.Contains(string(unit), protection+"\n") {
				t.Fatalf("lost protection: %s", protection)
			}
		}
	}
}

func TestAccountRemovalChecksProcessesBeforeOwnership(t *testing.T) {
	for _, scenario := range []string{"UID", "saved UID", "filesystem UID", "GID", "supplementary GID", "other thread", "missing credentials", "unreadable status", "missing procfs", "empty procfs", "busy data", "busy settings"} {
		t.Run(scenario, func(t *testing.T) {
			f := newAccountFixture(t)
			f.install(t)
			status := "Uid:\t0 0 0 0\nGid:\t0 0 0 0\nGroups:\t0\n"
			thread := "100"
			switch scenario {
			case "UID", "other thread":
				status = strings.Replace(status, "Uid:\t0 0 0 0", "Uid:\t991 991 991 991", 1)
				if scenario == "other thread" {
					thread = "101"
				}
			case "saved UID":
				status = strings.Replace(status, "Uid:\t0 0 0 0", "Uid:\t0 0 991 0", 1)
			case "filesystem UID":
				status = strings.Replace(status, "Uid:\t0 0 0 0", "Uid:\t0 0 0 991", 1)
			case "GID":
				status = strings.Replace(status, "Gid:\t0 0 0 0", "Gid:\t0 992 0 0", 1)
			case "supplementary GID":
				status = strings.Replace(status, "Groups:\t0", "Groups:\t0 992", 1)
			case "missing credentials":
				status = "Name:\tsleep\n"
			case "busy data":
				lock, err := store.LockDirectoryForCleanup(f.data)
				if err != nil {
					t.Fatal(err)
				}
				defer lock.Close()
			case "busy settings":
				lock, err := settings.TryLock(t.Context(), f.settings)
				if err != nil {
					t.Fatal(err)
				}
				defer lock.Close()
			}
			path := filepath.Join(f.manager.procRoot, "100", "task", thread, "status")
			if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
				t.Fatal(err)
			}
			if scenario == "unreadable status" {
				if err := os.Mkdir(path, 0700); err != nil {
					t.Fatal(err)
				}
			} else if err := os.WriteFile(path, []byte(status), 0600); err != nil {
				t.Fatal(err)
			}
			if scenario == "missing procfs" {
				f.manager.procRoot = filepath.Join(t.TempDir(), "missing")
			}
			if scenario == "empty procfs" {
				f.manager.procRoot = t.TempDir()
			}
			result, err := f.manager.Uninstall(t.Context(), UninstallRequest{Scope: ScopeSystem})
			if err == nil || !result.AccountRetained || !result.GroupRetained {
				t.Fatalf("uninstall: %+v, %v", result, err)
			}
			for _, call := range f.runner.calls {
				if call.name == "chown" || call.name == "userdel" || call.name == "groupdel" {
					t.Fatalf("mutation before checking account usage: %+v", call)
				}
			}
		})
	}
}

func TestAccountRetentionReportsActualPresence(t *testing.T) {
	for _, keepUser := range []bool{false, true} {
		for _, scenario := range []string{"absent", "group only", "custom account", "lookup error"} {
			t.Run(fmt.Sprintf("keep=%v/%s", keepUser, scenario), func(t *testing.T) {
				f := newAccountFixture(t)
				if scenario == "group only" {
					f.group = testGroupEntry
				}
				if scenario == "custom account" {
					f.passwd = strings.Replace(testPasswdEntry, "/usr/sbin/nologin", "/bin/bash", 1)
					f.group = testGroupEntry
				}
				if scenario == "lookup error" {
					f.failCommand = "getent"
				}
				result, err := f.manager.Uninstall(t.Context(), UninstallRequest{Scope: ScopeSystem, KeepUser: keepUser})
				if (err != nil) != (scenario == "lookup error") || result.AccountInspected != (scenario != "lookup error") || result.AccountRetained != (scenario == "custom account") || result.GroupRetained != (scenario == "group only" || scenario == "custom account") {
					t.Fatalf("incorrect account presence: %+v, %v", result, err)
				}
				if scenario == "absent" && result.AccountNote != "" {
					t.Fatalf("absent account reported retained: %+v", result)
				}
				for _, call := range f.runner.calls {
					if call.name == "chown" || call.name == "userdel" || call.name == "groupdel" {
						t.Fatalf("changed retained identity: %+v", call)
					}
				}
			})
		}
	}
}

func TestAccountRemovalRechecksEffectiveConfigurationAfterStop(t *testing.T) {
	f := newAccountFixture(t)
	f.install(t)
	f.active = true
	base := f.runner.run
	changed := false
	f.runner.run = func(name string, args []string) (CommandResult, error) {
		if name == "systemctl" && slices.Contains(args, "stop") {
			changed = true
		}
		result, err := base(name, args)
		if changed && name == "systemctl" && slices.Contains(args, "--property=LoadState") {
			result.Stdout = append(result.Stdout, []byte("NeedDaemonReload=yes\n")...)
		}
		return result, err
	}
	_, err := f.manager.Uninstall(t.Context(), UninstallRequest{Scope: ScopeSystem})
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("did not reject changed configuration: %v", err)
	}
	for _, call := range f.runner.calls {
		if call.name == "chown" || call.name == "userdel" || call.name == "groupdel" {
			t.Fatalf("mutated changed installation: %+v", call)
		}
	}
}

func TestAccountRemovalRejectsUnknownEffectiveSettings(t *testing.T) {
	for _, scenario := range []string{"drop-in", "unresolved ExecStart", "stale unit"} {
		t.Run(scenario, func(t *testing.T) {
			f := newAccountFixture(t)
			f.install(t)
			external := filepath.Join(t.TempDir(), "actual-data")
			if err := os.Mkdir(external, 0700); err != nil {
				t.Fatal(err)
			}
			externalSettings := filepath.Join(filepath.Dir(external), "actual-setting.json")
			if err := os.WriteFile(externalSettings, validTestSettings(external), 0600); err != nil {
				t.Fatal(err)
			}
			base := f.runner.run
			if scenario == "drop-in" {
				dir := f.layout.SystemUnitPath + ".d"
				if err := os.Mkdir(dir, 0755); err != nil {
					t.Fatal(err)
				}
				dropIn := filepath.Join(dir, "override.conf")
				if err := os.WriteFile(dropIn, []byte("[Service]\nExecStart=\nExecStart=/usr/local/bin/sing-box-panel server start --config \""+externalSettings+"\"\nWorkingDirectory="+external+"\n"), 0644); err != nil {
					t.Fatal(err)
				}
				f.runner.run = func(name string, args []string) (CommandResult, error) {
					result, err := base(name, args)
					if name == "systemctl" && slices.Contains(args, "--property=LoadState") {
						result.Stdout = append(result.Stdout, []byte("DropInPaths="+dropIn+"\n")...)
					}
					return result, err
				}
			} else if scenario == "unresolved ExecStart" {
				if err := os.WriteFile(f.layout.SystemUnitPath, []byte(managedMark+"\n[Service]\nEnvironment=CFG="+externalSettings+"\nExecStart=/usr/local/bin/sing-box-panel server start --config ${CFG}\nWorkingDirectory="+external+"\n"), 0644); err != nil {
					t.Fatal(err)
				}
			} else {
				f.runner.run = func(name string, args []string) (CommandResult, error) {
					result, err := base(name, args)
					if name == "systemctl" && slices.Contains(args, "--property=LoadState") {
						result.Stdout = append(result.Stdout, []byte("NeedDaemonReload=yes\n")...)
					}
					return result, err
				}
			}
			result, err := f.manager.Uninstall(t.Context(), UninstallRequest{Scope: ScopeSystem})
			protectedExternal := false
			for _, call := range f.runner.calls {
				if call.name == "chown" || call.name == "userdel" || call.name == "groupdel" || (call.name == "systemctl" && (slices.Contains(call.args, "stop") || slices.Contains(call.args, "disable"))) {
					t.Fatalf("mutated ambiguous installation: %+v", call)
				}
			}
			for _, call := range f.runner.calls {
				if call.name == "chown" && slices.Contains(call.args, external) {
					protectedExternal = true
				}
			}
			if err == nil || result.AccountRemoved {
				t.Fatalf("unsafe success: account removed despite %s; effective external data protected=%v; result=%+v", scenario, protectedExternal, result)
			}
		})
	}
}

func TestRepeatedUninstallReportsAbsentAccounts(t *testing.T) {
	f := newAccountFixture(t)
	f.install(t)
	if _, err := f.manager.Uninstall(t.Context(), UninstallRequest{Scope: ScopeSystem}); err != nil {
		t.Fatal(err)
	}
	base := f.runner.run
	f.runner.run = func(name string, args []string) (CommandResult, error) {
		if name == "systemctl" && slices.Contains(args, "--property=LoadState") {
			return CommandResult{Stdout: []byte("LoadState=not-found\nActiveState=inactive\nSubState=dead\nUnitFileState=\nMainPID=0\nFragmentPath=\n")}, nil
		}
		return base(name, args)
	}
	result, err := f.manager.Uninstall(t.Context(), UninstallRequest{Scope: ScopeSystem})
	if err != nil {
		t.Fatal(err)
	}
	if result.AccountRetained || result.GroupRetained {
		t.Fatalf("both accounts are absent but reported retained: %+v", result)
	}
}
