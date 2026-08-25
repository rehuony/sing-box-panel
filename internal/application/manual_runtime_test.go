// SPDX-License-Identifier: GPL-3.0-or-later

package application

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/rehuony/sing-box-panel/internal/canonical"
	"github.com/rehuony/sing-box-panel/internal/runtimeidentity"
	"github.com/rehuony/sing-box-panel/internal/store"
)

func TestManualReplacePreservesBytesAndQueuesBoundCheck(t *testing.T) {
	ctx := context.Background()
	database, err := store.Open(ctx, filepath.Join(t.TempDir(), "panel.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	app := newApplication(database)
	now := time.Date(2026, time.August, 26, 17, 0, 0, 0, time.UTC)
	app.now = func() time.Time { return now }
	core := applicationTestCore("core-a", "1.13.19", 'a', 'b', now)
	if _, err := database.UpsertCoreArtifact(ctx, core); err != nil {
		t.Fatal(err)
	}
	initial, err := app.ReplaceCanonical(ctx, "", canonical.Empty().CanonicalJSON())
	if err != nil {
		t.Fatal(err)
	}
	raw := []byte("{\n  // exact operator comment\n  \"log\": {\"level\": \"warn\"},\n}\n")
	saved, err := app.ReplaceManualJSON(ctx, ManualReplaceRequest{
		ExpectedHead: initial.Revision.ID, CoreVersion: "1.13.19",
		CoreArtifactID: core.ID, Raw: raw,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !saved.NoChange || saved.Revision.ID != initial.Revision.ID ||
		saved.Artifact.State != store.StartupArtifactPending || saved.Task.Kind != "startup-check" ||
		saved.Task.StartupArtifactID != saved.Artifact.ID || saved.Task.CanonicalRevisionID != initial.Revision.ID {
		t.Fatalf("manual save = %+v", saved)
	}
	if !bytes.Equal([]byte(saved.Artifact.Raw), raw) {
		t.Fatalf("manual bytes changed:\n%s", saved.Artifact.Raw)
	}
	loaded, err := app.ManualArtifact(ctx, saved.Artifact.ID)
	if err != nil || loaded.Raw != string(raw) {
		t.Fatalf("loaded manual = %+v, err=%v", loaded, err)
	}

	queuedAgain, err := app.QueueStartupCheck(ctx, saved.Artifact.ID)
	if err != nil || queuedAgain.ID != saved.Task.ID {
		t.Fatalf("idempotent check = %+v, err=%v", queuedAgain, err)
	}
}

func TestManualOmittedVersionUsesRunningIdentityAndNeverLatest(t *testing.T) {
	ctx := context.Background()
	database, err := store.Open(ctx, filepath.Join(t.TempDir(), "panel.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	app := newApplication(database)
	now := time.Date(2026, time.August, 26, 18, 0, 0, 0, time.UTC)
	core := applicationTestCore("core-running", "1.12.8", 'c', 'd', now)
	if _, err := database.UpsertCoreArtifact(ctx, core); err != nil {
		t.Fatal(err)
	}
	initial, err := app.ReplaceCanonical(ctx, "", canonical.Empty().CanonicalJSON())
	if err != nil {
		t.Fatal(err)
	}
	app.runtime = fakeRuntimeResolver{identity: runtimeidentity.Identity{
		PID: 77, ExactCoreVersion: core.ExactVersion, CoreArtifactID: core.ID,
	}}
	saved, err := app.ReplaceManualJSON(ctx, ManualReplaceRequest{
		ExpectedHead: initial.Revision.ID, Raw: []byte(`{"log":{}}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	if saved.Resolution.Source != "running" || saved.Artifact.ExactCoreVersion != "1.12.8" ||
		saved.Artifact.CoreArtifactID != core.ID {
		t.Fatalf("running-bound save = %+v", saved)
	}
}

func TestActivationPreparationAndRuntimeMaterialFreezeBinaryDigest(t *testing.T) {
	ctx := context.Background()
	database, err := store.Open(ctx, filepath.Join(t.TempDir(), "panel.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	app := newApplication(database)
	now := time.Date(2026, time.August, 26, 19, 0, 0, 0, time.UTC)
	app.now = func() time.Time { return now }
	core := applicationTestCore("core-runtime", "1.13.19", 'e', 'f', now)
	if _, err := database.UpsertCoreArtifact(ctx, core); err != nil {
		t.Fatal(err)
	}
	initial, err := app.ReplaceCanonical(ctx, "", canonical.Empty().CanonicalJSON())
	if err != nil {
		t.Fatal(err)
	}
	manual, err := app.ReplaceManualJSON(ctx, ManualReplaceRequest{
		ExpectedHead: initial.Revision.ID, CoreVersion: core.ExactVersion,
		CoreArtifactID: core.ID, Raw: []byte(`{"route":{}}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := app.CompleteStartupCheck(
		ctx, manual.Artifact.ID, true, json.RawMessage(`[{"code":"ok"}]`),
	); err != nil {
		t.Fatal(err)
	}
	prepared, task, err := app.PrepareAndQueueRuntimeApply(
		ctx, manual.Artifact.ID, store.MonitoringProcessOnly,
	)
	if err != nil {
		t.Fatal(err)
	}
	repeated, err := app.PrepareActivationBundle(ctx, manual.Artifact.ID, store.MonitoringProcessOnly)
	if err != nil || repeated.Bundle.ID != prepared.Bundle.ID {
		t.Fatalf("repeated preparation = %+v, err=%v", repeated, err)
	}
	if task.ActivationBundleID != prepared.Bundle.ID || task.Kind != string(store.RuntimeIntentApply) {
		t.Fatalf("runtime task = %+v", task)
	}
	material, err := app.LoadRuntimeMaterial(ctx, prepared.Bundle.ID)
	if err != nil {
		t.Fatal(err)
	}
	if material.Bundle.ArtifactDigest.String() != core.BinarySHA256 ||
		material.Bundle.ArtifactDigest.String() == core.ArchiveSHA256 ||
		!bytes.Equal(material.Bundle.StartupConfig, []byte(manual.Artifact.Raw)) {
		t.Fatalf("runtime material = %+v", material.Bundle)
	}
}

func applicationTestCore(
	id string,
	version string,
	archiveCharacter byte,
	binaryCharacter byte,
	createdAt time.Time,
) store.CoreArtifact {
	return store.CoreArtifact{
		ID: id, ExactVersion: version, OperatingSystem: "linux", Architecture: "amd64", Variant: "plain",
		SourceKind: store.CoreArtifactSourceOfficial, RepositoryID: 1, ReleaseID: int64(100 + archiveCharacter),
		AssetID: int64(200 + archiveCharacter), ArchiveSHA256: strings.Repeat(string(archiveCharacter), 64),
		BinarySHA256: strings.Repeat(string(binaryCharacter), 64), BinaryPath: "/secure/" + id + "/sing-box",
		ReportedVersion: version, FeatureFingerprint: json.RawMessage(`{"features":[]}`),
		VerificationState: store.CoreArtifactVerified, CreatedAt: createdAt,
	}
}
