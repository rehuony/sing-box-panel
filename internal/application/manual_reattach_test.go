// SPDX-License-Identifier: GPL-3.0-or-later

package application

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/rehuony/sing-box-panel/internal/capability"
	"github.com/rehuony/sing-box-panel/internal/coreartifact"
	"github.com/rehuony/sing-box-panel/internal/reconcile"
	"github.com/rehuony/sing-box-panel/internal/store"
)

func TestManualReattachPreviewAndApplyMergeNonOverlappingChanges(t *testing.T) {
	fixture := newManualReattachFixture(t, true)
	current := fixture.replaceCanonical(`{
		"schema_version":1,
		"global":{"mode":"direct","dns":"current"},
		"nodes":[],"rules":[],"subscription":{}
	}`)

	preview, err := fixture.application.PreviewManualReattach(fixture.ctx, fixture.manual.Artifact.ID)
	if err != nil {
		t.Fatal(err)
	}
	if preview.Base.ID != fixture.base.Revision.ID || preview.Current.ID != current.Revision.ID ||
		preview.Evidence.CurrentHeadID != current.Revision.ID ||
		preview.Evidence.Capability.ManifestSHA256 != fixture.manifestDigest ||
		len(preview.Conflicts) != 0 {
		t.Fatalf("preview identity/conflicts = %+v", preview)
	}
	if len(preview.ResidualPaths) != 1 || preview.ResidualPaths[0] != "/unknown/keep" {
		t.Fatalf("residual paths = %#v", preview.ResidualPaths)
	}
	var merged map[string]any
	if err := json.Unmarshal(preview.Merged, &merged); err != nil {
		t.Fatal(err)
	}
	global := merged["global"].(map[string]any)
	if global["mode"] != "block" || global["dns"] != "current" {
		t.Fatalf("merged global = %#v", global)
	}

	saved, err := fixture.application.ApplyManualReattach(fixture.ctx, ManualReattachApplyRequest{
		StartupArtifactID: fixture.manual.Artifact.ID,
		Evidence:          preview.Evidence,
		Decisions:         map[string]reconcile.Choice{},
	})
	if err != nil {
		t.Fatal(err)
	}
	if saved.Revision.ID == current.Revision.ID || saved.Revision.ParentID != current.Revision.ID ||
		saved.Artifact.ID == fixture.manual.Artifact.ID ||
		saved.Artifact.CanonicalRevisionID != saved.Revision.ID ||
		saved.Artifact.State != store.StartupArtifactPending ||
		saved.Task.Kind != "startup-check" || saved.Task.StartupArtifactID != saved.Artifact.ID {
		t.Fatalf("reattach save = %+v", saved)
	}
	if !bytes.Equal([]byte(saved.Artifact.Raw), []byte(fixture.manual.Artifact.Raw)) {
		t.Fatal("reattach changed exact manual bytes")
	}
	var taskPayload struct {
		ReattachEvidence ManualReattachEvidence      `json:"reattach_evidence"`
		Decisions        map[string]reconcile.Choice `json:"conflict_decisions"`
	}
	if err := json.Unmarshal(saved.Task.Payload, &taskPayload); err != nil ||
		taskPayload.ReattachEvidence != preview.Evidence || taskPayload.Decisions == nil {
		t.Fatalf("durable reattach evidence = %+v, %v", taskPayload, err)
	}
	source, err := fixture.database.GetStartupArtifact(fixture.ctx, fixture.manual.Artifact.ID)
	if err != nil || source.State != store.StartupArtifactStale {
		t.Fatalf("source = %+v, %v; want stale", source, err)
	}
	storedRevision, err := fixture.database.GetCanonicalRevision(fixture.ctx, saved.Revision.ID)
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]any
	if err := json.Unmarshal(storedRevision.Document, &document); err != nil {
		t.Fatal(err)
	}
	storedGlobal := document["global"].(map[string]any)
	if storedGlobal["mode"] != "block" || storedGlobal["dns"] != "current" {
		t.Fatalf("stored global = %#v", storedGlobal)
	}
}

