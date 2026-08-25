// SPDX-License-Identifier: GPL-3.0-or-later

package application

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/rehuony/sing-box-panel/internal/artifactstore"
	"github.com/rehuony/sing-box-panel/internal/catalog"
	"github.com/rehuony/sing-box-panel/internal/coreartifact"
	"github.com/rehuony/sing-box-panel/internal/store"
)

func TestCatalogSnapshotRoundTripFilteringAndInstallQueue(t *testing.T) {
	ctx := context.Background()
	database, err := store.Open(ctx, filepath.Join(t.TempDir(), "panel.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	application := newApplication(database)
	now := time.Date(2026, time.August, 26, 15, 0, 0, 0, time.UTC)
	application.now = func() time.Time { return now }
	asset := validCatalogAsset(t)
	refresher := fakeCatalogRefresher{result: catalog.RefreshResult{
		Catalog: catalog.Catalog{RepositoryID: catalog.OfficialRepositoryID, Releases: []catalog.Release{{
			ID: asset.ReleaseID, Tag: "v" + asset.Version.String(), Version: asset.Version, Assets: []catalog.Asset{asset},
		}}},
		ETag: "opaque-validator", Diagnostics: []catalog.Diagnostic{{Step: catalog.StepFilter, Severity: catalog.DiagnosticInfo, Code: "ok", Message: "catalog ready"}},
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
	filtered, err := application.ListCatalogAssets(ctx, CatalogAssetFilter{ExactVersion: "1.13.19", Architecture: "amd64", Installable: true})
	if err != nil || len(filtered.Assets) != 1 {
		t.Fatalf("filtered=%+v err=%v", filtered, err)
	}
	queued, err := application.QueueCoreInstall(ctx, asset.AssetID)
	if err != nil {
		t.Fatal(err)
	}
	retry, err := application.QueueCoreInstall(ctx, asset.AssetID)
	if err != nil || retry.ID != queued.ID {
		t.Fatalf("install retry=%+v err=%v", retry, err)
	}
	if queued.Kind != "core-install" || queued.Status != store.TaskStatusQueued {
		t.Fatalf("queued install=%+v", queued)
	}
	source, err := coreartifact.NewOfficialSource(asset.RepositoryID, asset.ReleaseID, asset.AssetID)
	if err != nil {
		t.Fatal(err)
	}
	identity, err := coreartifact.NewIdentity(
		source, asset.APIDigest, asset.OperatingSystem, asset.Architecture, asset.Variant, asset.Version,
	)
	if err != nil {
		t.Fatal(err)
	}
	checkpoints := 0
	installed, err := application.ExecuteCoreArtifactTask(ctx, queued.Kind, queued.Payload, fakeArtifactInstaller{
		installResult: artifactstore.Result{
			Identity: identity, BinarySHA256: mustDigest(t, "bc"), BinaryPath: "/secure/artifacts/sing-box",
		},
	}, func(context.Context) error {
		checkpoints++
		return nil
	})
	if err != nil || checkpoints != 1 || installed.AssetID != asset.AssetID {
		t.Fatalf("installed=%+v checkpoints=%d err=%v", installed, checkpoints, err)
	}
	if string(installed.FeatureFingerprint) != `{"status":"not_reported"}` {
		t.Fatalf("feature fingerprint = %s, want explicit not_reported", installed.FeatureFingerprint)
	}
}

func TestCatalogNotModifiedRequiresAndReusesLocalSnapshot(t *testing.T) {
	ctx := context.Background()
	database, err := store.Open(ctx, filepath.Join(t.TempDir(), "panel.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	application := newApplication(database)
	if _, err := application.refreshCatalogWith(ctx, fakeCatalogRefresher{result: catalog.RefreshResult{NotModified: true}}); err == nil {
		t.Fatal("not-modified without local state was accepted")
	}
	asset := validCatalogAsset(t)
	initial := catalog.Catalog{RepositoryID: catalog.OfficialRepositoryID, Releases: []catalog.Release{{
		ID: asset.ReleaseID, Tag: "v1.13.19", Version: asset.Version, Assets: []catalog.Asset{asset},
	}}}
	if _, err := application.refreshCatalogWith(ctx, fakeCatalogRefresher{result: catalog.RefreshResult{Catalog: initial, ETag: "v1"}}); err != nil {
		t.Fatal(err)
	}
	refresher := &capturingCatalogRefresher{result: catalog.RefreshResult{NotModified: true}}
	updated, err := application.refreshCatalogWith(ctx, refresher)
	if err != nil || refresher.previous != "v1" || len(updated.Catalog.Assets()) != 1 || !updated.NotModified {
		t.Fatalf("updated=%+v previous=%q err=%v", updated, refresher.previous, err)
	}
}

func TestPersistInstalledCorePreservesFullSourceIdentity(t *testing.T) {
	ctx := context.Background()
	database, err := store.Open(ctx, filepath.Join(t.TempDir(), "panel.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	application := newApplication(database)
	digest := mustDigest(t, "ab")
	version, _ := coreartifact.ParseExactVersion("1.13.19")
	source, _ := coreartifact.NewUserSource("administrator verified local archive")
	identity, err := coreartifact.NewIdentity(
		source, digest, coreartifact.OperatingSystemLinux, coreartifact.ArchitectureAMD64,
		coreartifact.VariantPlain, version,
	)
	if err != nil {
		t.Fatal(err)
	}
	result := artifactstore.Result{
		Identity: identity, BinarySHA256: mustDigest(t, "cd"),
		BinaryPath: "/secure/store/sing-box", ArchivePath: "/secure/store/archive.tar.gz",
		FeatureFingerprint: artifactstore.FeatureFingerprint{
			Status:   artifactstore.FeatureFingerprintReported,
			Features: []string{"with_utls", "with_quic", "with_utls"},
		},
	}
	first, err := application.PersistInstalledCore(ctx, result)
	if err != nil {
		t.Fatal(err)
	}
	second, err := application.PersistInstalledCore(ctx, result)
	if err != nil || second.ID != first.ID || first.UserSource != source.UserSource() || first.SourceKind != store.CoreArtifactSourceUserVerified {
		t.Fatalf("persisted first=%+v second=%+v err=%v", first, second, err)
	}
	if string(first.FeatureFingerprint) != `{"status":"reported","features":["with_quic","with_utls"]}` {
		t.Fatalf("persisted feature fingerprint = %s", first.FeatureFingerprint)
	}
	revoked, err := database.GetCoreArtifact(ctx, first.ID)
	if err != nil {
		t.Fatal(err)
	}
	revoked.VerificationState = store.CoreArtifactRevoked
	if _, err := database.UpsertCoreArtifact(ctx, revoked); err != nil {
		t.Fatal(err)
	}
	if _, err := application.PersistInstalledCore(ctx, result); !errors.Is(err, ErrCoreArtifactVerificationBlocked) {
		t.Fatalf("reinstall revoked artifact error = %v, want ErrCoreArtifactVerificationBlocked", err)
	}
	if _, err := application.QueueCoreImport(ctx, CoreImportRequest{
		SourcePath: "relative.tar.gz", SourceDescription: "admin", SHA256: digest.String(),
		ExactVersion: version.String(), Architecture: "amd64", Variant: "plain",
	}); err == nil {
		t.Fatal("core import accepted a relative path")
	}
}

type fakeCatalogRefresher struct {
	result catalog.RefreshResult
	err    error
}

func (refresher fakeCatalogRefresher) Refresh(context.Context, string) (catalog.RefreshResult, error) {
	return refresher.result, refresher.err
}

type capturingCatalogRefresher struct {
	previous string
	result   catalog.RefreshResult
}

type fakeArtifactInstaller struct {
	installResult artifactstore.Result
	importResult  artifactstore.Result
	err           error
}

func (installer fakeArtifactInstaller) InstallOfficial(context.Context, catalog.Asset) (artifactstore.Result, error) {
	return installer.installResult, installer.err
}

func (installer fakeArtifactInstaller) ImportLocal(context.Context, artifactstore.ImportRequest) (artifactstore.Result, error) {
	return installer.importResult, installer.err
}

func (refresher *capturingCatalogRefresher) Refresh(_ context.Context, previous string) (catalog.RefreshResult, error) {
	refresher.previous = previous
	return refresher.result, nil
}

func validCatalogAsset(t *testing.T) catalog.Asset {
	t.Helper()
	version, err := coreartifact.ParseExactVersion("1.13.19")
	if err != nil {
		t.Fatal(err)
	}
	return catalog.Asset{
		RepositoryID: catalog.OfficialRepositoryID, ReleaseID: 2001, AssetID: 3001,
		Name:        "sing-box-1.13.19-linux-amd64.tar.gz",
		DownloadURL: "https://github.com/SagerNet/sing-box/releases/download/v1.13.19/sing-box-1.13.19-linux-amd64.tar.gz",
		Size:        1234, Version: version, OperatingSystem: coreartifact.OperatingSystemLinux,
		Architecture: coreartifact.ArchitectureAMD64, Variant: coreartifact.VariantPlain,
		APIDigest: mustDigest(t, "cd"), HasAPIDigest: true,
	}
}

func mustDigest(t *testing.T, pair string) coreartifact.SHA256 {
	t.Helper()
	digest, err := coreartifact.ParseSHA256(strings.Repeat(pair, 32))
	if err != nil {
		t.Fatal(err)
	}
	return digest
}
