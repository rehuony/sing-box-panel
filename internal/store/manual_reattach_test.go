// SPDX-License-Identifier: GPL-3.0-or-later

package store

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/rehuony/sing-box-panel/internal/capability"
	"github.com/rehuony/sing-box-panel/internal/coreartifact"
)

func TestSaveReattachedManualArtifactForcesNewRevisionAndPreservesBytes(t *testing.T) {
	ctx := context.Background()
	database := openTestStore(t, ctx)
	fixture := prepareStoreManualReattach(t, ctx, database, "commit", false)
	input := fixture.input("success", fixture.head.Document, fixture.now.Add(time.Minute))

	saved, err := database.SaveReattachedManualArtifactAndTask(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	if saved.Revision.ID == fixture.head.ID || saved.Revision.ParentID != fixture.head.ID ||
		saved.Revision.Sequence != fixture.head.Sequence+1 {
		t.Fatalf("revision = %+v", saved.Revision)
	}
	if saved.StartupArtifact.CanonicalRevisionID != saved.Revision.ID ||
		saved.StartupArtifact.State != StartupArtifactPending ||
		string(saved.StartupArtifact.ConfigBytes) != string(fixture.source.ConfigBytes) ||
		saved.CheckTask.StartupArtifactID != saved.StartupArtifact.ID ||
		saved.CheckTask.CanonicalRevisionID != saved.Revision.ID {
		t.Fatalf("artifact/task = %+v / %+v", saved.StartupArtifact, saved.CheckTask)
	}
	source, err := database.GetStartupArtifact(ctx, fixture.source.ID)
	if err != nil || source.State != StartupArtifactStale {
		t.Fatalf("source = %+v, %v; want stale", source, err)
	}
}

func TestSaveReattachedManualArtifactRollsBackSourceAndHeadOnLateFailure(t *testing.T) {
	ctx := context.Background()
	database := openTestStore(t, ctx)
	fixture := prepareStoreManualReattach(t, ctx, database, "rollback", true)
	input := fixture.input("rollback", json.RawMessage(`{
		"schema_version":1,"global":{"mode":"block"},
		"nodes":[],"rules":[],"subscription":{}
	}`), fixture.now.Add(time.Minute))
	input.CheckTask.ID = fixture.sourceTask.ID

	if _, err := database.SaveReattachedManualArtifactAndTask(ctx, input); err == nil {
		t.Fatal("reattach with duplicate late task unexpectedly succeeded")
	}
	head, err := database.Head(ctx)
	if err != nil || head == nil || head.ID != fixture.head.ID {
		t.Fatalf("head after rollback = %+v, %v", head, err)
	}
	source, err := database.GetStartupArtifact(ctx, fixture.source.ID)
	if err != nil || source.State != StartupArtifactReady {
		t.Fatalf("source after rollback = %+v, %v; want ready", source, err)
	}
	if _, err := database.GetCanonicalRevision(ctx, input.Revision.ID); !errors.Is(err, ErrCanonicalRevisionNotFound) {
		t.Fatalf("rolled-back revision lookup = %v", err)
	}
	if _, err := database.GetStartupArtifact(ctx, input.Artifact.ID); !errors.Is(err, ErrStartupArtifactNotFound) {
		t.Fatalf("rolled-back startup lookup = %v", err)
	}
}

func TestSaveReattachedManualArtifactConcurrentCASHasOneWinner(t *testing.T) {
	ctx := context.Background()
	database := openTestStore(t, ctx)
	fixture := prepareStoreManualReattach(t, ctx, database, "race", false)
	inputs := []SaveReattachedManualArtifactInput{
		fixture.input("left", fixture.head.Document, fixture.now.Add(time.Minute)),
		fixture.input("right", fixture.head.Document, fixture.now.Add(2*time.Minute)),
	}

	results := make(chan error, len(inputs))
	var start sync.WaitGroup
	start.Add(1)
	for _, input := range inputs {
		input := input
		go func() {
			start.Wait()
			_, err := database.SaveReattachedManualArtifactAndTask(ctx, input)
			results <- err
		}()
	}
	start.Done()

	var succeeded, conflicted int
	for range inputs {
		err := <-results
		switch {
		case err == nil:
			succeeded++
		case errors.Is(err, ErrRevisionConflict):
			conflicted++
		default:
			t.Fatalf("unexpected concurrent error = %v", err)
		}
	}
	if succeeded != 1 || conflicted != 1 {
		t.Fatalf("concurrent results: succeeded=%d conflicted=%d", succeeded, conflicted)
	}
}

type storeManualReattachFixture struct {
	now        time.Time
	source     StartupArtifact
	sourceTask Task
	head       CanonicalRevision
	evidence   ManualReattachEvidence
}

func prepareStoreManualReattach(
	t *testing.T,
	ctx context.Context,
	database *Store,
	suffix string,
	ready bool,
) storeManualReattachFixture {
	t.Helper()
	now := time.Date(2026, time.August, 26, 13, 0, 0, 0, time.UTC)
	core := testCoreArtifact("core-reattach-"+suffix, int64(700+len(suffix)), '6', "amd64", now)
	if _, err := database.UpsertCoreArtifact(ctx, core); err != nil {
		t.Fatal(err)
	}
	baseDocument := json.RawMessage(`{
		"schema_version":1,"global":{"mode":"direct"},
		"nodes":[],"rules":[],"subscription":{}
	}`)
	sourceSave, err := database.SaveCanonicalManualArtifactAndTask(
		ctx,
		SaveCanonicalManualArtifactInput{
			ExpectedHead: "",
			Revision: NewCanonicalRevision{
				ID: "revision-source-" + suffix, SchemaVersion: 1, Document: baseDocument,
				CommandID: "command-source-" + suffix, CreatedAt: now,
			},
			Artifact: NewManualStartupArtifact{
				ID: "startup-source-" + suffix, ExactCoreVersion: "1.13.19",
				RendererVersion: "manual-json-v1", CoreArtifactID: core.ID,
				ConfigBytes: []byte("{\n  // exact\n  \"route_mode\": \"reject\",\n  \"unknown\": true\n}\n"),
				CreatedAt:   now,
			},
			CheckTask: NewTask{
				ID: "task-source-" + suffix, IdempotencyKey: "startup-check:source-" + suffix,
				Lane: TaskLaneMaintenance, Kind: "startup-check",
				Payload:   json.RawMessage(`{"startup_artifact_id":"startup-source-` + suffix + `"}`),
				CreatedAt: now,
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	source := sourceSave.StartupArtifact
	if ready {
		source, err = database.CompleteStartupArtifactCheck(ctx, source.ID, true, nil, now.Add(time.Second))
		if err != nil {
			t.Fatal(err)
		}
	}

	commit := strings.Repeat("b", 40)
	// The repository contents are not important to this store test; the
	// immutable reference and digest are.
	generation, digest := storeManualReattachGeneration(t, commit)
	if _, err := database.SaveCapabilityGeneration(ctx, generation, now); err != nil {
		t.Fatal(err)
	}
	pin, err := database.PinCapabilityGenerationManifest(ctx, commit, "1.13.19", digest, now)
	if err != nil {
		t.Fatal(err)
	}
	head, err := database.Head(ctx)
	if err != nil || head == nil {
		t.Fatalf("head = %+v, %v", head, err)
	}
	return storeManualReattachFixture{
		now: now, source: source, sourceTask: sourceSave.CheckTask, head: *head,
		evidence: ManualReattachEvidence{
			SourceArtifactID: source.ID, SourceConfigSHA256: source.ConfigSHA256,
			BaseRevisionID: source.CanonicalRevisionID, BaseRevisionSHA256: sourceSave.Revision.SHA256,
			ExpectedHead: head.ID, ExpectedHeadSHA256: head.SHA256,
			ExactCoreVersion: source.ExactCoreVersion, CoreArtifactID: source.CoreArtifactID,
			CapabilityRepository: pin.Repository, CapabilityCommit: pin.CommitSHA,
			CapabilitySHA256: pin.ManifestSHA256, CapabilitySupport: pin.SupportLevel,
		},
	}
}

func storeManualReattachGeneration(t *testing.T, commit string) ([]byte, string) {
	t.Helper()
	version, err := coreartifact.ParseExactVersion("1.13.19")
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
			ID: "global.mode.rename", FactID: "global.mode", Primitive: capability.PrimitiveRename,
			From: []string{"/global/mode"}, To: []string{"/route_mode"},
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
	value := struct {
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
	value.Manifests = append(value.Manifests, struct {
		Path           string          `json:"path"`
		ManifestSHA256 string          `json:"manifest_sha256"`
		Manifest       json.RawMessage `json:"manifest"`
	}{
		Path: "capabilities/1.13.19.json", ManifestSHA256: digest.String(), Manifest: manifestJSON,
	})
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return encoded, digest.String()
}

func (fixture storeManualReattachFixture) input(
	suffix string,
	document json.RawMessage,
	createdAt time.Time,
) SaveReattachedManualArtifactInput {
	startupID := "startup-reattach-" + suffix
	return SaveReattachedManualArtifactInput{
		Evidence: fixture.evidence,
		Revision: NewCanonicalRevision{
			ID: "revision-reattach-" + suffix, SchemaVersion: 1, Document: document,
			CommandID: "command-reattach-" + suffix, CreatedAt: createdAt,
		},
		Artifact: NewManualStartupArtifact{
			ID: startupID, ExactCoreVersion: fixture.source.ExactCoreVersion,
			RendererVersion: "manual-json-reattach-v1", CoreArtifactID: fixture.source.CoreArtifactID,
			ConfigBytes: fixture.source.ConfigBytes, ConfigSHA256: fixture.source.ConfigSHA256,
			Diagnostics: json.RawMessage(`[{"code":"manual_json_reattached"}]`), CreatedAt: createdAt,
		},
		CheckTask: NewTask{
			ID: "task-reattach-" + suffix, IdempotencyKey: "startup-check:" + startupID,
			Lane: TaskLaneMaintenance, Kind: "startup-check",
			Payload:   json.RawMessage(`{"startup_artifact_id":"` + startupID + `"}`),
			CreatedAt: createdAt,
		},
	}
}
