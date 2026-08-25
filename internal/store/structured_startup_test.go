// SPDX-License-Identifier: GPL-3.0-or-later

package store

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/rehuony/sing-box-panel/internal/capability"
)

func TestCreateStructuredStartupArtifactAndCheckTaskIsAtomic(t *testing.T) {
	ctx := testContext(t)
	database := openTestStore(t, ctx)
	now := time.Date(2026, time.August, 26, 14, 0, 0, 0, time.UTC)
	core := testCoreArtifact("core-structured", 801, 'f', "amd64", now)
	if _, err := database.UpsertCoreArtifact(ctx, core); err != nil {
		t.Fatal(err)
	}
	revision, err := database.SaveCanonicalRevisionAndTask(ctx, "", NewCanonicalRevision{
		ID: "revision-structured", SchemaVersion: 1, Document: json.RawMessage(`{}`),
		CommandID: "command-structured", CreatedAt: now,
	}, NewTask{ID: "revision-task-structured", Lane: TaskLaneMaintenance, Kind: "canonical-saved"})
	if err != nil {
		t.Fatal(err)
	}
	commit := strings.Repeat("a", 40)
	generation, digest := capabilityGenerationFixture(t, commit, []capabilityGenerationManifestFixture{{
		version: core.ExactVersion, support: capability.SupportNativeStructured,
	}})
	if _, err := database.SaveCapabilityGeneration(ctx, generation, now); err != nil {
		t.Fatal(err)
	}
	if _, err := database.PinCapabilityGenerationManifest(ctx, commit, core.ExactVersion, digest, now); err != nil {
		t.Fatal(err)
	}
	artifact := StartupArtifact{
		ID: "startup-structured", Kind: StartupArtifactStructured,
		CanonicalRevisionID: revision.ID, ExactCoreVersion: core.ExactVersion,
		CapabilityCommit: commit, CapabilityDigest: digest,
		RendererVersion: "structured-v1", CoreArtifactID: core.ID,
		ConfigBytes: []byte(`{"outbounds":[]}`), Diagnostics: json.RawMessage(`[]`), CreatedAt: now,
	}
	evidence := StructuredStartupEvidence{
		ExpectedCanonicalHeadID: revision.ID, CapabilityRepository: capability.ManifestRepository,
		CapabilityCommit: commit, CapabilityDigest: digest, CapabilitySupport: capability.SupportNativeStructured,
	}
	result, err := database.CreateStartupArtifactAndCheckTask(ctx, artifact, NewTask{
		ID: "check-structured", IdempotencyKey: "startup-check:" + artifact.ID,
		Lane: TaskLaneMaintenance, Kind: "startup-check",
		Payload: json.RawMessage(`{"startup_artifact_id":"startup-structured"}`), CreatedAt: now,
	}, evidence)
	if err != nil {
		t.Fatal(err)
	}
	if result.Artifact.State != StartupArtifactPending || result.Task.StartupArtifactID != artifact.ID || result.Task.Status != TaskStatusQueued {
		t.Fatalf("atomic render result = %+v", result)
	}

	failedArtifact := artifact
	failedArtifact.ID = "startup-rolled-back"
	if _, err := database.CreateStartupArtifactAndCheckTask(ctx, failedArtifact, NewTask{
		ID: "check-structured", Lane: TaskLaneMaintenance, Kind: "startup-check",
		Payload: json.RawMessage(`{}`), CreatedAt: now.Add(time.Second),
	}, evidence); err == nil {
		t.Fatal("duplicate task unexpectedly succeeded")
	}
	if _, err := database.GetStartupArtifact(ctx, failedArtifact.ID); !errors.Is(err, ErrStartupArtifactNotFound) {
		t.Fatalf("failed transaction left startup artifact: %v", err)
	}
}

