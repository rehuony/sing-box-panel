// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux || darwin

package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/rehuony/sing-box-panel/internal/panelprocess"
	"github.com/rehuony/sing-box-panel/internal/store"
	panelSystemd "github.com/rehuony/sing-box-panel/internal/systemd"
)

func TestSystemPruneWaitsForPanelAndCoreShutdown(t *testing.T) {
	for _, outcome := range []string{"success", "shutdown failure"} {
		t.Run(outcome, func(t *testing.T) {
			t.Setenv("SING_BOX_PANEL_SUPERVISOR", "")
			// Keep the real Unix control socket within the OS path-length limit.
			base, err := os.MkdirTemp("/tmp", "sbp-prune-")
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = os.RemoveAll(base) })
			dataDir := filepath.Join(base, "data")
			if err := os.Mkdir(dataDir, 0o700); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(base, "setting.json")
			contents, err := json.Marshal(map[string]string{"data_dir": dataDir})
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, contents, 0o600); err != nil {
				t.Fatal(err)
			}
			lease, err := panelprocess.AcquireLease(dataDir)
			if err != nil {
				t.Fatal(err)
			}
			defer lease.Close()
			database, err := store.Open(t.Context(), filepath.Join(dataDir, "panel.db"))
			if err != nil {
				t.Fatal(err)
			}
			defer database.Close()
			// This real child represents the core's lifetime. Stop is acknowledged
			// only after joining it and releasing the panel's storage and lease.
			core := exec.Command("sleep", "60")
			if err := core.Start(); err != nil {
				t.Fatal(err)
			}
			coreWaited := false
			defer func() {
				if !coreWaited {
					_ = core.Process.Kill()
					_ = core.Wait()
				}
			}()
			panelContext, stopPanel := context.WithCancel(t.Context())
			defer stopPanel()
			control, err := panelprocess.Listen(panelprocess.Status{SettingsPath: path, DataDir: dataDir}, stopPanel)
			if err != nil {
				t.Fatal(err)
			}
			finished := false
			defer func() {
				if !finished {
					control.StopAccepting()
					control.Finish(errors.New("fixture interrupted"))
				}
			}()
			control.Ready("fixture")
			var stdout, stderr bytes.Buffer
			root := NewRootCommand(Dependencies{CleanupHistoryPath: testHistoryPath(t), Stdout: &stdout, Stderr: &stderr,
				Systemd: &fakeSystemdService{err: panelSystemd.ErrUnsupportedOS}})
			root.SetArgs([]string{"system", "prune", "-c", path, "--yes"})
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			result := make(chan error, 1)
			go func() { result <- root.ExecuteContext(ctx) }()
			select {
			case <-panelContext.Done():
			case err := <-result:
				t.Fatalf("prune did not request panel shutdown: %v", err)
			case <-ctx.Done():
				t.Fatal("panel did not receive shutdown request")
			}
			select {
			case err := <-result:
				t.Fatalf("prune returned before core shutdown: %v", err)
			default:
			}
			if err := core.Process.Signal(syscall.Signal(0)); err != nil {
				t.Fatalf("core fixture exited before shutdown: %v", err)
			}
			for _, retained := range []string{path, filepath.Join(dataDir, "panel.db")} {
				if _, err := os.Stat(retained); err != nil {
					t.Fatalf("prune removed files before core exited: %v", err)
				}
			}
			if err := core.Process.Signal(syscall.SIGTERM); err != nil {
				t.Fatal(err)
			}
			waitErr := core.Wait()
			coreWaited = true
			var exitErr *exec.ExitError
			if waitErr != nil && !errors.As(waitErr, &exitErr) {
				t.Fatalf("join core: %v", waitErr)
			}
			if err := database.Close(); err != nil {
				t.Fatal(err)
			}
			control.StopAccepting()
			if err := lease.Close(); err != nil {
				t.Fatal(err)
			}
			var shutdownErr error
			if outcome == "shutdown failure" {
				shutdownErr = errors.New("core shutdown failed")
			}
			control.Finish(shutdownErr)
			finished = true
			select {
			case err = <-result:
			case <-ctx.Done():
				t.Fatal("prune did not finish after panel shutdown")
			}
			if shutdownErr == nil {
				if err != nil {
					t.Fatalf("prune after shutdown: %v", err)
				}
				for _, removed := range []string{path, dataDir} {
					if _, err := os.Lstat(removed); !os.IsNotExist(err) {
						t.Fatalf("cleanup left %s: %v", removed, err)
					}
				}
			} else {
				if err == nil || !strings.Contains(err.Error(), shutdownErr.Error()) {
					t.Fatalf("lost panel shutdown failure: %v", err)
				}
				for _, retained := range []string{path, filepath.Join(dataDir, "panel.db")} {
					if _, err := os.Stat(retained); err != nil {
						t.Fatalf("cleanup continued after shutdown failure: %v", err)
					}
				}
			}
		})
	}
}
