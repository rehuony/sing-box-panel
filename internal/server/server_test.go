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
	"time"

	"github.com/rehuony/sing-box-panel/internal/application"
	"github.com/rehuony/sing-box-panel/internal/buildinfo"
	"github.com/rehuony/sing-box-panel/internal/configuration"
	"github.com/rehuony/sing-box-panel/internal/panelprocess"
	"github.com/rehuony/sing-box-panel/internal/store"
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

func TestRuntimeRestartAlwaysRequiresTransition(t *testing.T) {
	t.Parallel()

	if !runtimeIntentNeedsTransition(store.RuntimeIntentRestart, true) {
		t.Fatal("restart of an already exact bundle was treated as a no-op")
	}
	if runtimeIntentNeedsTransition(store.RuntimeIntentStart, true) {
		t.Fatal("start of an already exact bundle should remain idempotent")
	}
	if !runtimeIntentNeedsTransition(store.RuntimeIntentApply, false) {
		t.Fatal("a non-running apply must start the runtime")
	}
}

func TestRuntimeExecutorLeaseExcludesSecondOwner(t *testing.T) {
	dataDirectory := t.TempDir()
	first, err := panelprocess.AcquireLease(dataDirectory)
	if err != nil {
		t.Fatalf("acquire first runtime executor lease: %v", err)
	}
	t.Cleanup(func() { _ = first.Close() })

	if _, err := panelprocess.AcquireLease(dataDirectory); !errors.Is(err, panelprocess.ErrLeaseHeld) {
		t.Fatalf("acquire second runtime executor lease error = %v, want ErrRuntimeExecutorLeaseHeld", err)
	}

	if err := first.Close(); err != nil {
		t.Fatalf("release first runtime executor lease: %v", err)
	}
	third, err := panelprocess.AcquireLease(dataDirectory)
	if err != nil {
		t.Fatalf("acquire runtime executor lease after release: %v", err)
	}
	if err := third.Close(); err != nil {
		t.Fatalf("release third runtime executor lease: %v", err)
	}

	info, err := os.Stat(filepath.Join(dataDirectory, panelprocess.LeaseFileName))
	if err != nil {
		t.Fatalf("stat runtime executor lease: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("runtime executor lease permissions = %#o, want 0600", got)
	}
}

func TestTaskLoggingRecordsLifecycleWithoutPayloadOrErrorText(t *testing.T) {
	ctx := context.Background()
	database, err := store.Open(ctx, filepath.Join(t.TempDir(), "panel.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	commands := application.FromStore(database)
	wantErr := errors.New("token=must-not-be-persisted")
	commit := &store.RuntimeTaskCommit{}
	handler := withTaskLogging(commands, taskResultHandlerFunc(func(
		context.Context,
		store.Task,
		taskExecutionControl,
	) (taskHandlerResult, error) {
		return taskHandlerResult{Runtime: commit}, wantErr
	}))
	task := store.Task{
		ID: "task-log-test", Kind: store.TaskKindCoreInstall, Lane: store.TaskLaneMaintenance,
		Attempt: 2, Payload: json.RawMessage(`{"token":"also-must-not-be-persisted"}`),
	}
	result, err := handler.Handle(ctx, task, nil)
	if !errors.Is(err, wantErr) {
		t.Fatalf("Handle() error = %v", err)
	}
	if result.Runtime != commit {
		t.Fatal("task logging did not preserve runtime completion evidence")
	}
	page, err := commands.ListLogs(ctx, application.LogListRequest{Source: store.LogSourceTask, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 2 || page.Items[0].Code != "task.failed" || page.Items[1].Code != "task.started" {
		t.Fatalf("logs = %+v", page.Items)
	}
	encoded, err := json.Marshal(page.Items)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "must-not-be-persisted") || !strings.Contains(string(encoded), `"task_id":"task-log-test"`) {
		t.Fatalf("unsafe or incomplete logs: %s", encoded)
	}
}

func TestStatusProviderReadsCanonicalHead(t *testing.T) {
	ctx := context.Background()
	dataDir := t.TempDir()
	database, err := store.Open(ctx, filepath.Join(dataDir, "panel.db"))
	if err != nil {
		t.Fatalf("store.Open() error = %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })

	provider := &statusProvider{database: database, build: buildinfo.Info{Version: "test-version"}}
	status, err := provider.SystemStatus(ctx)
	if err != nil {
		t.Fatalf("SystemStatus() error = %v", err)
	}
	if status.PanelVersion != "test-version" || status.CanonicalRevision != 0 || status.AppliedBundleID != nil ||
		status.ConfigurationState != "unresolved" {
		t.Fatalf("SystemStatus() = %+v", status)
	}
}

func TestDashboardContextUsesAppliedBundleAndConfigurationSupport(t *testing.T) {
	ctx := context.Background()
	database, err := store.Open(ctx, filepath.Join(t.TempDir(), "panel.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	commands := application.FromStore(database)
	now := time.Date(2026, time.August, 26, 12, 0, 0, 0, time.UTC)
	core := store.CoreArtifact{
		ID: "core-dashboard", ExactVersion: "1.13.19", OperatingSystem: "linux", Architecture: "arm64", Variant: "plain",
		SourceKind: store.CoreArtifactSourceUserVerified, UserSource: "dashboard fixture",
		ArchiveSHA256: strings.Repeat("a", 64), BinarySHA256: strings.Repeat("b", 64),
		BinaryPath: "/opt/sing-box-panel/core-dashboard/sing-box", ReportedVersion: "1.13.19",
		FeatureFingerprint: json.RawMessage(`{"status":"reported","features":["badlinkname","tfogo_checklinkname0","with_acme","with_ccm","with_clash_api","with_dhcp","with_gvisor","with_naive_outbound","with_ocm","with_purego","with_quic","with_tailscale","with_utls","with_wireguard"]}`),
		CreatedAt:          now,
	}
	if _, err := database.UpsertCoreArtifact(ctx, core); err != nil {
		t.Fatal(err)
	}
	canonicalSave, err := commands.ReplaceCanonical(ctx, "", configuration.Empty().CanonicalJSON())
	if err != nil {
		t.Fatal(err)
	}
	compiled, err := commands.CompileConfiguration(ctx, application.ConfigurationCompileRequest{CoreArtifactID: core.ID})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := commands.CompleteStartupCheck(ctx, compiled.Artifact.ID, true); err != nil {
		t.Fatal(err)
	}
	prepared, task, err := commands.PrepareAndQueueRuntimeApply(ctx, compiled.Artifact.ID, store.MonitoringProcessOnly)
	if err != nil {
		t.Fatal(err)
	}
	claimed, err := database.ClaimTask(ctx, store.ClaimTaskInput{
		Lane: store.TaskLaneRuntime, LeaseOwner: "dashboard-worker", Now: time.Now().UTC().Add(time.Second), LeaseDuration: time.Minute,
	})
	if err != nil || claimed == nil || claimed.ID != task.ID {
		t.Fatalf("claim = %+v, %v", claimed, err)
	}
	completedAt := time.Now().UTC().Add(2 * time.Second)
	if _, err := database.CompleteTask(ctx, claimed.ID, claimed.LeaseOwner, completedAt, store.TaskCompletion{Succeeded: true}); err != nil {
		t.Fatal(err)
	}
	provider := &statusProvider{database: database, commands: commands, build: buildinfo.Info{Version: "test"}}
	status, err := provider.SystemStatus(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if status.ConfigurationState != "raw" || status.AppliedBundleID == nil ||
		*status.AppliedBundleID != prepared.Bundle.ID || status.Running {
		t.Fatalf("system status = %+v", status)
	}
	contextValue, err := provider.DashboardContext(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if contextValue.Applied == nil || contextValue.Applied.Bundle != prepared.Bundle.ID ||
		contextValue.Applied.Revision != canonicalSave.Revision.Sequence ||
		contextValue.View.ExactVersion != core.ExactVersion || contextValue.Configuration.Supported ||
		contextValue.Configuration.Label != "Raw JSON" ||
		contextValue.Canonical.HasUnappliedChanges {
		t.Fatalf("dashboard context = %+v", contextValue)
	}
	if _, err := commands.PatchCanonical(ctx, canonicalSave.Revision.ID, []application.CanonicalChange{{Operation: "set", Path: "/log", ValueJSON: `{"level":"info"}`}}); err != nil {
		t.Fatal(err)
	}
	contextValue, err = provider.DashboardContext(ctx)
	if err != nil || !contextValue.Canonical.HasUnappliedChanges || contextValue.Applied == nil || contextValue.Applied.Bundle != prepared.Bundle.ID {
		t.Fatalf("dashboard after save = %+v, %v", contextValue, err)
	}
}
