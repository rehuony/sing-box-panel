// SPDX-License-Identifier: GPL-3.0-or-later

package application

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/rehuony/sing-box-panel/internal/artifactstore"
	"github.com/rehuony/sing-box-panel/internal/coreartifact"
	"github.com/rehuony/sing-box-panel/internal/store"
)

func TestCoreUploadIsRetainedUntilTaskCompletionAndThenFinalized(t *testing.T) {
	ctx := context.Background()
	application, database, directory := newCoreUploadTestApplication(t, ctx)
	now := time.Date(2026, time.August, 29, 11, 0, 0, 0, time.UTC)
	application.now = func() time.Time { return now }
	path := writeCoreUpload(t, directory, "core-upload-running")

	digest := mustDigest(t, "ab")
	version, err := coreartifact.ParseExactVersion("1.13.19")
	if err != nil {
		t.Fatal(err)
	}
	source, err := coreartifact.NewUserSource("browser upload")
	if err != nil {
		t.Fatal(err)
	}
	identity, err := coreartifact.NewIdentity(
		source, digest, coreartifact.OperatingSystemLinux, coreartifact.ArchitectureAMD64,
		coreartifact.VariantPlain, version,
	)
	if err != nil {
		t.Fatal(err)
	}
	queued, err := application.QueueCoreImport(ctx, CoreImportRequest{
		SourcePath: path, SourceDescription: "browser upload", SHA256: digest.String(),
		ExactVersion: version.String(), Architecture: "amd64", Variant: "plain", DeleteSource: true,
	})
	if err != nil {
		t.Fatalf("QueueCoreImport() error = %v", err)
	}
	claimed, err := database.ClaimTask(ctx, store.ClaimTaskInput{
		Lane: store.TaskLaneMaintenance, LeaseOwner: "test-worker", Now: now, LeaseDuration: time.Minute,
	})
	if err != nil || claimed == nil || claimed.ID != queued.ID {
		t.Fatalf("ClaimTask() = %+v, %v", claimed, err)
	}
	if _, err := application.ExecuteCoreArtifactTask(
		ctx,
		claimed.Kind,
		claimed.Payload,
		fakeArtifactInstaller{importResult: artifactstore.Result{
			Identity: identity, BinarySHA256: mustDigest(t, "cd"), BinaryPath: "/secure/artifacts/sing-box",
		}},
		nil,
	); err != nil {
		t.Fatalf("ExecuteCoreArtifactTask() error = %v", err)
	}
	assertPathExists(t, path)

	completed, err := database.CompleteTask(
		ctx, claimed.ID, claimed.LeaseOwner, now.Add(time.Second), store.TaskCompletion{Succeeded: true},
	)
	if err != nil {
		t.Fatalf("CompleteTask() error = %v", err)
	}
	assertPathExists(t, path)
	application.FinalizeTaskResources(ctx, completed)
	assertPathMissing(t, path)
}

