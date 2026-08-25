// SPDX-License-Identifier: GPL-3.0-or-later

package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestSaveCanonicalManualArtifactAndTaskCommitsAtomicSave(t *testing.T) {
	ctx := testContext(t)
	database := openTestStore(t, ctx)
	now := time.Date(2026, time.August, 26, 14, 0, 0, 0, time.UTC)
	core := testCoreArtifact("core-manual", 901, 'a', "amd64", now)
	if _, err := database.UpsertCoreArtifact(ctx, core); err != nil {
		t.Fatal(err)
	}

	rawConfig := []byte("{\n  // preserved byte-for-byte\n  \"log\": {},\n}\n")
	result, err := database.SaveCanonicalManualArtifactAndTask(
		ctx,
		manualStartupSaveInput("", "first", json.RawMessage(`{"value":1}`), rawConfig, now),
	)
	if err != nil {
		t.Fatalf("SaveCanonicalManualArtifactAndTask() error = %v", err)
	}
	if result.NoChange || result.Revision.ID != "revision-first" || result.Revision.Sequence != 1 {
		t.Fatalf("committed revision = %+v, no_change=%t", result.Revision, result.NoChange)
	}
	wantDigest := sha256.Sum256(rawConfig)
	if result.StartupArtifact.ID != "startup-first" ||
		result.StartupArtifact.CanonicalRevisionID != result.Revision.ID ||
		result.StartupArtifact.Kind != StartupArtifactManual ||
		result.StartupArtifact.State != StartupArtifactPending ||
		result.StartupArtifact.ConfigSHA256 != hex.EncodeToString(wantDigest[:]) ||
		string(result.StartupArtifact.ConfigBytes) != string(rawConfig) {
		t.Fatalf("committed startup artifact = %+v", result.StartupArtifact)
	}
	if result.CheckTask.ID != "task-first" ||
		result.CheckTask.Lane != TaskLaneMaintenance ||
		result.CheckTask.Status != TaskStatusQueued ||
		result.CheckTask.CanonicalRevisionID != result.Revision.ID ||
		result.CheckTask.StartupArtifactID != result.StartupArtifact.ID {
		t.Fatalf("committed check task = %+v", result.CheckTask)
	}

	rawConfig[0] = 'x'
	result.StartupArtifact.ConfigBytes[1] = 'x'
	stored, err := database.GetStartupArtifact(ctx, "startup-first")
	if err != nil || string(stored.ConfigBytes) != "{\n  // preserved byte-for-byte\n  \"log\": {},\n}\n" {
		t.Fatalf("stored exact config bytes = %q, error = %v", stored.ConfigBytes, err)
	}
	assertManualSaveCounts(t, ctx, database, "revision-first", 1, 1, 1)
}

func TestSaveCanonicalManualArtifactAndTaskRejectsStaleHeadWithoutWrites(t *testing.T) {
	ctx := testContext(t)
	database := openTestStore(t, ctx)
	now := time.Date(2026, time.August, 26, 15, 0, 0, 0, time.UTC)
	core := testCoreArtifact("core-manual", 902, 'b', "amd64", now)
	if _, err := database.UpsertCoreArtifact(ctx, core); err != nil {
		t.Fatal(err)
	}
	first, err := database.SaveCanonicalManualArtifactAndTask(
		ctx,
		manualStartupSaveInput("", "first", json.RawMessage(`{"value":1}`), []byte(`{"log":{}}`), now),
	)
	if err != nil {
		t.Fatal(err)
	}

	_, err = database.SaveCanonicalManualArtifactAndTask(
		ctx,
		manualStartupSaveInput("stale-head", "conflict", json.RawMessage(`{"value":2}`), []byte(`{}`), now.Add(time.Minute)),
	)
	if !errors.Is(err, ErrRevisionConflict) {
		t.Fatalf("stale save error = %v, want ErrRevisionConflict", err)
	}
	var conflict *RevisionConflictError
	if !errors.As(err, &conflict) || conflict.ActualHead != first.Revision.ID {
		t.Fatalf("stale save conflict = %#v, want actual head %q", conflict, first.Revision.ID)
	}
	assertManualSaveCounts(t, ctx, database, first.Revision.ID, 1, 1, 1)
	if _, err := database.GetStartupArtifact(ctx, "startup-conflict"); !errors.Is(err, ErrStartupArtifactNotFound) {
		t.Fatalf("rolled-back conflict artifact lookup error = %v", err)
	}
	if _, err := database.GetTask(ctx, "task-conflict"); !errors.Is(err, ErrTaskNotFound) {
		t.Fatalf("rolled-back conflict task lookup error = %v", err)
	}
}

