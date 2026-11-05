// SPDX-License-Identifier: GPL-3.0-or-later

package corecache

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/rehuony/sing-box-panel/internal/coreartifact"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/settings"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/store"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/telemetry"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/testutil"
)

func TestCatalogNotModifiedRequiresAndReusesLocalSnapshot(t *testing.T) {
	ctx := context.Background()
	database, err := store.Open(ctx, filepath.Join(t.TempDir(), "panel.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	application := New(database, nil, nil, Options{})
	if _, err := application.refreshCatalogWith(ctx, fakeCatalogRefresher{result: RefreshResult{NotModified: true}}); err == nil {
		t.Fatal("not-modified without local state was accepted")
	}
	asset := validCatalogAsset(t)
	initial := Catalog{RepositoryID: OfficialRepositoryID, Releases: []Release{{
		ID: asset.ReleaseID, Tag: "v1.13.19", Version: asset.Version, Assets: []Asset{asset},
	}}}
	if _, err := application.refreshCatalogWith(ctx, fakeCatalogRefresher{result: RefreshResult{Catalog: initial, ETag: "v1"}}); err != nil {
		t.Fatal(err)
	}
	beforeFailure, err := application.Catalog(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := application.refreshCatalogWith(ctx, fakeCatalogRefresher{err: errors.New("github unavailable")}); err == nil {
		t.Fatal("failed refresh unexpectedly succeeded")
	}
	afterFailure, err := application.Catalog(ctx)
	if err != nil || afterFailure.Validator != beforeFailure.Validator ||
		!afterFailure.RefreshedAt.Equal(beforeFailure.RefreshedAt) || len(afterFailure.Catalog.Assets()) != 1 {
		t.Fatalf("failed refresh changed last-known-good catalog: before=%+v after=%+v err=%v", beforeFailure, afterFailure, err)
	}
	refresher := &capturingCatalogRefresher{result: RefreshResult{NotModified: true}}
	updated, err := application.refreshCatalogWith(ctx, refresher)
	if err != nil || refresher.previous != "v1" || len(updated.Catalog.Assets()) != 1 || !updated.NotModified {
		t.Fatalf("updated=%+v previous=%q err=%v", updated, refresher.previous, err)
	}
}

func TestRefreshCatalogHonorsConfiguredRefreshInterval(t *testing.T) {
	ctx := context.Background()
	database, err := store.Open(ctx, filepath.Join(t.TempDir(), "panel.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	panelSettings := settings.NewService(database, "", settings.Settings{GitHub: settings.GitHub{CatalogRefreshIntervalHours: 12}})
	application := New(database, telemetry.New(database, panelSettings, telemetry.Options{}), panelSettings, Options{})
	now := time.Date(2026, time.August, 27, 8, 0, 0, 0, time.UTC)
	application.now = func() time.Time { return now }
	asset := validCatalogAsset(t)
	value := Catalog{RepositoryID: OfficialRepositoryID, Releases: []Release{{
		ID: asset.ReleaseID, Tag: "v1.13.19", Version: asset.Version, Assets: []Asset{asset},
	}}}
	if _, err := application.refreshCatalogWith(ctx, fakeCatalogRefresher{result: RefreshResult{
		Catalog: value, ETag: "interval-v1",
	}}); err != nil {
		t.Fatal(err)
	}
	application.now = func() time.Time { return now.Add(time.Hour) }
	result, err := application.RefreshCatalog(ctx, CatalogRefreshOptions{})
	if err != nil {
		t.Fatalf("RefreshCatalog: %v", err)
	}
	if !result.NotModified || result.Validator != "interval-v1" {
		t.Fatalf("refresh interval result = %+v", result)
	}
	found := false
	for _, diagnostic := range result.Diagnostics {
		found = found || diagnostic.Code == "refresh_not_due"
	}
	if !found {
		t.Fatalf("refresh interval diagnostic missing: %+v", result.Diagnostics)
	}
}

func (refresher fakeCatalogRefresher) Refresh(context.Context, string) (RefreshResult, error) {
	return refresher.result, refresher.err
}

type capturingCatalogRefresher struct {
	previous string
	result   RefreshResult
}

func (refresher *capturingCatalogRefresher) Refresh(_ context.Context, previous string) (RefreshResult, error) {
	refresher.previous = previous
	return refresher.result, nil
}

type fakeCatalogRefresher struct {
	result RefreshResult
	err    error
}

func validCatalogAsset(t *testing.T) Asset {
	t.Helper()
	version, err := coreartifact.ParseExactVersion("1.13.19")
	if err != nil {
		t.Fatal(err)
	}
	return Asset{
		RepositoryID: OfficialRepositoryID, ReleaseID: 2001, AssetID: 3001,
		Name:        "sing-box-1.13.19-linux-amd64-musl.tar.gz",
		DownloadURL: "https://github.com/SagerNet/sing-box/releases/download/v1.13.19/sing-box-1.13.19-linux-amd64-musl.tar.gz",
		Size:        1234, Version: version, OperatingSystem: coreartifact.OperatingSystemLinux,
		Architecture: coreartifact.ArchitectureAMD64, Variant: coreartifact.VariantMusl,
		APIDigest: testutil.Digest(t, "cd"), HasAPIDigest: true,
	}
}

func TestCatalogSnapshotRoundTripAndFiltering(t *testing.T) {
	ctx := context.Background()
	database, err := store.Open(ctx, filepath.Join(t.TempDir(), "panel.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	application := New(database, nil, nil, Options{})
	now := time.Date(2026, time.August, 26, 15, 0, 0, 0, time.UTC)
	application.now = func() time.Time { return now }
	asset := validCatalogAsset(t)
	refresher := fakeCatalogRefresher{result: RefreshResult{
		Catalog: Catalog{RepositoryID: OfficialRepositoryID, Releases: []Release{{
			ID: asset.ReleaseID, Tag: "v" + asset.Version.String(), Version: asset.Version, Assets: []Asset{asset},
		}}},
		ETag: "opaque-validator", Diagnostics: []Diagnostic{{Step: StepFilter, Severity: DiagnosticInfo, Code: "ok", Message: "catalog ready"}},
	}}
	snapshot, err := application.refreshCatalogWith(ctx, refresher)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Validator != "opaque-validator" || !snapshot.RefreshedAt.Equal(now) {
		t.Fatalf("snapshot=%+v", snapshot)
	}
	loaded, err := application.Catalog(ctx)
	if err != nil || len(loaded.Catalog.Assets()) != 1 || loaded.Catalog.Assets()[0].AssetID != asset.AssetID {
		t.Fatalf("loaded catalog=%+v err=%v", loaded, err)
	}
	filtered, err := application.ListCatalogAssets(ctx, CatalogAssetFilter{ExactVersion: "1.13.19", Architecture: "amd64"})
	if err != nil || len(filtered.Assets) != 1 {
		t.Fatalf("filtered=%+v err=%v", filtered, err)
	}
}