func TestManualReattachRequiresEveryCurrentConflictDecision(t *testing.T) {
	fixture := newManualReattachFixture(t, true)
	fixture.replaceCanonical(`{
		"schema_version":1,
		"global":{"mode":"alternate","dns":"base"},
		"nodes":[],"rules":[],"subscription":{}
	}`)
	preview, err := fixture.application.PreviewManualReattach(fixture.ctx, fixture.manual.Artifact.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(preview.Conflicts) != 1 || preview.Conflicts[0].Path != "/global/mode" {
		t.Fatalf("conflicts = %+v", preview.Conflicts)
	}
	_, err = fixture.application.ApplyManualReattach(fixture.ctx, ManualReattachApplyRequest{
		StartupArtifactID: fixture.manual.Artifact.ID,
		Evidence:          preview.Evidence,
	})
	if !IsManualReattachUnresolved(err) {
		t.Fatalf("missing decision error = %v", err)
	}
	headAfterFailure, err := fixture.application.CanonicalHead(fixture.ctx)
	if err != nil || headAfterFailure == nil || headAfterFailure.ID != preview.Current.ID {
		t.Fatalf("failed apply changed head = %+v, %v", headAfterFailure, err)
	}

	saved, err := fixture.application.ApplyManualReattach(fixture.ctx, ManualReattachApplyRequest{
		StartupArtifactID: fixture.manual.Artifact.ID,
		Evidence:          preview.Evidence,
		Decisions: map[string]reconcile.Choice{
			"/global/mode": reconcile.ChooseManual,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]any
	if err := json.Unmarshal(saved.Revision.Document, &document); err != nil {
		t.Fatal(err)
	}
	if document["global"].(map[string]any)["mode"] != "block" {
		t.Fatalf("manual conflict decision was not applied: %s", saved.Revision.Document)
	}
}

func TestManualReattachRejectsStalePreviewWithoutWrites(t *testing.T) {
	fixture := newManualReattachFixture(t, true)
	preview, err := fixture.application.PreviewManualReattach(fixture.ctx, fixture.manual.Artifact.ID)
	if err != nil {
		t.Fatal(err)
	}
	newHead := fixture.replaceCanonical(`{
		"schema_version":1,
		"global":{"mode":"direct","dns":"later"},
		"nodes":[],"rules":[],"subscription":{}
	}`)
	_, err = fixture.application.ApplyManualReattach(fixture.ctx, ManualReattachApplyRequest{
		StartupArtifactID: fixture.manual.Artifact.ID,
		Evidence:          preview.Evidence,
		Decisions:         map[string]reconcile.Choice{},
	})
	if !IsManualReattachPreviewStale(err) {
		t.Fatalf("stale preview error = %v", err)
	}
	head, err := fixture.application.CanonicalHead(fixture.ctx)
	if err != nil || head == nil || head.ID != newHead.Revision.ID {
		t.Fatalf("stale apply changed head = %+v, %v", head, err)
	}
}

func TestManualReattachUnavailableWithoutExactUsablePin(t *testing.T) {
	t.Run("missing pin", func(t *testing.T) {
		fixture := newManualReattachFixture(t, false)
		_, err := fixture.application.PreviewManualReattach(fixture.ctx, fixture.manual.Artifact.ID)
		if !IsManualReattachUnavailable(err) || !strings.Contains(err.Error(), "no usable pinned") {
			t.Fatalf("missing pin error = %v", err)
		}
	})

	t.Run("version mismatch", func(t *testing.T) {
		fixture := newManualReattachFixture(t, false)
		fixture.pinManifest("1.13.18")
		_, err := fixture.application.PreviewManualReattach(fixture.ctx, fixture.manual.Artifact.ID)
		if !IsManualReattachUnavailable(err) || !strings.Contains(err.Error(), "1.13.19") {
			t.Fatalf("version mismatch error = %v", err)
		}
	})

	t.Run("quarantine", func(t *testing.T) {
		fixture := newManualReattachFixture(t, true)
		if _, err := fixture.database.UpsertCapabilityQuarantine(fixture.ctx, store.CapabilityQuarantine{
			ManifestSHA256: fixture.manifestDigest,
			ReasonCode:     "round_trip_failed",
			Diagnostics:    json.RawMessage(`{"fixture":"reattach"}`),
		}); err != nil {
			t.Fatal(err)
		}
		_, err := fixture.application.PreviewManualReattach(fixture.ctx, fixture.manual.Artifact.ID)
		if !IsManualReattachUnavailable(err) || !strings.Contains(err.Error(), "quarantined") {
			t.Fatalf("quarantine error = %v", err)
		}
	})
}

func TestManualReplacePreviewsAndCommitsOnlyProvenReverseMapping(t *testing.T) {
	fixture := newManualReattachFixture(t, false)
	fixture.pinManifest("1.13.19")
	raw := []byte("{\n  // residual remains exact\n  \"route_mode\": \"reject\",\n  \"unknown\": {\"keep\": true}\n}\n")
	request := ManualReplaceRequest{
		ExpectedHead: fixture.base.Revision.ID, CoreVersion: "1.13.19",
		CoreArtifactID: "core-reattach", Raw: raw,
	}

	preview, err := fixture.application.PreviewManualReplace(fixture.ctx, request)
	if err != nil {
		fixture.t.Fatal(err)
	}
	if !preview.Reverse.Available || !preview.Reverse.CanonicalChanged ||
		preview.Reverse.Capability == nil ||
		preview.Reverse.Capability.ManifestSHA256 != fixture.manifestDigest ||
		len(preview.Reverse.ResidualPaths) != 1 || preview.Reverse.ResidualPaths[0] != "/unknown/keep" {
		fixture.t.Fatalf("manual replace preview = %+v", preview)
	}
	var proposed map[string]any
	if err := json.Unmarshal(preview.Reverse.ProposedCanonical, &proposed); err != nil {
		fixture.t.Fatal(err)
	}
	if proposed["global"].(map[string]any)["mode"] != "block" {
		fixture.t.Fatalf("proposed canonical = %s", preview.Reverse.ProposedCanonical)
	}

	saved, err := fixture.application.ReplaceManualJSON(fixture.ctx, request)
	if err != nil {
		fixture.t.Fatal(err)
	}
	if saved.NoChange || saved.Revision.ParentID != fixture.base.Revision.ID ||
		!saved.Preview.Reverse.Available || !bytes.Equal([]byte(saved.Artifact.Raw), raw) {
		fixture.t.Fatalf("manual save = %+v", saved)
	}
	var stored map[string]any
	if err := json.Unmarshal(saved.Revision.Document, &stored); err != nil {
		fixture.t.Fatal(err)
	}
	global := stored["global"].(map[string]any)
	if global["mode"] != "block" || global["dns"] != "base" {
		fixture.t.Fatalf("stored canonical = %s", saved.Revision.Document)
	}
	if bytes.Contains(saved.Revision.Document, []byte("unknown")) {
		fixture.t.Fatalf("residual manual path leaked into canonical: %s", saved.Revision.Document)
	}
}

func TestManualReplacePreviewFallsBackWithoutPinnedCapability(t *testing.T) {
	fixture := newManualReattachFixture(t, false)
	preview, err := fixture.application.PreviewManualReplace(fixture.ctx, ManualReplaceRequest{
		ExpectedHead: fixture.base.Revision.ID, CoreVersion: "1.13.19",
		CoreArtifactID: "core-reattach", Raw: []byte(`{"unknown":{"keep":true}}`),
	})
	if err != nil {
		fixture.t.Fatal(err)
	}
	if preview.Reverse.Available || preview.Reverse.ReasonCode != "capability_pin_unavailable" ||
		preview.Reverse.CanonicalChanged || len(preview.Reverse.ResidualPaths) != 1 ||
		preview.Reverse.ResidualPaths[0] != "/unknown/keep" ||
		!bytes.Equal(preview.Reverse.ProposedCanonical, fixture.base.Revision.Document) {
		fixture.t.Fatalf("manual fallback preview = %+v", preview)
	}
}

func TestManualReplaceFallsBackWhenReverseProjectionCapabilityChangesBeforeCommit(t *testing.T) {
	tests := []struct {
		name       string
		mutate     func(*manualReattachFixture)
		wantReason string
	}{
		{
			name: "pin moved",
			mutate: func(fixture *manualReattachFixture) {
				commit := strings.Repeat("a", 40)
				source, digest := manualReattachGeneration(fixture.t, commit, "1.13.19")
				if _, err := fixture.application.RefreshCapabilityGeneration(fixture.ctx, source); err != nil {
					fixture.t.Fatal(err)
				}
				if _, err := fixture.application.UpgradeCapability(fixture.ctx, CapabilityUpgradeRequest{
					ExactCoreVersion: "1.13.19", CommitSHA: commit, ManifestSHA256: digest,
				}); err != nil {
					fixture.t.Fatal(err)
				}
			},
			wantReason: "capability_pin_changed",
		},
		{
			name: "manifest quarantined",
			mutate: func(fixture *manualReattachFixture) {
				if _, err := fixture.database.UpsertCapabilityQuarantine(fixture.ctx, store.CapabilityQuarantine{
					ManifestSHA256: fixture.manifestDigest,
					ReasonCode:     "reverse_projection_failed",
					Diagnostics:    json.RawMessage(`{"fixture":"manual-save"}`),
				}); err != nil {
					fixture.t.Fatal(err)
				}
			},
			wantReason: "capability_quarantined",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newManualReattachFixture(t, false)
			fixture.pinManifest("1.13.19")
			raw := []byte("{\n  // exact bytes survive fallback\n  \"route_mode\": \"reject\",\n  \"unknown\": {\"keep\": true}\n}\n")
			request := ManualReplaceRequest{
				ExpectedHead: fixture.base.Revision.ID, CoreVersion: "1.13.19",
				CoreArtifactID: "core-reattach", Raw: raw,
			}
			prepared, err := fixture.application.prepareManualReplace(fixture.ctx, request)
			if err != nil {
				t.Fatal(err)
			}
			if !prepared.preview.Reverse.Available || !prepared.preview.Reverse.CanonicalChanged {
				t.Fatalf("pre-race reverse projection = %+v", prepared.preview.Reverse)
			}

			test.mutate(fixture)
			saved, err := fixture.application.commitPreparedManualReplace(fixture.ctx, prepared)
			if err != nil {
				t.Fatal(err)
			}
			if !saved.NoChange || saved.Revision.ID != fixture.base.Revision.ID ||
				saved.Preview.Reverse.Available || saved.Preview.Reverse.CanonicalChanged ||
				saved.Preview.Reverse.Capability != nil || saved.Preview.Reverse.ReasonCode != test.wantReason ||
				!bytes.Equal(saved.Preview.Reverse.ProposedCanonical, fixture.base.Revision.Document) ||
				!bytes.Equal([]byte(saved.Artifact.Raw), raw) ||
				saved.Artifact.CanonicalRevisionID != fixture.base.Revision.ID ||
				saved.Task.CanonicalRevisionID != fixture.base.Revision.ID ||
				saved.Task.StartupArtifactID != saved.Artifact.ID {
				t.Fatalf("fallback manual save = %+v", saved)
			}
			wantResidual := []string{"/route_mode", "/unknown/keep"}
			if !slices.Equal(saved.Preview.Reverse.ResidualPaths, wantResidual) ||
				string(saved.Preview.Reverse.OwnedPartial) != `{}` {
				t.Fatalf("fallback ownership = %+v", saved.Preview.Reverse)
			}
			var diagnostics []map[string]any
			if err := json.Unmarshal(saved.Artifact.Diagnostics, &diagnostics); err != nil || len(diagnostics) != 1 ||
				diagnostics[0]["reverse_available"] != false ||
				diagnostics[0]["reverse_reason_code"] != test.wantReason ||
				diagnostics[0]["canonical_changed"] != nil {
				t.Fatalf("fallback diagnostics = %#v, %v", diagnostics, err)
			}
			revisions, err := fixture.application.ListCanonicalRevisions(fixture.ctx, 0, 10)
			if err != nil || len(revisions.Items) != 1 || revisions.Items[0].ID != fixture.base.Revision.ID {
				t.Fatalf("canonical revisions after fallback = %+v, %v", revisions, err)
			}
		})
	}
}

type manualReattachFixture struct {
	t              *testing.T
	ctx            context.Context
	database       *store.Store
	application    *Application
	base           CanonicalSave
	manual         ManualSave
	manifestDigest string
	manifestCommit string
}

func newManualReattachFixture(t *testing.T, pin bool) *manualReattachFixture {
	t.Helper()
	ctx := context.Background()
	database, err := store.Open(ctx, filepath.Join(t.TempDir(), "panel.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	app := newApplication(database)
	now := time.Date(2026, time.August, 26, 12, 0, 0, 0, time.UTC)
	app.now = func() time.Time { return now }
	core := applicationTestCore("core-reattach", "1.13.19", '7', '8', now)
	if _, err := database.UpsertCoreArtifact(ctx, core); err != nil {
		t.Fatal(err)
	}
	base, err := app.ReplaceCanonical(ctx, "", []byte(`{
		"schema_version":1,
		"global":{"mode":"direct","dns":"base"},
		"nodes":[],"rules":[],"subscription":{}
	}`))
	if err != nil {
		t.Fatal(err)
	}
	manual, err := app.ReplaceManualJSON(ctx, ManualReplaceRequest{
		ExpectedHead: base.Revision.ID, CoreVersion: core.ExactVersion, CoreArtifactID: core.ID,
		Raw: []byte("{\n  // exact residual stays here\n  \"route_mode\": \"reject\",\n  \"unknown\": {\"keep\": true}\n}\n"),
	})
	if err != nil {
		t.Fatal(err)
	}
	fixture := &manualReattachFixture{
		t: t, ctx: ctx, database: database, application: app, base: base, manual: manual,
		manifestCommit: strings.Repeat("9", 40),
	}
	if pin {
		fixture.pinManifest(core.ExactVersion)
	}
	return fixture
}

func (fixture *manualReattachFixture) replaceCanonical(raw string) CanonicalSave {
	fixture.t.Helper()
	head, err := fixture.application.CanonicalHead(fixture.ctx)
	if err != nil || head == nil {
		fixture.t.Fatalf("read head = %+v, %v", head, err)
	}
	saved, err := fixture.application.ReplaceCanonical(fixture.ctx, head.ID, []byte(raw))
	if err != nil {
		fixture.t.Fatal(err)
	}
	return saved
}

func (fixture *manualReattachFixture) pinManifest(exactVersion string) {
	fixture.t.Helper()
	source, digest := manualReattachGeneration(fixture.t, fixture.manifestCommit, exactVersion)
	if _, err := fixture.application.RefreshCapabilityGeneration(fixture.ctx, source); err != nil {
		fixture.t.Fatal(err)
	}
	if _, err := fixture.application.UpgradeCapability(fixture.ctx, CapabilityUpgradeRequest{
		ExactCoreVersion: exactVersion,
		CommitSHA:        fixture.manifestCommit,
		ManifestSHA256:   digest,
	}); err != nil {
		fixture.t.Fatal(err)
	}
	fixture.manifestDigest = digest
}

func manualReattachGeneration(t *testing.T, commit, exactVersion string) ([]byte, string) {
	t.Helper()
	version, err := coreartifact.ParseExactVersion(exactVersion)
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := capability.NewManifest(capability.ManifestSpec{
		SchemaVersion: capability.ManifestSchemaVersion,
		CoreVersion:   version,
		SupportLevel:  capability.SupportNativeStructured,
		SemanticFacts: []capability.SemanticFact{{
			ID: "global.mode", CanonicalPath: "/global/mode",
			Classification: capability.CoverageSupported, OwnedPaths: []string{"/route_mode"},
		}},
		Transforms: []capability.Transform{{
			ID: "global.mode.enum", FactID: "global.mode", Primitive: capability.PrimitiveEnum,
			From: []string{"/global/mode"}, To: []string{"/route_mode"},
			Enum: map[string]string{"direct": "direct", "block": "reject"},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	canonicalManifest, err := manifest.CanonicalJSON()
	if err != nil {
		t.Fatal(err)
	}
	digest, err := manifest.Digest()
	if err != nil {
		t.Fatal(err)
	}
	envelope := struct {
		SchemaVersion int    `json:"schema_version"`
		Repository    string `json:"repository"`
		CommitSHA     string `json:"commit_sha"`
		ManifestCount int    `json:"manifest_count"`
		Manifests     []struct {
			Path           string          `json:"path"`
			ManifestSHA256 string          `json:"manifest_sha256"`
			Manifest       json.RawMessage `json:"manifest"`
		} `json:"manifests"`
	}{
		SchemaVersion: capability.GenerationSchemaVersion,
		Repository:    capability.ManifestRepository, CommitSHA: commit, ManifestCount: 1,
	}
	envelope.Manifests = append(envelope.Manifests, struct {
		Path           string          `json:"path"`
		ManifestSHA256 string          `json:"manifest_sha256"`
		Manifest       json.RawMessage `json:"manifest"`
	}{
		Path:           "capabilities/" + exactVersion + ".json",
		ManifestSHA256: digest.String(), Manifest: canonicalManifest,
	})
	encoded, err := json.Marshal(envelope)
	if err != nil {
		t.Fatal(err)
	}
	return encoded, digest.String()
}