func TestSaveCanonicalManualArtifactAndTaskRollsBackEveryStep(t *testing.T) {
	ctx := testContext(t)
	database := openTestStore(t, ctx)
	now := time.Date(2026, time.August, 26, 16, 0, 0, 0, time.UTC)
	core := testCoreArtifact("core-manual", 903, 'c', "amd64", now)
	if _, err := database.UpsertCoreArtifact(ctx, core); err != nil {
		t.Fatal(err)
	}
	first, err := database.SaveCanonicalManualArtifactAndTask(
		ctx,
		manualStartupSaveInput("", "first", json.RawMessage(`{"value":1}`), []byte(`{"log":{}}`), now),
	)
	if err != nil {
		t.Fatal(err)
	}
	ready, err := database.CompleteStartupArtifactCheck(
		ctx,
		first.StartupArtifact.ID,
		true,
		nil,
		now.Add(time.Second),
	)
	if err != nil || ready.State != StartupArtifactReady {
		t.Fatalf("prepare old ready candidate = %+v, %v", ready, err)
	}

	tests := []struct {
		name   string
		mutate func(*SaveCanonicalManualArtifactInput)
	}{
		{
			name: "reference validation after revision insert",
			mutate: func(input *SaveCanonicalManualArtifactInput) {
				input.Artifact.ExactCoreVersion = "1.12.0"
			},
		},
		{
			name: "task insert after artifact insert",
			mutate: func(input *SaveCanonicalManualArtifactInput) {
				input.CheckTask.ID = first.CheckTask.ID
			},
		},
	}
	for index, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			input := manualStartupSaveInput(
				first.Revision.ID,
				"rollback-"+string(rune('a'+index)),
				json.RawMessage(`{"value":2}`),
				[]byte(`{"log":{"level":"debug"}}`),
				now.Add(time.Duration(index+2)*time.Second),
			)
			test.mutate(&input)
			if _, err := database.SaveCanonicalManualArtifactAndTask(ctx, input); err == nil {
				t.Fatal("save unexpectedly succeeded")
			}

			assertManualSaveCounts(t, ctx, database, first.Revision.ID, 1, 1, 1)
			storedOld, err := database.GetStartupArtifact(ctx, first.StartupArtifact.ID)
			if err != nil || storedOld.State != StartupArtifactReady {
				t.Fatalf("old candidate after rollback = %+v, %v; want ready", storedOld, err)
			}
			if _, err := database.GetStartupArtifact(ctx, input.Artifact.ID); !errors.Is(err, ErrStartupArtifactNotFound) {
				t.Fatalf("rolled-back artifact lookup error = %v", err)
			}
			if _, err := database.GetCanonicalRevision(ctx, input.Revision.ID); !errors.Is(err, ErrCanonicalRevisionNotFound) {
				t.Fatalf("rolled-back revision lookup error = %v", err)
			}
		})
	}
}

