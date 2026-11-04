// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rehuony/sing-box-panel/internal/procctl"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/app"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/settings"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/store"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/telemetry"
)

func TestPrepareDataDirectory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "data")
	if err := prepareDataDirectory(path); err != nil {
		t.Fatalf("prepareDataDirectory() error = %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat data directory: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o700 {
		t.Fatalf("data directory permissions = %#o, want %#o", got, os.FileMode(0o700))
	}
}

func TestRuntimeExecutorLeaseExcludesSecondOwner(t *testing.T) {
	dataDirectory := t.TempDir()
	first, err := procctl.AcquireLease(dataDirectory)
	if err != nil {
		t.Fatalf("acquire first runtime executor lease: %v", err)
	}
	t.Cleanup(func() { _ = first.Close() })

	if _, err := procctl.AcquireLease(dataDirectory); !errors.Is(err, procctl.ErrLeaseHeld) {
		t.Fatalf("acquire second runtime executor lease error = %v, want ErrRuntimeExecutorLeaseHeld", err)
	}

	if err := first.Close(); err != nil {
		t.Fatalf("release first runtime executor lease: %v", err)
	}
	third, err := procctl.AcquireLease(dataDirectory)
	if err != nil {
		t.Fatalf("acquire runtime executor lease after release: %v", err)
	}
	if err := third.Close(); err != nil {
		t.Fatalf("release third runtime executor lease: %v", err)
	}

	info, err := os.Stat(filepath.Join(dataDirectory, procctl.LeaseFileName))
	if err != nil {
		t.Fatalf("stat runtime executor lease: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("runtime executor lease permissions = %#o, want 0600", got)
	}
}

func TestOperationLoggingOmitsSensitiveErrorText(t *testing.T) {
	ctx := context.Background()
	database := openRunnerStore(t, ctx)
	commands := app.New(database, settings.Settings{}, app.Options{})
	commands.Telemetry.RecordOperation(ctx, "core.install", "Core installation", errors.New("token=must-not-be-persisted"), telemetry.OperationLogContext{})
	page, err := commands.Telemetry.ListLogs(ctx, telemetry.LogListRequest{Source: store.LogSourcePanel, Limit: 10})
	if err != nil || len(page.Items) != 1 || page.Items[0].Code != "core.install.failed" {
		t.Fatalf("logs: %+v %v", page, err)
	}
	encoded, err := json.Marshal(page.Items)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "must-not-be-persisted") {
		t.Fatalf("unsafe logs: %s", encoded)
	}
}

func openRunnerStore(t *testing.T, ctx context.Context) *store.Store {
	t.Helper()
	database, err := store.Open(ctx, filepath.Join(t.TempDir(), "panel.db"))
	if err != nil {
		t.Fatalf("store.Open() error = %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	return database
}
