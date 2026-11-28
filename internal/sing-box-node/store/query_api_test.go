package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/rehuony/sing-box-panel/internal/configuration"
)

func TestReadAPIsReturnEmptyCollections(t *testing.T) {
	ctx := testContext(t)
	store := openTestStore(t, ctx)

	artifacts, err := store.ListCoreArtifacts(ctx, CoreArtifactListFilter{})
	if err != nil || artifacts.Items == nil || len(artifacts.Items) != 0 || artifacts.Next != nil {
		t.Fatalf("empty ListCoreArtifacts() = %+v, %v", artifacts, err)
	}
}

func TestConfigurationSnapshotsRemainReadableByID(t *testing.T) {
	ctx := testContext(t)
	store := openTestStore(t, ctx)
	now := time.Date(2026, time.August, 28, 9, 0, 0, 0, time.UTC)
	for i := 1; i <= 3; i++ {
		_, err := recordTestRevision(ctx, store,
			NewCanonicalRevision{
				ID:            fmt.Sprintf("revision-%d", i),
				SchemaVersion: configuration.SchemaVersion,
				Document:      json.RawMessage(fmt.Sprintf(`{"experimental":{"value":%d}}`, i)),
				CommandID:     fmt.Sprintf("command-%d", i),
				CreatedAt:     now.Add(time.Duration(i) * time.Second),
			},
		)
		if err != nil {
			t.Fatalf("RecordRevision(%d) error = %v", i, err)
		}
	}

	byID, err := store.GetCanonicalRevision(ctx, "revision-2")
	if err != nil {
		t.Fatalf("GetCanonicalRevision() error = %v", err)
	}
	byID.Document[0] = 'x'
	unchanged, err := store.GetCanonicalRevision(ctx, "revision-2")
	if err != nil {
		t.Fatalf("GetCanonicalRevision() after mutation error = %v", err)
	}
	if string(unchanged.Document) != `{"experimental":{"value":2}}` {
		t.Fatalf("stored canonical document = %s, want defensive copy", unchanged.Document)
	}
	if _, err := store.GetCanonicalRevision(ctx, "missing"); !errors.Is(err, ErrCanonicalRevisionNotFound) {
		t.Fatalf("missing revision error = %v, want ErrCanonicalRevisionNotFound", err)
	}
}

