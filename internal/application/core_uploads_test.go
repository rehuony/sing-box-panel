// SPDX-License-Identifier: GPL-3.0-or-later

package application

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/rehuony/sing-box-panel/internal/artifactstore"
	"github.com/rehuony/sing-box-panel/internal/coreartifact"
	"github.com/rehuony/sing-box-panel/internal/store"
)

func TestGarbageCollectCoreUploadsDeletesOnlyStagedRegularFilesAtStartup(t *testing.T) {
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

	result, err := application.GarbageCollectCoreUploads(ctx)
	if err != nil {
		t.Fatalf("GarbageCollectCoreUploads() error = %v", err)
	}
	if result.Deleted != 2 || result.Retained != 0 || result.Aborted {
		t.Fatalf("garbage collection result = %+v", result)
	}
	assertPathMissing(t, active)
	assertPathMissing(t, orphan)
	assertPathExists(t, unmanaged)
	assertPathExists(t, directoryEntry)
	assertPathExists(t, symlinkEntry)
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

func TestDirectCoreImportCleansStagedFilesOnSuccessFailureAndCancellation(t *testing.T) {
	for _, scenario := range []string{"success", "failed", "canceled", "cleanup-failed"} {
		t.Run(scenario, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			app, _, directory := newCoreUploadTestApplication(t, context.Background())
			path := writeCoreUpload(t, directory, "core-upload-import")
			digest := mustDigest(t, "ab")
			version, _ := coreartifact.ParseExactVersion("1.13.19")
			source, _ := coreartifact.NewUserSource("browser upload")
			identity, err := coreartifact.NewIdentity(source, digest, coreartifact.OperatingSystemLinux, coreartifact.ArchitectureAMD64, coreartifact.VariantMusl, version)
			if err != nil {
				t.Fatal(err)
			}
			installer := fakeArtifactInstaller{importResult: artifactstore.Result{Identity: identity, BinarySHA256: mustDigest(t, "cd"), BinaryPath: "/secure/artifacts/sing-box"}}
			if scenario == "failed" {
				installer.err = errors.New("verification failed")
			}
			if scenario == "canceled" {
				cancel()
				installer.err = context.Canceled
			}
			if scenario == "cleanup-failed" {
				app.removeFile = func(string) error { return errors.New("injected removal failure") }
			}
			app.SetArtifactInstaller(installer)
			_, err = app.ImportCore(ctx, CoreImportRequest{SourcePath: path, SourceDescription: "browser upload", SHA256: digest.String(), ExactVersion: version.String(), Architecture: "amd64", Variant: "musl", DeleteSource: true})
			if (scenario == "success" || scenario == "cleanup-failed") && err != nil {
				t.Fatal(err)
			}
			if (scenario == "failed" || scenario == "canceled") && err == nil {
				t.Fatal("failed import succeeded")
			}
			if scenario == "cleanup-failed" {
				assertPathExists(t, path)
				logs, err := app.ListLogs(context.Background(), LogListRequest{Code: "core_upload.cleanup_failed"})
				if err != nil || len(logs.Items) != 1 || logs.Items[0].Level != store.LogLevelWarn {
					t.Fatalf("cleanup warning: %+v %v", logs, err)
				}
			} else {
				assertPathMissing(t, path)
			}
		})
	}
}
