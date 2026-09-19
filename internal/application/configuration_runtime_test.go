// SPDX-License-Identifier: GPL-3.0-or-later

package application

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/rehuony/sing-box-panel/internal/store"
)

func TestAppliedCoreArtifactIDFollowsAppliedBundle(t *testing.T) {
	ctx := context.Background()
	database, err := store.Open(ctx, filepath.Join(t.TempDir(), "panel.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	app := FromStore(database)

	coreID, err := app.AppliedCoreArtifactID(ctx)
	if err != nil || coreID != "" {
		t.Fatalf("before apply: id=%q err=%v", coreID, err)
	}

	now := app.now().UTC()
	core := store.CoreArtifact{
		ID: "core-applied", ExactVersion: "1.13.19", OperatingSystem: "linux", Architecture: "arm64", Variant: "plain",
		SourceKind: store.CoreArtifactSourceUserVerified, UserSource: "test", ArchiveSHA256: strings.Repeat("a", 64),
		BinarySHA256: strings.Repeat("b", 64), BinaryPath: "/tmp/sing-box", ReportedVersion: "1.13.19",
		FeatureFingerprint: json.RawMessage(`{"status":"reported","features":[]}`),
		VerificationState:  store.CoreArtifactVerified, CreatedAt: now,
	}
	if _, err := database.UpsertCoreArtifact(ctx, core); err != nil {
		t.Fatal(err)
	}
	startupBytes := []byte(`{}`)
	saved, err := app.ReplaceConfiguration(ctx, "", startupBytes)
	if err != nil {
		t.Fatal(err)
	}
	ready, err := database.CreateStartupArtifact(ctx, store.StartupArtifact{
		ID: "startup-applied", CanonicalRevisionID: saved.Revision.ID, ExactCoreVersion: core.ExactVersion,
		CoreArtifactID: core.ID, ConfigBytes: startupBytes, CreatedAt: now.Add(time.Second),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.CompleteStartupArtifactCheck(ctx, ready.ID, true, now.Add(2*time.Second)); err != nil {
		t.Fatal(err)
	}
	prepared, err := app.PrepareActivationBundle(ctx, ready.ID, store.MonitoringProcessOnly)
	if err != nil {
		t.Fatal(err)
	}
	task, err := app.QueueRuntimeApply(ctx, prepared.Bundle.ID)
	if err != nil {
		t.Fatal(err)
	}
	claimed, err := database.ClaimTask(ctx, store.ClaimTaskInput{
		Lane: store.TaskLaneRuntime, LeaseOwner: "applied-test", Now: now.Add(3 * time.Second), LeaseDuration: time.Minute,
	})
	if err != nil || claimed == nil || claimed.ID != task.ID {
		t.Fatalf("claim apply task = %+v, %v", claimed, err)
	}
	if _, err := database.CompleteTask(ctx, claimed.ID, claimed.LeaseOwner, now.Add(4*time.Second), store.TaskCompletion{Succeeded: true}); err != nil {
		t.Fatal(err)
	}

	coreID, err = app.AppliedCoreArtifactID(ctx)
	if err != nil || coreID != core.ID {
		t.Fatalf("after apply: id=%q err=%v", coreID, err)
	}
}