func TestCoreArtifactRepositoryAndRemovalEligibility(t *testing.T) {
	ctx := testContext(t)
	store := openTestStore(t, ctx)
	now := time.Date(2026, time.August, 28, 10, 0, 0, 0, time.UTC)

	artifacts := []CoreArtifact{
		testCoreArtifact("artifact-1", 1, '1', "amd64", now),
		testCoreArtifact("artifact-2", 2, '2', "arm64", now.Add(time.Second)),
		testCoreArtifact("artifact-3", 3, '3', "amd64", now.Add(2*time.Second)),
	}
	for _, artifact := range artifacts {
		if _, err := store.UpsertCoreArtifact(ctx, artifact); err != nil {
			t.Fatalf("UpsertCoreArtifact(%q) error = %v", artifact.ID, err)
		}
	}

	first, err := store.ListCoreArtifacts(ctx, CoreArtifactListFilter{Limit: 2})
	if err != nil {
		t.Fatalf("ListCoreArtifacts(first) error = %v", err)
	}
	assertArtifactIDs(t, first.Items, "artifact-3", "artifact-2")
	if first.Next == nil || first.Next.ID != "artifact-2" {
		t.Fatalf("first artifact cursor = %+v, want artifact-2", first.Next)
	}
	second, err := store.ListCoreArtifacts(ctx, CoreArtifactListFilter{Limit: 2, Cursor: first.Next})
	if err != nil {
		t.Fatalf("ListCoreArtifacts(second) error = %v", err)
	}
	assertArtifactIDs(t, second.Items, "artifact-1")
	if second.Next != nil {
		t.Fatalf("second artifact cursor = %+v, want nil", second.Next)
	}
	filtered, err := store.ListCoreArtifacts(ctx, CoreArtifactListFilter{Architecture: "amd64", Limit: 10})
	if err != nil {
		t.Fatalf("ListCoreArtifacts(filtered) error = %v", err)
	}
	assertArtifactIDs(t, filtered.Items, "artifact-3", "artifact-1")

	updated := artifacts[0]
	updated.CreatedAt = now.Add(time.Hour)
	stored, err := store.UpsertCoreArtifact(ctx, updated)
	if err != nil {
		t.Fatalf("UpsertCoreArtifact(retry) error = %v", err)
	}
	if !stored.CreatedAt.Equal(now) {
		t.Fatalf("updated artifact = %+v, want original creation time", stored)
	}
	reinstall := artifacts[0]
	reinstalled, err := store.UpsertCoreArtifact(ctx, reinstall)
	if err != nil {
		t.Fatalf("UpsertCoreArtifact(reinstall) error = %v", err)
	}
	if reinstalled.ID != artifacts[0].ID || !reinstalled.CreatedAt.Equal(now) {
		t.Fatalf("reinstalled artifact identity or creation time changed: %+v", reinstalled)
	}
	mismatch := updated
	mismatch.BinaryPath += ".changed"
	if _, err := store.UpsertCoreArtifact(ctx, mismatch); !errors.Is(err, ErrCoreArtifactIdentityConflict) {
		t.Fatalf("identity-changing upsert error = %v, want ErrCoreArtifactIdentityConflict", err)
	}

	stored.FeatureFingerprint[0] = 'x'
	unchanged, err := store.GetCoreArtifact(ctx, stored.ID)
	if err != nil {
		t.Fatalf("GetCoreArtifact() after mutation error = %v", err)
	}
	if string(unchanged.FeatureFingerprint) != `{"features":["with_clash_api"]}` {
		t.Fatalf("stored feature fingerprint = %s, want defensive copy", unchanged.FeatureFingerprint)
	}

	// A core nothing selects is removed and no longer listed; installing it
	// again restores it as of the new installation.
	removedAt := now.Add(time.Hour)
	removal, err := store.RemoveCoreArtifact(ctx, "artifact-2", removedAt)
	if err != nil || removal.Artifact.ID != "artifact-2" || removal.FilesShared {
		t.Fatalf("RemoveCoreArtifact(artifact-2) = %+v, %v; want removed with its own files", removal, err)
	}
	if _, err := store.GetCoreArtifact(ctx, "artifact-2"); !errors.Is(err, ErrCoreArtifactNotFound) {
		t.Fatalf("removed artifact lookup error = %v, want ErrCoreArtifactNotFound", err)
	}
	listed, err := store.ListCoreArtifacts(ctx, CoreArtifactListFilter{Limit: 10})
	if err != nil {
		t.Fatalf("ListCoreArtifacts(after removal) error = %v", err)
	}
	assertArtifactIDs(t, listed.Items, "artifact-3", "artifact-1")
	again := artifacts[1]
	again.CreatedAt = removedAt
	if restored, err := store.UpsertCoreArtifact(ctx, again); err != nil || !restored.CreatedAt.Equal(removedAt) {
		t.Fatalf("UpsertCoreArtifact(removed) = %+v, %v; want it installed again at %v", restored, err, removedAt)
	}

	// Earlier startup records and bundles are history and keep no core; the
	// selected one stays, and a rollback to a removed core is dropped.
	revision, err := recordTestRevision(ctx, store,
		NewCanonicalRevision{
			ID: "artifact-reference-revision", SchemaVersion: configuration.SchemaVersion,
			Document: json.RawMessage(`{}`), CommandID: "artifact-reference-command", CreatedAt: now,
		},
	)
	if err != nil {
		t.Fatalf("SaveConfigurationFile(reference) error = %v", err)
	}
	for _, reference := range []struct{ startup, bundle, core string }{
		{"startup-1", "bundle-1", "artifact-1"},
		{"startup-3", "bundle-3", "artifact-3"},
	} {
		if _, err := store.db.ExecContext(
			ctx,
			`INSERT INTO startup_artifacts(
			    id, canonical_revision_id, exact_core_version, core_artifact_id,
			    config_bytes, config_sha256, state, created_at
			 ) VALUES (?, ?, '1.13.19', ?, ?, ?, 'ready', ?)`,
			reference.startup, revision.ID, reference.core, []byte(`{}`), strings.Repeat("f", 64), formatTime(now),
		); err != nil {
			t.Fatalf("insert startup artifact %s: %v", reference.startup, err)
		}
		if _, err := store.db.ExecContext(
			ctx,
			`INSERT INTO activation_bundles(id, startup_artifact_id, monitoring_tier, sha256, created_at)
			 VALUES (?, ?, 'process_only', ?, ?)`,
			reference.bundle, reference.startup, strings.Repeat(reference.core[len(reference.core)-1:], 64), formatTime(now),
		); err != nil {
			t.Fatalf("insert activation bundle %s: %v", reference.bundle, err)
		}
	}
	if _, err := store.db.ExecContext(
		ctx,
		`UPDATE hub_state SET desired_bundle_id = 'bundle-3', applied_bundle_id = 'bundle-3', rollback_bundle_id = 'bundle-1'`,
	); err != nil {
		t.Fatalf("select bundle-3: %v", err)
	}
	if _, err := store.RemoveCoreArtifact(ctx, "artifact-3", removedAt); !errors.Is(err, ErrCoreArtifactInUse) {
		t.Fatalf("RemoveCoreArtifact(selected) error = %v, want ErrCoreArtifactInUse", err)
	}
	if _, err := store.RemoveCoreArtifact(ctx, "artifact-1", removedAt); err != nil {
		t.Fatalf("RemoveCoreArtifact(rollback target) error = %v", err)
	}
	var rollback sql.NullString
	if err := store.db.QueryRowContext(ctx, `SELECT rollback_bundle_id FROM hub_state`).Scan(&rollback); err != nil || rollback.Valid {
		t.Fatalf("rollback after removing its core = %+v, %v; want none", rollback, err)
	}
}

