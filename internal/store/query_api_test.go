package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestListTasksFiltersAndPaginatesWithStableCursor(t *testing.T) {
	ctx := testContext(t)
	store := openTestStore(t, ctx)
	now := time.Date(2026, time.August, 28, 8, 0, 0, 0, time.UTC)

	for _, input := range []EnqueueTaskInput{
		{ID: "m1", Lane: TaskLaneMaintenance, Kind: TaskKindCatalogRefresh, Payload: json.RawMessage(`{"n":1}`), CreatedAt: now},
		{ID: "m2", Lane: TaskLaneMaintenance, Kind: TaskKindCatalogRefresh, Payload: json.RawMessage(`{"n":2}`), CreatedAt: now.Add(time.Second)},
		{ID: "m3", Lane: TaskLaneMaintenance, Kind: TaskKindCoreInstall, Payload: json.RawMessage(`{"n":3}`), CreatedAt: now.Add(time.Second)},
		{ID: "m4", Lane: TaskLaneMaintenance, Kind: TaskKindCatalogRefresh, Payload: json.RawMessage(`{"n":4}`), CreatedAt: now.Add(2 * time.Second)},
	} {
		enqueueTask(t, ctx, store, input)
	}
	if _, _, err := store.RequestTaskCancellation(ctx, "m4", now.Add(3*time.Second)); err != nil {
		t.Fatalf("RequestTaskCancellation(m4) error = %v", err)
	}

	first, err := store.ListTasks(ctx, TaskListFilter{Limit: 2})
	if err != nil {
		t.Fatalf("ListTasks(first) error = %v", err)
	}
	assertTaskIDs(t, first.Items, "m4", "m3")
	if first.Next == nil || first.Next.ID != "m3" {
		t.Fatalf("first task cursor = %+v, want m3", first.Next)
	}
	second, err := store.ListTasks(ctx, TaskListFilter{Limit: 2, Cursor: first.Next})
	if err != nil {
		t.Fatalf("ListTasks(second) error = %v", err)
	}
	assertTaskIDs(t, second.Items, "m2", "m1")
	if second.Next != nil {
		t.Fatalf("second task cursor = %+v, want nil", second.Next)
	}

	filtered, err := store.ListTasks(ctx, TaskListFilter{
		Lane: TaskLaneMaintenance, Status: TaskStatusQueued, Kind: TaskKindCatalogRefresh, Limit: 10,
	})
	if err != nil {
		t.Fatalf("ListTasks(filtered) error = %v", err)
	}
	assertTaskIDs(t, filtered.Items, "m2", "m1")

	if _, err := store.ListTasks(ctx, TaskListFilter{Kind: `alpha' OR 1=1 --`, Limit: 10}); err == nil {
		t.Fatal("ListTasks accepted an unknown injection-shaped task kind")
	}

	first.Items[0].Payload[0] = 'x'
	stored, err := store.GetTask(ctx, "m4")
	if err != nil {
		t.Fatalf("GetTask(m4) error = %v", err)
	}
	if string(stored.Payload) != `{"n":4}` {
		t.Fatalf("stored payload = %s, want defensive copy", stored.Payload)
	}
}

func TestReadAPIsReturnEmptyCollections(t *testing.T) {
	ctx := testContext(t)
	store := openTestStore(t, ctx)

	tasks, err := store.ListTasks(ctx, TaskListFilter{})
	if err != nil || tasks.Items == nil || len(tasks.Items) != 0 || tasks.Next != nil {
		t.Fatalf("empty ListTasks() = %+v, %v", tasks, err)
	}
	revisions, err := store.ListCanonicalRevisions(ctx, CanonicalRevisionListFilter{})
	if err != nil || revisions.Items == nil || len(revisions.Items) != 0 || revisions.Next != nil {
		t.Fatalf("empty ListCanonicalRevisions() = %+v, %v", revisions, err)
	}
	artifacts, err := store.ListCoreArtifacts(ctx, CoreArtifactListFilter{})
	if err != nil || artifacts.Items == nil || len(artifacts.Items) != 0 || artifacts.Next != nil {
		t.Fatalf("empty ListCoreArtifacts() = %+v, %v", artifacts, err)
	}
}

