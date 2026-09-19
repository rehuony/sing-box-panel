// SPDX-License-Identifier: GPL-3.0-or-later

package panelprocess

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func shortDirectory(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("/tmp", "sbp-control-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return dir
}

func TestStopWaitsForCleanupAndReportsItsResult(t *testing.T) {
	for _, cleanupErr := range []error{nil, errors.New("runtime cleanup failed")} {
		t.Run(fmtError(cleanupErr), func(t *testing.T) {
			t.Setenv("INVOCATION_ID", "")
			dir := shortDirectory(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			control, err := Listen(Status{DataDir: dir}, cancel)
			if err != nil {
				t.Fatal(err)
			}
			control.Ready("127.0.0.1:3000")
			info, err := os.Stat(filepath.Join(dir, socketName))
			if err != nil || info.Mode().Perm() != 0o600 {
				t.Fatalf("socket permissions: %v %v", info, err)
			}
			result := make(chan error, 1)
			go func() {
				wait, stop := context.WithTimeout(context.Background(), 3*time.Second)
				defer stop()
				status, err := Stop(wait, dir)
				if err == nil && status.State != "stopped" {
					err = errors.New("stop returned before stopped")
				}
				result <- err
			}()
			select {
			case <-ctx.Done():
			case <-time.After(2 * time.Second):
				t.Fatal("stop did not cancel panel")
			}
			status, err := Inspect(context.Background(), dir)
			if err != nil || status.State != "stopping" {
				t.Fatalf("status=%+v err=%v", status, err)
			}
			select {
			case err := <-result:
				t.Fatalf("stop returned before cleanup: %v", err)
			default:
			}
			control.StopAccepting()
			control.Finish(cleanupErr)
			err = <-result
			if fmtError(err) != fmtError(cleanupErr) {
				t.Fatalf("stop error=%v want=%v", err, cleanupErr)
			}
			status, err = Inspect(context.Background(), dir)
			if err != nil || status.State != "stopped" {
				t.Fatalf("after shutdown=%+v err=%v", status, err)
			}
		})
	}
}

func fmtError(err error) string {
	if err == nil {
		return "success"
	}
	return err.Error()
}

func TestControlRejectsSystemdStop(t *testing.T) {
	t.Setenv("SING_BOX_PANEL_SUPERVISOR", "systemd")
	dir := shortDirectory(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	control, err := Listen(Status{DataDir: dir}, cancel)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { control.StopAccepting(); control.Finish(nil) }()
	control.Ready("127.0.0.1:3000")
	status, err := Stop(context.Background(), dir)
	if err == nil || !strings.Contains(err.Error(), "systemd stop") || status.ManagedBy != "systemd" {
		t.Fatalf("status=%+v err=%v", status, err)
	}
	if ctx.Err() != nil {
		t.Fatal("manual stop canceled systemd process")
	}
}

func TestInheritedTerminalInvocationDoesNotImplySystemdOwnership(t *testing.T) {
	t.Setenv("INVOCATION_ID", "terminal-service-invocation")
	t.Setenv("SING_BOX_PANEL_SUPERVISOR", "")
	dir := shortDirectory(t)
	control, err := Listen(Status{DataDir: dir}, func() {})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { control.StopAccepting(); control.Finish(nil) }()
	status, err := Inspect(context.Background(), dir)
	if err != nil || status.ManagedBy != "terminal" {
		t.Fatalf("status=%+v err=%v", status, err)
	}
}

func TestControlReplacesOnlyStaleSockets(t *testing.T) {
	dir := shortDirectory(t)
	path := filepath.Join(dir, socketName)
	listener, err := net.ListenUnix("unix", &net.UnixAddr{Name: path, Net: "unix"})
	if err != nil {
		t.Fatal(err)
	}
	listener.SetUnlinkOnClose(false)
	_ = listener.Close()
	control, err := Listen(Status{DataDir: dir}, func() {})
	if err != nil {
		t.Fatalf("recover stale socket: %v", err)
	}
	control.StopAccepting()
	control.Finish(nil)
	if err := os.WriteFile(path, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Listen(Status{DataDir: dir}, func() {}); err == nil {
		t.Fatal("replaced regular file")
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(dir, "target")
	if err := os.WriteFile(target, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, path); err != nil {
		t.Fatal(err)
	}
	if _, err := Listen(Status{DataDir: dir}, func() {}); err == nil {
		t.Fatal("replaced symlink")
	}
	if content, err := os.ReadFile(target); err != nil || string(content) != "keep" {
		t.Fatalf("target changed: %q %v", content, err)
	}
}

func TestStopTimeoutDoesNotClaimSuccessOrCancelCleanup(t *testing.T) {
	t.Setenv("INVOCATION_ID", "")
	dir := shortDirectory(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	control, err := Listen(Status{DataDir: dir}, cancel)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { control.StopAccepting(); control.Finish(nil) }()
	wait, stop := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer stop()
	_, err = Stop(wait, dir)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("stop error=%v", err)
	}
	if ctx.Err() == nil {
		t.Fatal("stop did not initiate cleanup")
	}
}

func TestAlreadyAcceptedStopCannotRevertCompletedState(t *testing.T) {
	t.Setenv("SING_BOX_PANEL_SUPERVISOR", "")
	control, err := Listen(Status{DataDir: shortDirectory(t)}, func() {})
	if err != nil {
		t.Fatal(err)
	}
	control.StopAccepting()
	control.Finish(nil)
	// A request accepted just before listener shutdown may reach its handler
	// after cleanup has already completed.
	writer := httptest.NewRecorder()
	control.serveStop(writer, httptest.NewRequest("POST", "/stop", nil))
	var result response
	if err := json.Unmarshal(writer.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Status.State != "stopped" {
		t.Fatalf("late stop changed completed state: %+v", result)
	}
}
