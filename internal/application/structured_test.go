// SPDX-License-Identifier: GPL-3.0-or-later

package application

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/rehuony/sing-box-panel/internal/capability"
	"github.com/rehuony/sing-box-panel/internal/coreartifact"
	"github.com/rehuony/sing-box-panel/internal/store"
)

func TestRenderStructuredPinsExactProjectionAndQueuesCheckAtomically(t *testing.T) {
	ctx := context.Background()
	database, err := store.Open(ctx, filepath.Join(t.TempDir(), "panel.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	app := newApplication(database)
	now := time.Date(2026, time.August, 26, 20, 0, 0, 0, time.UTC)
	app.now = func() time.Time { return now }
	core := applicationTestCore("core-structured", "1.13.19", 'a', 'b', now)
	if _, err := database.UpsertCoreArtifact(ctx, core); err != nil {
		t.Fatal(err)
	}
	canonicalSave, err := app.ReplaceCanonical(ctx, "", []byte(`{"schema_version":1,"global":{"mode":"direct"},"nodes":[],"rules":[],"subscription":{}}`))
	if err != nil {
		t.Fatal(err)
	}
	commit, digest := pinStructuredCapability(t, ctx, app, "1.13.19", capability.SupportNativeStructured)

	rendered, err := app.RenderStructured(ctx, StructuredRenderRequest{
		CoreVersion: "1.13.19", CoreArtifactID: core.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if rendered.Resolution.Source != "explicit" || rendered.Pin.CommitSHA != commit ||
		rendered.Artifact.CapabilityDigest != digest || rendered.Artifact.State != store.StartupArtifactPending ||
		rendered.Task.Kind != "startup-check" || rendered.Task.StartupArtifactID != rendered.Artifact.ID {
		t.Fatalf("rendered = %+v", rendered)
	}
	if string(rendered.Artifact.Config) != `{"route":{"final":"direct"}}` {
		t.Fatalf("projected config = %s", rendered.Artifact.Config)
	}
	listed, err := app.ListStartupArtifacts(ctx, StartupArtifactListRequest{
		ExactCoreVersion: "1.13.19", Kind: store.StartupArtifactStructured, Limit: 10,
	})
	if err != nil || len(listed.Items) != 1 || listed.Items[0].ID != rendered.Artifact.ID {
		t.Fatalf("listed startup artifacts = %+v, %v", listed, err)
	}
	queued, err := app.QueueStartupCheck(ctx, rendered.Artifact.ID)
	if err != nil || queued.ID != rendered.Task.ID {
		t.Fatalf("idempotent startup check = %+v, %v", queued, err)
	}
	if _, err := app.CompleteStartupCheck(ctx, rendered.Artifact.ID, true, nil); err != nil {
		t.Fatal(err)
	}
	detached, err := app.DetachManualJSON(ctx, rendered.Artifact.ID)
	if err != nil {
		t.Fatal(err)
	}
	if detached.Artifact.Raw != string(rendered.Artifact.Config) ||
		detached.Artifact.ExactCoreVersion != rendered.Artifact.ExactCoreVersion ||
		detached.Task.Kind != "startup-check" {
		t.Fatalf("detached = %+v", detached)
	}
	if _, err := app.DetachManualJSON(ctx, detached.Artifact.ID); !errors.Is(err, ErrManualDetachSource) {
		t.Fatalf("manual source detach error = %v", err)
	}
	if _, err := app.PrepareActivationBundle(ctx, rendered.Artifact.ID, store.MonitoringProcessOnly); err != nil {
		t.Fatalf("current structured candidate was not activatable: %v", err)
	}
	if _, err := app.SetCanonicalValue(ctx, canonicalSave.Revision.ID, "/global/mode", json.RawMessage(`"block"`)); err != nil {
		t.Fatal(err)
	}
	if _, err := app.PrepareActivationBundle(ctx, rendered.Artifact.ID, store.MonitoringProcessOnly); !errors.Is(err, store.ErrActivationBundleNotReady) {
		t.Fatalf("stale structured candidate activation error = %v", err)
	}
}

func TestRenderStructuredRequiresExplicitCompatibleAcceptance(t *testing.T) {
	ctx := context.Background()
	database, err := store.Open(ctx, filepath.Join(t.TempDir(), "panel.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	app := newApplication(database)
	now := time.Date(2026, time.August, 26, 21, 0, 0, 0, time.UTC)
	core := applicationTestCore("core-compatible", "1.12.8", 'c', 'd', now)
	if _, err := database.UpsertCoreArtifact(ctx, core); err != nil {
		t.Fatal(err)
	}
	if _, err := app.ReplaceCanonical(ctx, "", []byte(`{"schema_version":1,"global":{"mode":"direct"},"nodes":[],"rules":[],"subscription":{}}`)); err != nil {
		t.Fatal(err)
	}
	pinStructuredCapability(t, ctx, app, "1.12.8", capability.SupportCompatibleStructured)

	request := StructuredRenderRequest{CoreVersion: "1.12.8", CoreArtifactID: core.ID}
	if _, err := app.RenderStructured(ctx, request); !errors.Is(err, ErrCompatibleCapabilityNotAccepted) {
		t.Fatalf("compatible render error = %v", err)
	}
	request.AllowCompatible = true
	if _, err := app.RenderStructured(ctx, request); err != nil {
		t.Fatalf("accepted compatible render: %v", err)
	}
}

func TestActivationPreparationRejectsCapabilityPinMovedAfterPrecheck(t *testing.T) {
	ctx := context.Background()
	database, err := store.Open(ctx, filepath.Join(t.TempDir(), "panel.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	app := newApplication(database)
	now := time.Date(2026, time.August, 26, 21, 30, 0, 0, time.UTC)
	app.now = func() time.Time { return now }
	core := applicationTestCore("core-moved-pin", "1.13.19", 'e', 'f', now)
	if _, err := database.UpsertCoreArtifact(ctx, core); err != nil {
		t.Fatal(err)
	}
	if _, err := app.ReplaceCanonical(ctx, "", []byte(`{"schema_version":1,"global":{"mode":"direct"},"nodes":[],"rules":[],"subscription":{}}`)); err != nil {
		t.Fatal(err)
	}
	originalCommit, originalDigest := pinStructuredCapability(
		t, ctx, app, core.ExactVersion, capability.SupportNativeStructured,
	)
	rendered, err := app.RenderStructured(ctx, StructuredRenderRequest{
		CoreVersion: core.ExactVersion, CoreArtifactID: core.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := app.CompleteStartupCheck(ctx, rendered.Artifact.ID, true, nil); err != nil {
		t.Fatal(err)
	}
	startup, err := database.GetStartupArtifact(ctx, rendered.Artifact.ID)
	if err != nil {
		t.Fatal(err)
	}
	if startup.CapabilityCommit != originalCommit || startup.CapabilityDigest != originalDigest {
		t.Fatalf("structured startup pin = %s/%s, want %s/%s", startup.CapabilityCommit, startup.CapabilityDigest, originalCommit, originalDigest)
	}
	if err := app.verifyActivationCandidate(ctx, startup); err != nil {
		t.Fatalf("initial activation precheck: %v", err)
	}
	content, sourceSnapshots, err := app.prepareSubscriptionFreeze(ctx, startup)
	if err != nil {
		t.Fatalf("prepare subscription freeze: %v", err)
	}

	movedCommit := strings.Repeat("8", 40)
	movedGeneration, movedDigest := applicationCapabilityGeneration(t, movedCommit, core.ExactVersion)
	if _, err := app.RefreshCapabilityGeneration(ctx, movedGeneration); err != nil {
		t.Fatal(err)
	}
	if _, err := app.UpgradeCapability(ctx, CapabilityUpgradeRequest{
		ExactCoreVersion: core.ExactVersion,
		CommitSHA:        movedCommit,
		ManifestSHA256:   movedDigest,
	}); err != nil {
		t.Fatal(err)
	}

	publicAddresses := json.RawMessage(`{}`)
	snapshotID := stableRuntimeID("snapshot", startup.ID, string(content))
	bundleID := stableRuntimeID(
		"bundle", startup.ID, snapshotID, string(publicAddresses),
		string(sourceSnapshots), string(store.MonitoringProcessOnly),
	)
	_, err = database.SaveActivationBundle(ctx, store.SubscriptionSnapshot{
		ID: snapshotID, CanonicalRevisionID: startup.CanonicalRevisionID,
		StartupArtifactID: startup.ID, Content: content, CreatedAt: now,
	}, store.ActivationBundle{
		ID: bundleID, StartupArtifactID: startup.ID, SubscriptionSnapshotID: snapshotID,
		PublicAddresses: publicAddresses, SourceSnapshots: sourceSnapshots,
		MonitoringTier: store.MonitoringProcessOnly, CreatedAt: now,
	})
	if !errors.Is(err, store.ErrActivationBundleNotReady) {
		t.Fatalf("SaveActivationBundle() after pin move error = %v, want ErrActivationBundleNotReady", err)
	}
	if _, err := database.GetSubscriptionSnapshot(ctx, snapshotID); !errors.Is(err, store.ErrSubscriptionSnapshotNotFound) {
		t.Fatalf("rejected application preparation left subscription snapshot: %v", err)
	}
	if _, err := database.GetActivationBundle(ctx, bundleID); !errors.Is(err, store.ErrActivationBundleNotFound) {
		t.Fatalf("rejected application preparation left activation bundle: %v", err)
	}
}

func TestActivationPreparationRejectsCanonicalHeadMovedAfterPrecheck(t *testing.T) {
	ctx := context.Background()
	database, err := store.Open(ctx, filepath.Join(t.TempDir(), "panel.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	app := newApplication(database)
	now := time.Date(2026, time.August, 26, 21, 40, 0, 0, time.UTC)
	app.now = func() time.Time { return now }
	core := applicationTestCore("core-moved-head", "1.13.20", '1', '2', now)
	if _, err := database.UpsertCoreArtifact(ctx, core); err != nil {
		t.Fatal(err)
	}
	canonicalSave, err := app.ReplaceCanonical(ctx, "", []byte(`{"schema_version":1,"global":{"mode":"direct"},"nodes":[],"rules":[],"subscription":{}}`))
	if err != nil {
		t.Fatal(err)
	}
	pinStructuredCapability(t, ctx, app, core.ExactVersion, capability.SupportNativeStructured)
	rendered, err := app.RenderStructured(ctx, StructuredRenderRequest{
		CoreVersion: core.ExactVersion, CoreArtifactID: core.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := app.CompleteStartupCheck(ctx, rendered.Artifact.ID, true, nil); err != nil {
		t.Fatal(err)
	}
	startup, err := database.GetStartupArtifact(ctx, rendered.Artifact.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := app.verifyActivationCandidate(ctx, startup); err != nil {
		t.Fatalf("initial activation precheck: %v", err)
	}
	content, sourceSnapshots, err := app.prepareSubscriptionFreeze(ctx, startup)
	if err != nil {
		t.Fatalf("prepare subscription freeze: %v", err)
	}

	if _, err := app.SetCanonicalValue(
		ctx,
		canonicalSave.Revision.ID,
		"/global/mode",
		json.RawMessage(`"block"`),
	); err != nil {
		t.Fatal(err)
	}
	current, err := database.GetStartupArtifact(ctx, startup.ID)
	if err != nil || current.State != store.StartupArtifactReady {
		t.Fatalf("structured startup after head move = %+v, %v; want ready to exercise store fence", current, err)
	}

	publicAddresses := json.RawMessage(`{}`)
	snapshotID := stableRuntimeID("snapshot", startup.ID, string(content))
	bundleID := stableRuntimeID(
		"bundle", startup.ID, snapshotID, string(publicAddresses),
		string(sourceSnapshots), string(store.MonitoringProcessOnly),
	)
	_, err = database.SaveActivationBundle(ctx, store.SubscriptionSnapshot{
		ID: snapshotID, CanonicalRevisionID: startup.CanonicalRevisionID,
		StartupArtifactID: startup.ID, Content: content, CreatedAt: now,
	}, store.ActivationBundle{
		ID: bundleID, StartupArtifactID: startup.ID, SubscriptionSnapshotID: snapshotID,
		PublicAddresses: publicAddresses, SourceSnapshots: sourceSnapshots,
		MonitoringTier: store.MonitoringProcessOnly, CreatedAt: now,
	})
	if !errors.Is(err, store.ErrActivationBundleNotReady) {
		t.Fatalf("SaveActivationBundle() after head move error = %v, want ErrActivationBundleNotReady", err)
	}
	if _, err := database.GetSubscriptionSnapshot(ctx, snapshotID); !errors.Is(err, store.ErrSubscriptionSnapshotNotFound) {
		t.Fatalf("rejected application preparation left subscription snapshot: %v", err)
	}
	if _, err := database.GetActivationBundle(ctx, bundleID); !errors.Is(err, store.ErrActivationBundleNotFound) {
		t.Fatalf("rejected application preparation left activation bundle: %v", err)
	}
}

func pinStructuredCapability(
	t *testing.T,
	ctx context.Context,
	app *Application,
	exactVersion string,
	support capability.SupportLevel,
) (string, string) {
	t.Helper()
	version, err := coreartifact.ParseExactVersion(exactVersion)
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := capability.NewManifest(capability.ManifestSpec{
		SchemaVersion: capability.ManifestSchemaVersion,
		CoreVersion:   version,
		SupportLevel:  support,
		SemanticFacts: []capability.SemanticFact{{
			ID: "global.mode", CanonicalPath: "/global/mode",
			Classification: capability.CoverageSupported, OwnedPaths: []string{"/route/final"},
		}},
		Transforms: []capability.Transform{{
			ID: "global.mode.rename", FactID: "global.mode", Primitive: capability.PrimitiveRename,
			From: []string{"/global/mode"}, To: []string{"/route/final"},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	manifestJSON, err := manifest.CanonicalJSON()
	if err != nil {
		t.Fatal(err)
	}
	digest, err := manifest.Digest()
	if err != nil {
		t.Fatal(err)
	}
	commit := strings.Repeat("9", 40)
	envelope := map[string]any{
		"schema_version": capability.GenerationSchemaVersion,
		"repository":     capability.ManifestRepository,
		"commit_sha":     commit,
		"manifest_count": 1,
		"manifests": []map[string]any{{
			"path":            "capabilities/" + exactVersion + ".json",
			"manifest_sha256": digest.String(),
			"manifest":        json.RawMessage(manifestJSON),
		}},
	}
	source, err := json.Marshal(envelope)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := app.RefreshCapabilityGeneration(ctx, source); err != nil {
		t.Fatal(err)
	}
	if _, err := app.UpgradeCapability(ctx, CapabilityUpgradeRequest{
		ExactCoreVersion: exactVersion, CommitSHA: commit, ManifestSHA256: digest.String(),
	}); err != nil {
		t.Fatal(err)
	}
	return commit, digest.String()
}