func TestConcurrentCoreArtifactUpsertsAndLists(t *testing.T) {
	ctx := testContext(t)
	path := filepath.Join(t.TempDir(), "panel.db")
	first, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("first Open() error = %v", err)
	}
	t.Cleanup(func() { _ = first.Close() })
	second, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("second Open() error = %v", err)
	}
	t.Cleanup(func() { _ = second.Close() })

	now := time.Date(2026, time.August, 28, 11, 0, 0, 0, time.UTC)
	start := make(chan struct{})
	errCh := make(chan error, 16)
	var workers sync.WaitGroup
	for i := 1; i <= 12; i++ {
		workers.Add(1)
		go func(i int) {
			defer workers.Done()
			<-start
			candidate := first
			if i%2 == 0 {
				candidate = second
			}
			artifact := testCoreArtifact(
				fmt.Sprintf("concurrent-%02d", i),
				int64(i),
				'1',
				"amd64",
				now.Add(time.Duration(i)*time.Nanosecond),
			)
			artifact.ArchiveSHA256 = fmt.Sprintf("%064x", i)
			artifact.BinaryPath = fmt.Sprintf("/var/lib/sing-box-panel/core/%064x", i)
			_, err := candidate.UpsertCoreArtifact(ctx, artifact)
			errCh <- err
		}(i)
	}
	workers.Add(1)
	go func() {
		defer workers.Done()
		<-start
		_, err := second.ListCoreArtifacts(ctx, CoreArtifactListFilter{Limit: 50})
		errCh <- err
	}()
	close(start)
	workers.Wait()
	close(errCh)
	for err := range errCh {
		if err != nil {
			t.Fatalf("concurrent repository operation error = %v", err)
		}
	}

	page, err := first.ListCoreArtifacts(ctx, CoreArtifactListFilter{Limit: 50})
	if err != nil {
		t.Fatalf("final ListCoreArtifacts() error = %v", err)
	}
	if len(page.Items) != 12 {
		t.Fatalf("final artifact count = %d, want 12", len(page.Items))
	}
}

func testCoreArtifact(
	id string,
	officialID int64,
	digestCharacter byte,
	architecture string,
	createdAt time.Time,
) CoreArtifact {
	return CoreArtifact{
		ID:                 id,
		ExactVersion:       "1.13.19",
		OperatingSystem:    "linux",
		Architecture:       architecture,
		Variant:            "musl",
		SourceKind:         CoreArtifactSourceOfficial,
		RepositoryID:       1,
		ReleaseID:          100,
		AssetID:            officialID,
		ArchiveSHA256:      strings.Repeat(string(digestCharacter), 64),
		BinarySHA256:       strings.Repeat(string(digestCharacter), 64),
		BinaryPath:         "/var/lib/sing-box-panel/core/" + id,
		ReportedVersion:    "1.13.19",
		FeatureFingerprint: json.RawMessage(`{"features":["with_clash_api"]}`),

		CreatedAt: createdAt,
	}
}

func assertArtifactIDs(t *testing.T, artifacts []CoreArtifact, want ...string) {
	t.Helper()
	if len(artifacts) != len(want) {
		t.Fatalf("artifact count = %d, want %d: %+v", len(artifacts), len(want), artifacts)
	}
	for index := range want {
		if artifacts[index].ID != want[index] {
			t.Fatalf("artifact[%d] id = %q, want %q", index, artifacts[index].ID, want[index])
		}
	}
}

func TestRecordRevisionKeepsThePanelsIDForAnUnchangedDocument(t *testing.T) {
	ctx := testContext(t)
	store := openTestStore(t, ctx)
	now := time.Date(2026, time.November, 9, 9, 0, 0, 0, time.UTC)
	document := json.RawMessage(`{"log":{"level":"info"}}`)
	// A node enrolled again receives its running document under a new ID.
	for _, id := range []string{"revision-before", "revision-after"} {
		recorded, err := store.RecordRevision(ctx, id, document, now)
		if err != nil || recorded.ID != id {
			t.Fatalf("RecordRevision(%s) = %+v, %v", id, recorded, err)
		}
		head, err := store.Head(ctx)
		if err != nil || head == nil || head.ID != id {
			t.Fatalf("head after %s = %+v, %v", id, head, err)
		}
	}
}
