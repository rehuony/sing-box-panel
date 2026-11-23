// SPDX-License-Identifier: GPL-3.0-or-later

package corecache

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/store"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/telemetry"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/testutil"
)

func TestGarbageCollectCoreUploadsDeletesOnlyStagedRegularFilesAtStartup(t *testing.T) {
	ctx := context.Background()
	application, _, directory := newCoreUploadTestApplication(t, ctx)
	active := testutil.WriteCoreUpload(t, directory, "core-upload-active")
	orphan := testutil.WriteCoreUpload(t, directory, "core-upload-orphan")
	unmanaged := testutil.WriteCoreUpload(t, directory, "administrator-archive")
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
	testutil.AssertPathMissing(t, active)
	testutil.AssertPathMissing(t, orphan)
	testutil.AssertPathExists(t, unmanaged)
	testutil.AssertPathExists(t, directoryEntry)
	testutil.AssertPathExists(t, symlinkEntry)
}

func newCoreUploadTestApplication(
	t *testing.T,
	ctx context.Context,
) (*Service, *store.Store, string) {
	t.Helper()
	dataDirectory := t.TempDir()
	database, err := store.Open(ctx, filepath.Join(dataDirectory, "panel.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	application := New(database, telemetry.New(database, nil, telemetry.Options{}), nil, Options{DataDir: dataDirectory})
	directory := filepath.Join(dataDirectory, "imports")
	if err := os.MkdirAll(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	return application, database, directory
}