func TestCreateStructuredStartupArtifactRejectsStaleEvidenceWithoutWrites(t *testing.T) {
	mutations := []struct {
		name   string
		mutate func(*testing.T, *Store, string, string, string, time.Time)
	}{
		{
			name: "canonical head moved",
			mutate: func(t *testing.T, database *Store, revisionID, _, _ string, now time.Time) {
				t.Helper()
				if _, err := database.SaveCanonicalRevisionAndTask(testContext(t), revisionID, NewCanonicalRevision{
					ID: "revision-new-head", SchemaVersion: 1, Document: json.RawMessage(`{}`),
					CommandID: "command-new-head", CreatedAt: now.Add(time.Second),
				}, NewTask{ID: "revision-task-new-head", Lane: TaskLaneMaintenance, Kind: "canonical-saved"}); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "capability pin moved",
			mutate: func(t *testing.T, database *Store, _, version, _ string, now time.Time) {
				t.Helper()
				commit := strings.Repeat("c", 40)
				source, digest := capabilityGenerationFixture(t, commit, []capabilityGenerationManifestFixture{{
					version: version, support: capability.SupportNativeStructured,
				}})
				if _, err := database.SaveCapabilityGeneration(testContext(t), source, now.Add(time.Second)); err != nil {
					t.Fatal(err)
				}
				if _, err := database.PinCapabilityGenerationManifest(testContext(t), commit, version, digest, now.Add(2*time.Second)); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "capability quarantined",
			mutate: func(t *testing.T, database *Store, _, _, digest string, now time.Time) {
				t.Helper()
				if _, err := database.UpsertCapabilityQuarantine(testContext(t), CapabilityQuarantine{
					ManifestSHA256: digest, ReasonCode: "projection_failed",
					Diagnostics: json.RawMessage(`{"fixture":"stale-render"}`), QuarantinedAt: now.Add(time.Second),
				}); err != nil {
					t.Fatal(err)
				}
			},
		},
	}

	for _, test := range mutations {
		t.Run(test.name, func(t *testing.T) {
			ctx := testContext(t)
			database := openTestStore(t, ctx)
			now := time.Date(2026, time.August, 26, 14, 30, 0, 0, time.UTC)
			core := testCoreArtifact("core-stale-evidence", 811, 'e', "amd64", now)
			if _, err := database.UpsertCoreArtifact(ctx, core); err != nil {
				t.Fatal(err)
			}
			revision, err := database.SaveCanonicalRevisionAndTask(ctx, "", NewCanonicalRevision{
				ID: "revision-stale-evidence", SchemaVersion: 1, Document: json.RawMessage(`{}`),
				CommandID: "command-stale-evidence", CreatedAt: now,
			}, NewTask{ID: "revision-task-stale-evidence", Lane: TaskLaneMaintenance, Kind: "canonical-saved"})
			if err != nil {
				t.Fatal(err)
			}
			commit := strings.Repeat("a", 40)
			source, digest := capabilityGenerationFixture(t, commit, []capabilityGenerationManifestFixture{{
				version: core.ExactVersion, support: capability.SupportNativeStructured,
			}})
			if _, err := database.SaveCapabilityGeneration(ctx, source, now); err != nil {
				t.Fatal(err)
			}
			if _, err := database.PinCapabilityGenerationManifest(ctx, commit, core.ExactVersion, digest, now); err != nil {
				t.Fatal(err)
			}

			artifact := StartupArtifact{
				ID: "startup-stale-evidence", Kind: StartupArtifactStructured,
				CanonicalRevisionID: revision.ID, ExactCoreVersion: core.ExactVersion,
				CapabilityCommit: commit, CapabilityDigest: digest,
				RendererVersion: "structured-v1", CoreArtifactID: core.ID,
				ConfigBytes: []byte(`{"route":{"final":"direct"}}`), Diagnostics: json.RawMessage(`[]`), CreatedAt: now,
			}
			evidence := StructuredStartupEvidence{
				ExpectedCanonicalHeadID: revision.ID, CapabilityRepository: capability.ManifestRepository,
				CapabilityCommit: commit, CapabilityDigest: digest, CapabilitySupport: capability.SupportNativeStructured,
			}
			test.mutate(t, database, revision.ID, core.ExactVersion, digest, now)
			_, err = database.CreateStartupArtifactAndCheckTask(ctx, artifact, NewTask{
				ID: "check-stale-evidence", Lane: TaskLaneMaintenance, Kind: "startup-check",
				Payload: json.RawMessage(`{"startup_artifact_id":"startup-stale-evidence"}`), CreatedAt: now,
			}, evidence)
			if !errors.Is(err, ErrStructuredStartupEvidenceStale) {
				t.Fatalf("stale evidence error = %v", err)
			}
			if _, err := database.GetStartupArtifact(ctx, artifact.ID); !errors.Is(err, ErrStartupArtifactNotFound) {
				t.Fatalf("rejected render left startup artifact: %v", err)
			}
			if _, err := database.GetTask(ctx, "check-stale-evidence"); !errors.Is(err, ErrTaskNotFound) {
				t.Fatalf("rejected render left check task: %v", err)
			}
		})
	}
}