func TestSaveCanonicalManualArtifactAndTaskReusesUnchangedHeadAndStalesOldCandidate(t *testing.T) {
	ctx := testContext(t)
	database := openTestStore(t, ctx)
	now := time.Date(2026, time.August, 26, 17, 0, 0, 0, time.UTC)
	core := testCoreArtifact("core-manual", 904, 'd', "amd64", now)
	if _, err := database.UpsertCoreArtifact(ctx, core); err != nil {
		t.Fatal(err)
	}
	first, err := database.SaveCanonicalManualArtifactAndTask(
		ctx,
		manualStartupSaveInput("", "first", json.RawMessage(`{"value":1}`), []byte(`{"log":{}}`), now),
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.CompleteStartupArtifactCheck(
		ctx,
		first.StartupArtifact.ID,
		true,
		nil,
		now.Add(time.Second),
	); err != nil {
		t.Fatal(err)
	}

	second, err := database.SaveCanonicalManualArtifactAndTask(
		ctx,
		manualStartupSaveInput(
			first.Revision.ID,
			"second",
			json.RawMessage(`{"value":1}`),
			[]byte("{\n  \"log\": {\"level\": \"debug\"}\n}\n"),
			now.Add(2*time.Second),
		),
	)
	if err != nil {
		t.Fatal(err)
	}
	if !second.NoChange || second.Revision.ID != first.Revision.ID || second.Revision.Sequence != 1 {
		t.Fatalf("unchanged save revision = %+v, no_change=%t", second.Revision, second.NoChange)
	}
	if second.StartupArtifact.CanonicalRevisionID != first.Revision.ID ||
		second.CheckTask.CanonicalRevisionID != first.Revision.ID ||
		second.CheckTask.StartupArtifactID != second.StartupArtifact.ID {
		t.Fatalf("unchanged save bindings: artifact=%+v task=%+v", second.StartupArtifact, second.CheckTask)
	}
	assertManualSaveCounts(t, ctx, database, first.Revision.ID, 1, 2, 2)

	old, err := database.GetStartupArtifact(ctx, first.StartupArtifact.ID)
	if err != nil || old.State != StartupArtifactStale {
		t.Fatalf("superseded manual candidate = %+v, %v; want stale", old, err)
	}
	late, err := database.CompleteStartupArtifactCheck(
		ctx,
		first.StartupArtifact.ID,
		true,
		json.RawMessage(`[{"code":"late"}]`),
		now.Add(3*time.Second),
	)
	if err != nil || late.State != StartupArtifactStale || late.CheckedAt == nil || !late.CheckedAt.Equal(old.CheckedAt.UTC()) {
		t.Fatalf("late completion = %+v, %v; want unchanged stale candidate", late, err)
	}
}

func TestSaveCanonicalManualArtifactAndTaskRechecksReverseProjectionCapability(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*testing.T, context.Context, *Store, storeManualReattachFixture)
		wantErr error
	}{
		{
			name: "pin moved",
			mutate: func(t *testing.T, ctx context.Context, database *Store, fixture storeManualReattachFixture) {
				commit := strings.Repeat("c", 40)
				generation, digest := storeManualReattachGeneration(t, commit)
				if _, err := database.SaveCapabilityGeneration(ctx, generation, fixture.now.Add(time.Second)); err != nil {
					t.Fatal(err)
				}
				if _, err := database.PinCapabilityGenerationManifest(
					ctx, commit, fixture.evidence.ExactCoreVersion, digest, fixture.now.Add(2*time.Second),
				); err != nil {
					t.Fatal(err)
				}
			},
			wantErr: ErrManualProjectionEvidenceStale,
		},
		{
			name: "manifest quarantined",
			mutate: func(t *testing.T, ctx context.Context, database *Store, fixture storeManualReattachFixture) {
				if _, err := database.UpsertCapabilityQuarantine(ctx, CapabilityQuarantine{
					ManifestSHA256: fixture.evidence.CapabilitySHA256,
					ReasonCode:     "reverse_projection_failed",
					Diagnostics:    json.RawMessage(`{"fixture":"manual-save"}`),
					QuarantinedAt:  fixture.now.Add(time.Second),
				}); err != nil {
					t.Fatal(err)
				}
			},
			wantErr: ErrCapabilityManifestQuarantined,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx := testContext(t)
			database := openTestStore(t, ctx)
			fixture := prepareStoreManualReattach(t, ctx, database, "projection-"+strings.ReplaceAll(test.name, " ", "-"), false)
			input := manualStartupSaveInput(
				fixture.head.ID,
				"projected-"+strings.ReplaceAll(test.name, " ", "-"),
				json.RawMessage(`{"schema_version":1,"global":{"mode":"block"},"nodes":[],"rules":[],"subscription":{}}`),
				[]byte("{\n  // exact manual bytes\n  \"route_mode\": \"reject\"\n}\n"),
				fixture.now.Add(3*time.Second),
			)
			input.Artifact.ExactCoreVersion = fixture.evidence.ExactCoreVersion
			input.Artifact.CoreArtifactID = fixture.evidence.CoreArtifactID
			input.ProjectionEvidence = &ManualProjectionEvidence{
				ExactCoreVersion: fixture.evidence.ExactCoreVersion,
				Repository:       fixture.evidence.CapabilityRepository,
				CommitSHA:        fixture.evidence.CapabilityCommit,
				ManifestSHA256:   fixture.evidence.CapabilitySHA256,
				SupportLevel:     fixture.evidence.CapabilitySupport,
			}

			test.mutate(t, ctx, database, fixture)
			_, err := database.SaveCanonicalManualArtifactAndTask(ctx, input)
			if !errors.Is(err, test.wantErr) {
				t.Fatalf("SaveCanonicalManualArtifactAndTask() error = %v, want %v", err, test.wantErr)
			}
			assertManualSaveCounts(t, ctx, database, fixture.head.ID, 1, 1, 1)
			if _, err := database.GetCanonicalRevision(ctx, input.Revision.ID); !errors.Is(err, ErrCanonicalRevisionNotFound) {
				t.Fatalf("rejected projection revision lookup = %v", err)
			}
			if _, err := database.GetStartupArtifact(ctx, input.Artifact.ID); !errors.Is(err, ErrStartupArtifactNotFound) {
				t.Fatalf("rejected projection artifact lookup = %v", err)
			}
			if _, err := database.GetTask(ctx, input.CheckTask.ID); !errors.Is(err, ErrTaskNotFound) {
				t.Fatalf("rejected projection task lookup = %v", err)
			}
			storedSource, err := database.GetStartupArtifact(ctx, fixture.source.ID)
			if err != nil || storedSource.State != fixture.source.State {
				t.Fatalf("source after rejected projection = %+v, %v; want state %s", storedSource, err, fixture.source.State)
			}
		})
	}
}

