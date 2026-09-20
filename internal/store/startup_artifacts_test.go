// SPDX-License-Identifier: GPL-3.0-or-later

package store

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/rehuony/sing-box-panel/internal/configuration"
)

func TestStartupArtifactBindsRawConfigurationToRevisionAndExactBinary(t *testing.T) {
	ctx := context.Background()
	database := openTestStore(t, ctx)
	now := time.Date(2026, time.September, 1, 8, 0, 0, 0, time.UTC)
	revision, err := database.SaveCanonicalRevisionAndTask(ctx, "", NewCanonicalRevision{
		ID: "revision-raw", SchemaVersion: configuration.SchemaVersion,
		Document: json.RawMessage(`{"log":{"level":"warn"}}`), CommandID: "command-raw", CreatedAt: now,
	}, NewTask{ID: "task-raw", Lane: TaskLaneMaintenance, Kind: TaskKindCanonicalSaved, CreatedAt: now})
	if err != nil {
		t.Fatal(err)
	}
	core, err := database.UpsertCoreArtifact(ctx, CoreArtifact{
		ID: "core-raw", ExactVersion: "1.13.19", OperatingSystem: "linux", Architecture: "amd64", Variant: "plain",
		SourceKind: CoreArtifactSourceUserVerified, UserSource: "test", ArchiveSHA256: strings.Repeat("a", 64),
		BinarySHA256: strings.Repeat("b", 64), BinaryPath: "/tmp/sing-box", ReportedVersion: "1.13.19",
		FeatureFingerprint: json.RawMessage(`{"status":"not_reported"}`), CreatedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}

	startup, err := database.CreateStartupArtifact(ctx, StartupArtifact{
		ID: "startup-raw", CanonicalRevisionID: revision.ID, ExactCoreVersion: core.ExactVersion,
		CoreArtifactID: core.ID, ConfigBytes: []byte("{\n  \"log\": {\"level\": \"warn\"}\n}"), CreatedAt: now.Add(time.Second),
	})
	if err != nil {
		t.Fatalf("CreateStartupArtifact() error = %v", err)
	}
	if startup.ConfigSHA256 != revision.SHA256 || string(startup.ConfigBytes) != string(revision.Document) ||
		startup.State != StartupArtifactPending {
		t.Fatalf("startup = %+v config=%s revision=%s", startup, startup.ConfigBytes, revision.Document)
	}
	ready, err := database.CompleteStartupArtifactCheck(ctx, startup.ID, true, now.Add(2*time.Second))
	if err != nil || ready.State != StartupArtifactReady || ready.CheckedAt == nil {
		t.Fatalf("CompleteStartupArtifactCheck() = %+v, %v", ready, err)
	}

	_, err = database.CreateStartupArtifact(ctx, StartupArtifact{
		ID: "startup-mismatch", CanonicalRevisionID: revision.ID, ExactCoreVersion: core.ExactVersion,
		CoreArtifactID: core.ID, ConfigBytes: []byte(`{"log":{"level":"debug"}}`), CreatedAt: now.Add(3 * time.Second),
	})
	if err == nil || !strings.Contains(err.Error(), "does not match canonical revision") {
		t.Fatalf("mismatched config error = %v", err)
	}
	_, err = database.CreateStartupArtifact(ctx, StartupArtifact{
		ID: "startup-array", CanonicalRevisionID: revision.ID, ExactCoreVersion: core.ExactVersion,
		CoreArtifactID: core.ID, ConfigBytes: []byte(`[]`), CreatedAt: now.Add(4 * time.Second),
	})
	if !errors.Is(err, configuration.ErrInvalidDocument) {
		t.Fatalf("non-object config error = %v, want ErrInvalidDocument", err)
	}
}

func TestCompiledRawStartupRechecksHeadBeforeInsert(t *testing.T) {
	ctx := context.Background()
	database := openTestStore(t, ctx)
	now := time.Date(2026, time.September, 1, 9, 0, 0, 0, time.UTC)
	first, err := database.SaveCanonicalRevisionAndTask(ctx, "", NewCanonicalRevision{
		ID: "revision-first", SchemaVersion: configuration.SchemaVersion,
		Document: json.RawMessage(`{}`), CommandID: "command-first", CreatedAt: now,
	}, NewTask{ID: "task-first", Lane: TaskLaneMaintenance, Kind: TaskKindCanonicalSaved, CreatedAt: now})
	if err != nil {
		t.Fatal(err)
	}
	core, err := database.UpsertCoreArtifact(ctx, CoreArtifact{
		ID: "core-stale", ExactVersion: "1.13.19", OperatingSystem: "linux", Architecture: "amd64", Variant: "plain",
		SourceKind: CoreArtifactSourceUserVerified, UserSource: "test", ArchiveSHA256: strings.Repeat("c", 64),
		BinarySHA256: strings.Repeat("d", 64), BinaryPath: "/tmp/sing-box", ReportedVersion: "1.13.19",
		FeatureFingerprint: json.RawMessage(`{"status":"not_reported"}`), CreatedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = database.SaveCanonicalRevisionAndTask(ctx, first.ID, NewCanonicalRevision{
		ID: "revision-second", SchemaVersion: configuration.SchemaVersion,
		Document: json.RawMessage(`{"log":{}}`), CommandID: "command-second", CreatedAt: now.Add(time.Second),
	}, NewTask{ID: "task-second", Lane: TaskLaneMaintenance, Kind: TaskKindCanonicalSaved, CreatedAt: now.Add(time.Second)})
	if err != nil {
		t.Fatal(err)
	}

	_, err = database.CreateStartupArtifactAndCheckTask(ctx, StartupArtifact{
		ID: "startup-stale", CanonicalRevisionID: first.ID, ExactCoreVersion: core.ExactVersion,
		CoreArtifactID: core.ID, ConfigBytes: first.Document, CreatedAt: now.Add(2 * time.Second),
	}, NewTask{
		ID: "task-stale", Lane: TaskLaneMaintenance, Kind: TaskKindStartupCheck, CreatedAt: now.Add(2 * time.Second),
	}, CompiledStartupEvidence{ExpectedCanonicalHeadID: first.ID})
	if !errors.Is(err, ErrCompiledStartupEvidenceStale) {
		t.Fatalf("stale compile error = %v, want ErrCompiledStartupEvidenceStale", err)
	}
	if _, artifactErr := database.GetStartupArtifact(ctx, "startup-stale"); !errors.Is(artifactErr, ErrStartupArtifactNotFound) {
		t.Fatalf("stale compile persisted artifact: %v", artifactErr)
	}
	if _, taskErr := database.GetTask(ctx, "task-stale"); !errors.Is(taskErr, ErrTaskNotFound) {
		t.Fatalf("stale compile persisted task: %v", taskErr)
	}
}
