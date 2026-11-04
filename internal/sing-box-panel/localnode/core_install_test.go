// SPDX-License-Identifier: GPL-3.0-or-later

package localnode

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/rehuony/sing-box-panel/internal/coreartifact"
	"github.com/rehuony/sing-box-panel/internal/sing-box-node/artifacts"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/corecache"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/settings"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/store"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/telemetry"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/testutil"
)

func TestCatalogInstallationIsIdempotent(t *testing.T) {
	ctx := context.Background()
	database, err := store.Open(ctx, filepath.Join(t.TempDir(), "panel.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	application := newApplication(database)
	asset := validCatalogAsset(t)
	catalogJSON, err := json.Marshal(corecache.Catalog{RepositoryID: corecache.OfficialRepositoryID, Releases: []corecache.Release{{
		ID: asset.ReleaseID, Tag: "v" + asset.Version.String(), Version: asset.Version, Assets: []corecache.Asset{asset},
	}}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.SaveCatalogState(ctx, store.CatalogState{Validator: "v1", Catalog: catalogJSON, Diagnostics: json.RawMessage(`[]`), RefreshedAt: time.Now().UTC()}); err != nil {
		t.Fatal(err)
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
	application.SetArtifactInstaller(fakeArtifactInstaller{installResult: artifacts.Result{Identity: identity, BinarySHA256: testutil.Digest(t, "bc"), BinaryPath: "/secure/artifacts/sing-box"}})
	installed, err := application.InstallCore(ctx, asset.AssetID)
	if err != nil || installed.AssetID != asset.AssetID {
		t.Fatalf("installed=%+v err=%v", installed, err)
	}
	retry, err := application.InstallCore(ctx, asset.AssetID)
	if err != nil || retry.ID != installed.ID {
		t.Fatalf("repeat installation=%+v err=%v", retry, err)
	}
	if string(installed.FeatureFingerprint) != `{"status":"not_reported"}` {
		t.Fatalf("feature fingerprint = %s, want explicit not_reported", installed.FeatureFingerprint)
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
	application.settings = settings.Defaults()
	application.settings.DataDir = t.TempDir()
	application.rebind()
	if err := os.MkdirAll(filepath.Join(application.settings.DataDir, "imports"), 0o700); err != nil {
		t.Fatal(err)
	}
	digest := testutil.Digest(t, "ab")
	version, _ := coreartifact.ParseExactVersion("1.13.19")
	source, _ := coreartifact.NewUserSource("administrator verified local archive")
	identity, err := coreartifact.NewIdentity(
		source, digest, coreartifact.OperatingSystemLinux, coreartifact.ArchitectureAMD64,
		coreartifact.VariantMusl, version,
	)
	if err != nil {
		t.Fatal(err)
	}
	result := artifacts.Result{
		Identity: identity, BinarySHA256: testutil.Digest(t, "cd"),
		BinaryPath: "/secure/store/sing-box", ArchivePath: "/secure/store/archive.tar.gz",
		FeatureFingerprint: artifacts.FeatureFingerprint{
			Status:   artifacts.FeatureFingerprintReported,
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
	if _, err := application.ImportCore(ctx, CoreImportRequest{
		SourcePath: "relative.tar.gz", SourceDescription: "admin",
		ExactVersion: version.String(), Architecture: "amd64", Variant: "musl",
	}); err == nil {
		t.Fatal("core import accepted a relative path")
	}
	firstUploadPath := filepath.Join(application.settings.DataDir, "imports", "core-upload-one")
	secondUploadPath := filepath.Join(application.settings.DataDir, "imports", "core-upload-two")
	if err := os.WriteFile(firstUploadPath, []byte("one"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(secondUploadPath, []byte("two"), 0o600); err != nil {
		t.Fatal(err)
	}
	application.SetArtifactInstaller(fakeArtifactInstaller{importResult: result})
	firstUpload, err := application.ImportCore(ctx, CoreImportRequest{
		SourcePath: firstUploadPath, SourceDescription: "browser upload",
		ExactVersion: version.String(), Architecture: "amd64", Variant: "musl", DeleteSource: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	secondUpload, err := application.ImportCore(ctx, CoreImportRequest{
		SourcePath: secondUploadPath, SourceDescription: "browser upload",
		ExactVersion: version.String(), Architecture: "amd64", Variant: "musl", DeleteSource: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if firstUpload.ID != secondUpload.ID {
		t.Fatalf("same artifact changed identity across staged uploads: first=%+v second=%+v", firstUpload, secondUpload)
	}
	testutil.AssertPathMissing(t, firstUploadPath)
	testutil.AssertPathMissing(t, secondUploadPath)
}

func TestDirectCoreImportCleansStagedFilesOnSuccessFailureAndCancellation(t *testing.T) {
	for _, scenario := range []string{"success", "failed", "canceled", "cleanup-failed"} {
		t.Run(scenario, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			app, _, directory := newCoreUploadTestApplication(t, context.Background())
			path := testutil.WriteCoreUpload(t, directory, "core-upload-import")
			digest := testutil.Digest(t, "ab")
			version, _ := coreartifact.ParseExactVersion("1.13.19")
			source, _ := coreartifact.NewUserSource("browser upload")
			identity, err := coreartifact.NewIdentity(source, digest, coreartifact.OperatingSystemLinux, coreartifact.ArchitectureAMD64, coreartifact.VariantMusl, version)
			if err != nil {
				t.Fatal(err)
			}
			installer := fakeArtifactInstaller{importResult: artifacts.Result{Identity: identity, BinarySHA256: testutil.Digest(t, "cd"), BinaryPath: "/secure/artifacts/sing-box"}}
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
			_, err = app.ImportCore(ctx, CoreImportRequest{SourcePath: path, SourceDescription: "browser upload", ExactVersion: version.String(), Architecture: "amd64", Variant: "musl", DeleteSource: true})
			if (scenario == "success" || scenario == "cleanup-failed") && err != nil {
				t.Fatal(err)
			}
			if (scenario == "failed" || scenario == "canceled") && err == nil {
				t.Fatal("failed import succeeded")
			}
			if scenario == "cleanup-failed" {
				testutil.AssertPathExists(t, path)
				logs, err := app.telemetry.ListLogs(context.Background(), telemetry.LogListRequest{Code: "core_upload.cleanup_failed"})
				if err != nil || len(logs.Items) != 1 || logs.Items[0].Level != store.LogLevelWarn {
					t.Fatalf("cleanup warning: %+v %v", logs, err)
				}
			} else {
				testutil.AssertPathMissing(t, path)
			}
		})
	}
}

type fakeArtifactInstaller struct {
	installResult artifacts.Result
	importResult  artifacts.Result
	err           error
}

func (installer fakeArtifactInstaller) InstallOfficial(context.Context, artifacts.OfficialArchive) (artifacts.Result, error) {
	return installer.installResult, installer.err
}
func (installer fakeArtifactInstaller) ImportLocal(context.Context, artifacts.ImportRequest) (artifacts.Result, error) {
	return installer.importResult, installer.err
}

func validCatalogAsset(t *testing.T) corecache.Asset {
	t.Helper()
	version, err := coreartifact.ParseExactVersion("1.13.19")
	if err != nil {
		t.Fatal(err)
	}
	return corecache.Asset{
		RepositoryID: corecache.OfficialRepositoryID, ReleaseID: 2001, AssetID: 3001,
		Name:        "sing-box-1.13.19-linux-amd64-musl.tar.gz",
		DownloadURL: "https://github.com/SagerNet/sing-box/releases/download/v1.13.19/sing-box-1.13.19-linux-amd64-musl.tar.gz",
		Size:        1234, Version: version, OperatingSystem: coreartifact.OperatingSystemLinux,
		Architecture: coreartifact.ArchitectureAMD64, Variant: coreartifact.VariantMusl,
		APIDigest: testutil.Digest(t, "cd"), HasAPIDigest: true,
	}
}

func newCoreUploadTestApplication(
	t *testing.T,
	ctx context.Context,
) (*Node, *store.Store, string) {
	t.Helper()
	dataDirectory := t.TempDir()
	database, err := store.Open(ctx, filepath.Join(dataDirectory, "panel.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	application := newApplication(database)
	application.settings.DataDir = dataDirectory
	application.rebind()
	application.rebind()
	directory := filepath.Join(application.settings.DataDir, "imports")
	if err := os.MkdirAll(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	return application, database, directory
}
