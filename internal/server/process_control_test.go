// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux

package server

import (
	"context"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/rehuony/sing-box-panel/internal/buildinfo"
	"github.com/rehuony/sing-box-panel/internal/panelprocess"
	"github.com/rehuony/sing-box-panel/internal/settings"
)

func TestForegroundPanelCanBeStoppedFromAnotherClient(t *testing.T) {
	t.Setenv("INVOCATION_ID", "")
	value, path := processSettings(t)
	var err error
	for range 2 {
		ctx, cancel := context.WithCancel(context.Background())
		result := make(chan error, 1)
		go func() { result <- Run(ctx, path, buildinfo.Info{Version: "test"}, fstest.MapFS{}) }()
		wait, stop := context.WithTimeout(context.Background(), 10*time.Second)
		var status panelprocess.Status
		for {
			status, err = panelprocess.Inspect(wait, value.DataDir)
			if err == nil && status.State == "ready" {
				break
			}
			select {
			case err := <-result:
				cancel()
				stop()
				t.Fatalf("startup exited: %v", err)
			case <-wait.Done():
				cancel()
				stop()
				t.Fatalf("panel never ready: %v", err)
			case <-time.After(10 * time.Millisecond):
			}
		}
		if status.SettingsPath != path || status.PID != os.Getpid() {
			t.Fatalf("live status=%+v", status)
		}
		if err := Run(wait, path, buildinfo.Info{}, fstest.MapFS{}); err == nil || !strings.Contains(err.Error(), "owns this data directory") {
			t.Fatalf("duplicate start error=%v", err)
		}
		status, err = panelprocess.Stop(wait, value.DataDir)
		if err != nil || status.State != "stopped" {
			cancel()
			stop()
			t.Fatalf("stop=%+v err=%v", status, err)
		}
		lease, err := panelprocess.AcquireLease(value.DataDir)
		if err != nil {
			t.Fatalf("stop acknowledged before lease release: %v", err)
		}
		_ = lease.Close()
		if err := <-result; err != nil {
			t.Fatalf("foreground exit=%v", err)
		}
		cancel()
		stop()
	}
}

func processSettings(t *testing.T) (settings.Settings, string) {
	t.Helper()
	dir, err := os.MkdirTemp("/tmp", "sbp-server-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	value := settings.Defaults()
	value.DataDir = filepath.Join(dir, "data")
	value.Auth.Token = strings.Repeat("s", 32)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	value.Server.Port = listener.Addr().(*net.TCPAddr).Port
	_ = listener.Close()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "setting.json")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return value, path
}

func TestCanceledStartupReleasesControlAndLease(t *testing.T) {
	value, path := processSettings(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := Run(ctx, path, buildinfo.Info{}, fstest.MapFS{}); err != nil {
		t.Fatalf("canceled startup: %v", err)
	}
	status, err := panelprocess.Inspect(context.Background(), value.DataDir)
	if err != nil || status.State != "stopped" {
		t.Fatalf("status=%+v err=%v", status, err)
	}
	lease, err := panelprocess.AcquireLease(value.DataDir)
	if err != nil {
		t.Fatal(err)
	}
	_ = lease.Close()
}
