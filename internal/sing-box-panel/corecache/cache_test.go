// SPDX-License-Identifier: GPL-3.0-or-later

package corecache

import (
	"bytes"
	"context"
	"crypto/sha256"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/rehuony/sing-box-panel/internal/coreartifact"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/store"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/telemetry"
)

type countingDownloader struct {
	content []byte
	calls   int
}

func (downloader *countingDownloader) Download(_ context.Context, _ string, destination io.Writer, _ int64) (int64, error) {
	downloader.calls++
	written, err := destination.Write(downloader.content)
	return int64(written), err
}

func TestCacheOfficialReusesTheCachedArchiveWhileItIsIntact(t *testing.T) {
	ctx := context.Background()
	database, err := store.Open(ctx, filepath.Join(t.TempDir(), "panel.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	content := bytes.Repeat([]byte("core"), 64)
	downloader := &countingDownloader{content: content}
	application := New(database, telemetry.New(database, nil, telemetry.Options{}), nil,
		Options{DataDir: t.TempDir(), Downloader: downloader})
	asset := validCatalogAsset(t)
	asset.Size, asset.APIDigest = int64(len(content)), coreartifact.NewSHA256(sha256.Sum256(content))
	if _, err := application.refreshCatalogWith(ctx, fakeCatalogRefresher{result: RefreshResult{Catalog: Catalog{
		RepositoryID: OfficialRepositoryID,
		Releases:     []Release{{ID: asset.ReleaseID, Tag: "v" + asset.Version.String(), Version: asset.Version, Assets: []Asset{asset}}},
	}}}); err != nil {
		t.Fatal(err)
	}

	first, err := application.CacheOfficial(ctx, asset.AssetID)
	if err != nil {
		t.Fatal(err)
	}
	second, err := application.CacheOfficial(ctx, asset.AssetID)
	if err != nil || second.ID != first.ID || downloader.calls != 1 {
		t.Fatalf("second install: id=%q err=%v downloads=%d", second.ID, err, downloader.calls)
	}

	// An altered or missing archive is downloaded again.
	archive := application.archivePath(first.SHA256)
	if err := os.WriteFile(archive, bytes.Repeat([]byte("x"), len(content)), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := application.CacheOfficial(ctx, asset.AssetID); err != nil || downloader.calls != 2 {
		t.Fatalf("altered archive: err=%v downloads=%d", err, downloader.calls)
	}
	if err := os.Remove(archive); err != nil {
		t.Fatal(err)
	}
	if _, err := application.CacheOfficial(ctx, asset.AssetID); err != nil || downloader.calls != 3 {
		t.Fatalf("missing archive: err=%v downloads=%d", err, downloader.calls)
	}
}