func manualStartupSaveInput(
	expectedHead string,
	suffix string,
	document json.RawMessage,
	config []byte,
	createdAt time.Time,
) SaveCanonicalManualArtifactInput {
	return SaveCanonicalManualArtifactInput{
		ExpectedHead: expectedHead,
		Revision: NewCanonicalRevision{
			ID:            "revision-" + suffix,
			SchemaVersion: 1,
			Document:      document,
			CommandID:     "command-" + suffix,
			CreatedAt:     createdAt,
		},
		Artifact: NewManualStartupArtifact{
			ID:               "startup-" + suffix,
			ExactCoreVersion: "1.13.19",
			RendererVersion:  "manual-v1",
			CoreArtifactID:   "core-manual",
			ConfigBytes:      config,
			CreatedAt:        createdAt,
		},
		CheckTask: NewTask{
			ID:             "task-" + suffix,
			IdempotencyKey: "startup-check:" + suffix,
			Lane:           TaskLaneMaintenance,
			Kind:           "startup-check",
			Payload:        json.RawMessage(`{"startup_artifact_id":"startup-` + suffix + `"}`),
			CreatedAt:      createdAt,
		},
	}
}

func assertManualSaveCounts(
	t *testing.T,
	ctx context.Context,
	database *Store,
	wantHead string,
	wantRevisions int,
	wantStartupArtifacts int,
	wantTasks int,
) {
	t.Helper()
	head, err := database.Head(ctx)
	if err != nil || head == nil || head.ID != wantHead {
		t.Fatalf("canonical head = %+v, %v; want %q", head, err, wantHead)
	}
	for table, want := range map[string]int{
		"canonical_revisions": wantRevisions,
		"startup_artifacts":   wantStartupArtifacts,
		"tasks":               wantTasks,
	} {
		var count int
		if err := database.db.QueryRowContext(ctx, `SELECT count(*) FROM `+table).Scan(&count); err != nil {
			t.Fatalf("count %s: %v", table, err)
		}
		if count != want {
			t.Fatalf("%s count = %d, want %d", table, count, want)
		}
	}
}
