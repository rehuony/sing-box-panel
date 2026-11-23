// SPDX-License-Identifier: GPL-3.0-or-later

package systemd

import (
	"context"
	"errors"
	"github.com/rehuony/sing-box-panel/internal/systemd"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// Explicit opt-in in a disposable container only. This test uses the real
// account tools and filesystem; systemctl alone is replaced because the test
// container is not booted with a systemd manager.
func TestSystemAccountLifecycleInContainer(t *testing.T) {
	if os.Getenv("SING_BOX_PANEL_TEST_SYSTEM_ACCOUNTS") != "1" || os.Geteuid() != 0 {
		t.Skip("requires an explicitly opted-in, disposable root container")
	}
	if _, err := os.Stat("/.dockerenv"); err != nil {
		t.Fatal("refusing to modify accounts outside a Docker container")
	}
	f := newManagerFixture(t, 0)
	f.options.Runner = containerAccountRunner{unitPath: f.layout.SystemUnitPath}
	f.options.LookPath = exec.LookPath
	f.options.ProcRoot = "/proc"
	f.rebuild(t)
	initial, err := f.manager.Account(t.Context())
	if err != nil || initial != (systemd.Account{}) {
		t.Fatalf("container must not contain an existing service account: %+v, %v", initial, err)
	}
	payload := filepath.Join(f.data, "panel.db")
	if err := os.WriteFile(payload, []byte("retained database fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	certDir := t.TempDir()
	if err := os.Chmod(certDir, 0700); err != nil {
		t.Fatal(err)
	}
	cert := filepath.Join(certDir, "privkey.pem")
	if err := os.WriteFile(cert, []byte("root-owned certificate fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(cert, filepath.Join(f.data, "certificate-link")); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if _, err := f.manager.Install(t.Context(), systemd.InstallRequest{Scope: systemd.ScopeSystem, StatePath: f.settings}); err != nil {
			t.Fatal(err)
		}
		account, err := f.manager.Account(t.Context())
		if err != nil || account.UID == "" || account.GID == "" {
			t.Fatalf("account not created: %+v, %v", account, err)
		}
		uid, _ := strconv.ParseUint(account.UID, 10, 32)
		gid, _ := strconv.ParseUint(account.GID, 10, 32)
		for _, path := range []string{f.settings, f.data, payload} {
			assertFileOwner(t, path, uint32(uid), uint32(gid))
		}
		plain := []string{"--reuid=" + account.UID, "--regid=" + account.GID, "--clear-groups"}
		if err := exec.CommandContext(t.Context(), "setpriv", append(plain, "cat", cert)...).Run(); err == nil {
			t.Fatal("unprivileged service user unexpectedly read the private certificate")
		}
		privileged := append(slices.Clone(plain), "--bounding-set=-all,+dac_read_search", "--inh-caps=+dac_read_search", "--ambient-caps=+dac_read_search", "--no-new-privs")
		output, err := exec.CommandContext(t.Context(), "setpriv", append(privileged, "cat", cert)...).CombinedOutput()
		if err != nil || string(output) != "root-owned certificate fixture" {
			t.Fatalf("read capability did not grant certificate access: %s, %v", output, err)
		}
		writeArgs := append(slices.Clone(privileged), "sh", "-c", `printf changed > "$1"`, "sh", cert)
		if err := exec.CommandContext(t.Context(), "setpriv", writeArgs...).Run(); err == nil {
			t.Fatal("read capability unexpectedly permitted a normal file write")
		}
		result, err := f.manager.Uninstall(t.Context(), systemd.UninstallRequest{Scope: systemd.ScopeSystem})
		if err != nil || !result.AccountRemoved || !result.GroupRemoved || result.AccountRetained || result.GroupRetained {
			t.Fatalf("uninstall: %+v, %v", result, err)
		}
		for _, path := range []string{f.settings, f.data, payload} {
			assertFileOwner(t, path, 0, 0)
		}
		if data, err := os.ReadFile(payload); err != nil || string(data) != "retained database fixture" {
			t.Fatalf("retained data changed: %s, %v", data, err)
		}
		assertFileOwner(t, cert, 0, 0)
		for path, mode := range map[string]os.FileMode{cert: 0600, certDir: 0700} {
			info, err := os.Stat(path)
			if err != nil || info.Mode().Perm() != mode {
				t.Fatalf("certificate permissions changed: %s, %v", path, err)
			}
		}
	}
}

func assertFileOwner(t *testing.T, path string, uid, gid uint32) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	stat := info.Sys().(*syscall.Stat_t)
	if stat.Uid != uid || stat.Gid != gid {
		t.Fatalf("%s: owner=%d:%d, want %d:%d", path, stat.Uid, stat.Gid, uid, gid)
	}
}

type containerAccountRunner struct{ unitPath string }

func (runner containerAccountRunner) Run(ctx context.Context, name string, args ...string) (systemd.CommandResult, error) {
	if name != "systemctl" {
		return (systemd.ExecRunner{}).Run(ctx, name, args...)
	}
	if slices.Contains(args, "--property=LoadState") {
		return systemd.CommandResult{Stdout: []byte("LoadState=loaded\nActiveState=inactive\nSubState=dead\nUnitFileState=disabled\nMainPID=0\nFragmentPath=" + runner.unitPath + "\n")}, nil
	}
	return systemd.CommandResult{}, nil
}

func TestBusySystemAccountInContainer(t *testing.T) {
	if os.Getenv("SING_BOX_PANEL_TEST_SYSTEM_ACCOUNTS") != "1" || os.Geteuid() != 0 {
		t.Skip("disposable container only")
	}
	if _, err := os.Stat("/.dockerenv"); err != nil {
		t.Fatal(err)
	}
	f := newManagerFixture(t, 0)
	f.options.Runner = containerAccountRunner{unitPath: f.layout.SystemUnitPath}
	f.options.LookPath = exec.LookPath
	f.options.ProcRoot = "/proc"
	f.rebuild(t)
	initial, err := f.manager.Account(t.Context())
	if err != nil || initial != (systemd.Account{}) {
		t.Fatalf("unsafe container account state: %+v %v", initial, err)
	}
	if _, err := f.manager.Install(t.Context(), systemd.InstallRequest{Scope: systemd.ScopeSystem, StatePath: f.settings}); err != nil {
		t.Fatal(err)
	}
	account, err := f.manager.Account(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	for _, directory := range []string{filepath.Dir(f.data), filepath.Dir(filepath.Dir(f.data)), filepath.Dir(filepath.Dir(filepath.Dir(f.data)))} {
		if err := os.Chmod(directory, 0755); err != nil {
			t.Fatal(err)
		}
	}
	child := exec.Command("setpriv", "--reuid="+account.UID, "--regid="+account.GID, "--clear-groups", "sh", "-c", `touch "$1/ready"; exec sleep 60`, "sh", f.data)
	if err := child.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = child.Process.Kill(); _ = child.Wait() }()
	ready := filepath.Join(f.data, "ready")
	for i := 0; i < 100; i++ {
		if _, err := os.Stat(ready); err == nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if _, err := os.Stat(ready); err != nil {
		t.Fatalf("child did not become ready: %v", err)
	}
	result, err := f.manager.Uninstall(t.Context(), systemd.UninstallRequest{Scope: systemd.ScopeSystem})
	if !errors.Is(err, systemd.ErrConflict) || !strings.Contains(err.Error(), "still used by process") {
		t.Fatalf("expected independent process check to refuse busy account: %v", err)
	}
	current, readErr := f.manager.Account(t.Context())
	if readErr != nil || current.UID != account.UID {
		t.Fatalf("busy user no longer present: %+v %v", current, readErr)
	}
	info, statErr := os.Stat(f.data)
	if statErr != nil {
		t.Fatal(statErr)
	}
	uid, _ := strconv.ParseUint(account.UID, 10, 32)
	stat := info.Sys().(*syscall.Stat_t)
	if stat.Uid != uint32(uid) {
		t.Fatalf("live user %s retained but its 0700 data directory was transferred to %d:%d; uninstall=%+v; error=%v", account.UID, stat.Uid, stat.Gid, result, err)
	}
	_ = child.Process.Kill()
	_ = child.Wait()
	result, err = f.manager.Uninstall(t.Context(), systemd.UninstallRequest{Scope: systemd.ScopeSystem})
	if err != nil || !result.AccountRemoved || !result.GroupRemoved {
		t.Fatalf("retry after stopping process: %+v, %v", result, err)
	}
}
