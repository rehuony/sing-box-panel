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

func TestOperationLoggingOmitsSensitiveErrorText(t *testing.T) {
	ctx := context.Background()
	database := openRunnerStore(t, ctx)
	commands := application.FromStore(database)
	commands.RecordOperation(ctx, "core.install", "Core installation", errors.New("token=must-not-be-persisted"), application.OperationLogContext{})
	page, err := commands.ListLogs(ctx, application.LogListRequest{Source: store.LogSourcePanel, Limit: 10})
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
		ID: "core-dashboard", ExactVersion: "1.13.19", OperatingSystem: "linux", Architecture: "arm64", Variant: "musl",
		SourceKind: store.CoreArtifactSourceUserVerified, UserSource: "dashboard fixture",
		ArchiveSHA256: strings.Repeat("a", 64), BinarySHA256: strings.Repeat("b", 64),
		BinaryPath: "/opt/sing-box-panel/core-dashboard/sing-box", ReportedVersion: "1.13.19",
		FeatureFingerprint: json.RawMessage(`{"status":"reported","features":["badlinkname","tfogo_checklinkname0","with_acme","with_ccm","with_clash_api","with_dhcp","with_gvisor","with_naive_outbound","with_ocm","with_purego","with_quic","with_tailscale","with_utls","with_wireguard"]}`),
		CreatedAt:          now,
	}
	if _, err := database.UpsertCoreArtifact(ctx, core); err != nil {
		t.Fatal(err)
	}
	canonicalSave, err := commands.SaveConfigurationFile(ctx, application.ConfigurationFileWrite{Content: "{}"})
	if err != nil {
		t.Fatal(err)
	}
	intent, err := database.RequestConfigurationRuntimeIntent(ctx, store.RuntimeIntentInput{Kind: store.RuntimeIntentRestart, SelectOnly: true, CreatedAt: now}, store.StartupArtifact{ID: "dashboard-startup", CanonicalRevisionID: canonicalSave.CanonicalRevisionID, CoreArtifactID: core.ID, ExactCoreVersion: core.ExactVersion, ConfigBytes: configuration.Empty().CanonicalJSON(), CreatedAt: now})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := commands.CompleteStartupCheck(ctx, intent.StartupArtifactID, true); err != nil {
		t.Fatal(err)
	}
	prepared, err := commands.PrepareActivationBundle(ctx, intent.StartupArtifactID, store.MonitoringProcessOnly)
	if err != nil {
		t.Fatal(err)
	}
	intent, err = database.BindCheckedRuntimeIntent(ctx, intent, prepared.Bundle.ID, now)
	if err != nil {
		t.Fatal(err)
	}
	if err := database.CompleteRuntimeIntent(ctx, intent, true, &store.RuntimeCommit{ClearObservation: true, Transitions: []store.RuntimeTransitionInput{{DedupeKey: "dashboard-selection", State: store.RuntimeTransitionStopped, Reason: "core_selected", Generation: intent.Generation, OccurredAt: now}}}, now); err != nil {
		t.Fatal(err)
	}
	provider := &statusProvider{database: database, commands: commands, build: buildinfo.Info{Version: "test"}}
	status, err := provider.SystemStatus(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if status.ConfigurationState != "schema@1.13.19" || status.AppliedBundleID == nil ||
		*status.AppliedBundleID != prepared.Bundle.ID || status.Running {
		t.Fatalf("system status = %+v", status)
	}
	contextValue, err := provider.DashboardContext(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if contextValue.Applied == nil || contextValue.Applied.Bundle != prepared.Bundle.ID ||
		contextValue.Applied.Revision != int64(1) ||
		contextValue.View.ExactVersion != core.ExactVersion || !contextValue.Configuration.Supported ||

		contextValue.Canonical.HasUnappliedChanges {
		t.Fatalf("dashboard context = %+v", contextValue)
	}
	if _, err := commands.SaveConfigurationFile(ctx, application.ConfigurationFileWrite{Revision: canonicalSave.Revision, Content: `{"log":{"level":"info"}}`}); err != nil {
		t.Fatal(err)
	}
	contextValue, err = provider.DashboardContext(ctx)
	if err != nil || !contextValue.Canonical.HasUnappliedChanges || contextValue.Applied == nil || contextValue.Applied.Bundle != prepared.Bundle.ID {
		t.Fatalf("dashboard after save = %+v, %v", contextValue, err)
	}
}