func TestCanonicalRevisionQueriesAndPagination(t *testing.T) {
	ctx := testContext(t)
	store := openTestStore(t, ctx)
	now := time.Date(2026, time.August, 28, 9, 0, 0, 0, time.UTC)
	head := ""
	for i := 1; i <= 3; i++ {
		revision, err := store.SaveCanonicalRevisionAndTask(
			ctx,
			head,
			NewCanonicalRevision{
				ID:            fmt.Sprintf("revision-%d", i),
				SchemaVersion: 2,
				Document:      json.RawMessage(fmt.Sprintf(`{"schema_version":2,"configuration":{"experimental":{"value":%d}}}`, i)),
				CommandID:     fmt.Sprintf("command-%d", i),
				CreatedAt:     now.Add(time.Duration(i) * time.Second),
			},
			NewTask{
				ID:      fmt.Sprintf("revision-task-%d", i),
				Lane:    TaskLaneMaintenance,
				Kind:    TaskKindCanonicalSaved,
				Payload: json.RawMessage(`{}`),
			},
		)
		if err != nil {
			t.Fatalf("SaveCanonicalRevisionAndTask(%d) error = %v", i, err)
		}
		head = revision.ID
	}

	first, err := store.ListCanonicalRevisions(ctx, CanonicalRevisionListFilter{Limit: 2})
	if err != nil {
		t.Fatalf("ListCanonicalRevisions(first) error = %v", err)
	}
	if len(first.Items) != 2 || first.Items[0].Sequence != 3 || first.Items[1].Sequence != 2 {
		t.Fatalf("first revision page = %+v, want sequences 3,2", first.Items)
	}
	if first.Next == nil || first.Next.BeforeSequence != 2 {
		t.Fatalf("first revision cursor = %+v, want before 2", first.Next)
	}
	second, err := store.ListCanonicalRevisions(ctx, CanonicalRevisionListFilter{
		Cursor: first.Next,
		Limit:  2,
	})
	if err != nil {
		t.Fatalf("ListCanonicalRevisions(second) error = %v", err)
	}
	if len(second.Items) != 1 || second.Items[0].Sequence != 1 || second.Next != nil {
		t.Fatalf("second revision page = %+v cursor=%+v, want sequence 1 and no cursor", second.Items, second.Next)
	}

	byID, err := store.GetCanonicalRevision(ctx, "revision-2")
	if err != nil {
		t.Fatalf("GetCanonicalRevision() error = %v", err)
	}
	bySequence, err := store.GetCanonicalRevisionBySequence(ctx, 2)
	if err != nil {
		t.Fatalf("GetCanonicalRevisionBySequence() error = %v", err)
	}
	if byID.ID != bySequence.ID || string(byID.Document) != string(bySequence.Document) {
		t.Fatalf("revision selectors disagree: by ID=%+v by sequence=%+v", byID, bySequence)
	}
	byID.Document[0] = 'x'
	unchanged, err := store.GetCanonicalRevision(ctx, "revision-2")
	if err != nil {
		t.Fatalf("GetCanonicalRevision() after mutation error = %v", err)
	}
	if string(unchanged.Document) != `{"configuration":{"experimental":{"value":2}},"schema_version":2}` {
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

	quarantined, err := store.RestrictCoreArtifactVerification(ctx, artifacts[1].ID, CoreArtifactQuarantined, now.Add(time.Minute))
	if err != nil || quarantined.VerificationState != CoreArtifactQuarantined {
		t.Fatalf("quarantined artifact = %+v, error = %v", quarantined, err)
	}
	revokedFromQuarantine, err := store.RestrictCoreArtifactVerification(ctx, artifacts[1].ID, CoreArtifactRevoked, now.Add(2*time.Minute))
	if err != nil || revokedFromQuarantine.VerificationState != CoreArtifactRevoked {
		t.Fatalf("revoked artifact = %+v, error = %v", revokedFromQuarantine, err)
	}
	stillRevoked, err := store.RestrictCoreArtifactVerification(ctx, artifacts[1].ID, CoreArtifactQuarantined, now.Add(3*time.Minute))
	if err != nil || stillRevoked.VerificationState != CoreArtifactRevoked {
		t.Fatalf("post-revocation quarantine = %+v, error = %v", stillRevoked, err)
	}

	updated := artifacts[0]
	updated.VerificationState = CoreArtifactRevoked
	updated.CreatedAt = now.Add(time.Hour)
	stored, err := store.UpsertCoreArtifact(ctx, updated)
	if err != nil {
		t.Fatalf("UpsertCoreArtifact(update state) error = %v", err)
	}
	if stored.VerificationState != CoreArtifactRevoked || !stored.CreatedAt.Equal(now) {
		t.Fatalf("updated artifact = %+v, want revoked with original creation time", stored)
	}
	reinstall := artifacts[0]
	reinstalled, err := store.UpsertCoreArtifact(ctx, reinstall)
	if err != nil {
		t.Fatalf("UpsertCoreArtifact(reinstall revoked bytes) error = %v", err)
	}
	if reinstalled.VerificationState != CoreArtifactRevoked {
		t.Fatalf("reinstalled artifact verification = %q, want terminal revoked", reinstalled.VerificationState)
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

	eligibility, err := store.CoreArtifactRemovalEligibility(ctx, "artifact-2")
	if err != nil || !eligibility.Eligible {
		t.Fatalf("artifact-2 eligibility = %+v, %v, want eligible", eligibility, err)
	}
	if err := store.RemoveCoreArtifact(ctx, "artifact-2"); err != nil {
		t.Fatalf("RemoveCoreArtifact(artifact-2) error = %v", err)
	}
	if _, err := store.GetCoreArtifact(ctx, "artifact-2"); !errors.Is(err, ErrCoreArtifactNotFound) {
		t.Fatalf("removed artifact lookup error = %v, want ErrCoreArtifactNotFound", err)
	}

	revision, err := store.SaveCanonicalRevisionAndTask(
		ctx,
		"",
		NewCanonicalRevision{
			ID: "artifact-reference-revision", SchemaVersion: 2,
			Document: json.RawMessage(`{"schema_version":2,"configuration":{}}`), CommandID: "artifact-reference-command", CreatedAt: now,
		},
		NewTask{ID: "artifact-reference-task", Lane: TaskLaneMaintenance, Kind: TaskKindCanonicalSaved},
	)
	if err != nil {
		t.Fatalf("SaveCanonicalRevisionAndTask(reference) error = %v", err)
	}
	if _, err := store.db.ExecContext(
		ctx,
		`INSERT INTO startup_artifacts(
		    id, canonical_revision_id, exact_core_version, adapter_id,
		    adapter_revision, core_artifact_id, config_bytes, config_sha256,
		    diagnostics_json, state, created_at
		 ) VALUES (?, ?, '1.13.19', 'test-adapter', '1', ?, ?, ?, '[]', 'ready', ?)`,
		"startup-reference",
		revision.ID,
		"artifact-1",
		[]byte(`{}`),
		strings.Repeat("f", 64),
		formatTaskTime(now),
	); err != nil {
		t.Fatalf("insert startup artifact reference: %v", err)
	}
	blocked, err := store.CoreArtifactRemovalEligibility(ctx, "artifact-1")
	if err != nil || blocked.Eligible || blocked.StartupArtifactReferences != 1 {
		t.Fatalf("artifact-1 eligibility = %+v, %v, want one blocking startup", blocked, err)
	}
	if err := store.RemoveCoreArtifact(ctx, "artifact-1"); !errors.Is(err, ErrCoreArtifactInUse) {
		t.Fatalf("RemoveCoreArtifact(in use) error = %v, want ErrCoreArtifactInUse", err)
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
		Variant:            "plain",
		SourceKind:         CoreArtifactSourceOfficial,
		RepositoryID:       1,
		ReleaseID:          100,
		AssetID:            officialID,
		ArchiveSHA256:      strings.Repeat(string(digestCharacter), 64),
		BinarySHA256:       strings.Repeat(string(digestCharacter), 64),
		BinaryPath:         "/var/lib/sing-box-panel/core/" + id,
		ReportedVersion:    "1.13.19",
		FeatureFingerprint: json.RawMessage(`{"features":["with_clash_api"]}`),
		VerificationState:  CoreArtifactVerified,
		CreatedAt:          createdAt,
	}
}

func assertTaskIDs(t *testing.T, tasks []Task, want ...string) {
	t.Helper()
	if len(tasks) != len(want) {
		t.Fatalf("task count = %d, want %d: %+v", len(tasks), len(want), tasks)
	}
	for index := range want {
		if tasks[index].ID != want[index] {
			t.Fatalf("task[%d] id = %q, want %q", index, tasks[index].ID, want[index])
		}
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