func TestQueuedCoreUploadCancellationFinalizesAfterCommit(t *testing.T) {
	ctx := context.Background()
	application, _, directory := newCoreUploadTestApplication(t, ctx)
	path := writeCoreUpload(t, directory, "core-upload-canceled")
	queued := queueCoreUploadTestTask(t, ctx, application, path)

	canceled, err := application.CancelTask(ctx, queued.ID)
	if err != nil {
		t.Fatalf("CancelTask() error = %v", err)
	}
	if canceled.Status != store.TaskStatusCanceled {
		t.Fatalf("canceled status = %q, want canceled", canceled.Status)
	}
	assertPathMissing(t, path)

	// Reusing the old staging filename must not let an idempotent cancellation
	// of an already-terminal task delete an unrelated, newly-created file.
	if err := os.WriteFile(path, []byte("new upload"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := application.CancelTask(ctx, queued.ID); err != nil {
		t.Fatalf("second CancelTask() error = %v", err)
	}
	assertPathExists(t, path)
}

func TestCoreUploadCleanupFailureIsWarnedWithoutRollingBackTask(t *testing.T) {
	ctx := context.Background()
	application, _, directory := newCoreUploadTestApplication(t, ctx)
	path := writeCoreUpload(t, directory, "core-upload-blocked")
	queued := queueCoreUploadTestTask(t, ctx, application, path)
	application.removeFile = func(string) error { return errors.New("injected removal failure") }

	canceled, err := application.CancelTask(ctx, queued.ID)
	if err != nil {
		t.Fatalf("CancelTask() error = %v", err)
	}
	if canceled.Status != store.TaskStatusCanceled {
		t.Fatalf("canceled status = %q, want canceled", canceled.Status)
	}
	assertPathExists(t, path)
	logs, err := application.ListLogs(ctx, LogListRequest{Code: "task.resource_cleanup_failed", Limit: 10})
	if err != nil {
		t.Fatalf("ListLogs() error = %v", err)
	}
	if len(logs.Items) != 1 || logs.Items[0].Level != store.LogLevelWarn {
		t.Fatalf("cleanup logs = %+v, want one warning", logs.Items)
	}
}

func TestGarbageCollectCoreUploadsDeletesOnlyUnreferencedRegularFiles(t *testing.T) {
	ctx := context.Background()
	application, _, directory := newCoreUploadTestApplication(t, ctx)
	active := writeCoreUpload(t, directory, "core-upload-active")
	orphan := writeCoreUpload(t, directory, "core-upload-orphan")
	unmanaged := writeCoreUpload(t, directory, "administrator-archive")
	directoryEntry := filepath.Join(directory, "core-upload-directory")
	if err := os.Mkdir(directoryEntry, 0o700); err != nil {
		t.Fatal(err)
	}
	symlinkEntry := filepath.Join(directory, "core-upload-symlink")
	if err := os.Symlink(orphan, symlinkEntry); err != nil {
		t.Fatal(err)
	}
	queueCoreUploadTestTask(t, ctx, application, active)

	result, err := application.GarbageCollectCoreUploads(ctx)
	if err != nil {
		t.Fatalf("GarbageCollectCoreUploads() error = %v", err)
	}
	if result.Deleted != 1 || result.Retained != 1 || result.Aborted {
		t.Fatalf("garbage collection result = %+v", result)
	}
	assertPathExists(t, active)
	assertPathMissing(t, orphan)
	assertPathExists(t, unmanaged)
	assertPathExists(t, directoryEntry)
	assertPathExists(t, symlinkEntry)
}

func TestGarbageCollectCoreUploadsAbortsOnUnparseableActivePayload(t *testing.T) {
	ctx := context.Background()
	application, database, directory := newCoreUploadTestApplication(t, ctx)
	orphan := writeCoreUpload(t, directory, "core-upload-orphan")
	if _, err := database.EnqueueTask(ctx, store.EnqueueTaskInput{
		ID: "invalid-active-import", Lane: store.TaskLaneMaintenance, Kind: store.TaskKindCoreImport,
		Payload:   []byte(`{"source_path":"/uncertain","delete_source":true,"unexpected":true}`),
		CreatedAt: time.Date(2026, time.August, 29, 12, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("EnqueueTask() error = %v", err)
	}

	result, err := application.GarbageCollectCoreUploads(ctx)
	if err == nil || !result.Aborted || result.Deleted != 0 {
		t.Fatalf("GarbageCollectCoreUploads() = %+v, %v; want aborted pass", result, err)
	}
	assertPathExists(t, orphan)
}

func newCoreUploadTestApplication(
	t *testing.T,
	ctx context.Context,
) (*Application, *store.Store, string) {
	t.Helper()
	dataDirectory := t.TempDir()
	database, err := store.Open(ctx, filepath.Join(dataDirectory, "panel.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	application := newApplication(database)
	application.settings.DataDir = dataDirectory
	directory := filepath.Join(application.settings.DataDir, "imports")
	if err := os.MkdirAll(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	return application, database, directory
}

func queueCoreUploadTestTask(t *testing.T, ctx context.Context, application *Application, path string) Task {
	t.Helper()
	task, err := application.QueueCoreImport(ctx, CoreImportRequest{
		SourcePath: path, SourceDescription: "browser upload",
		SHA256:       "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		ExactVersion: "1.13.19", Architecture: "amd64", Variant: "plain", DeleteSource: true,
	})
	if err != nil {
		t.Fatalf("QueueCoreImport() error = %v", err)
	}
	return task
}

func writeCoreUpload(t *testing.T, directory, name string) string {
	t.Helper()
	path := filepath.Join(directory, name)
	if err := os.WriteFile(path, []byte("archive"), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func assertPathExists(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Lstat(path); err != nil {
		t.Fatalf("expected %s to exist: %v", path, err)
	}
}

func assertPathMissing(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected %s to be absent, got %v", path, err)
	}
}
