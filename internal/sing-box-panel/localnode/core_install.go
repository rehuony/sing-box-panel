// SPDX-License-Identifier: GPL-3.0-or-later

package localnode

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"

	"github.com/rehuony/sing-box-panel/internal/coreartifact"
	"github.com/rehuony/sing-box-panel/internal/sing-box-node/artifacts"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/corecache"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/store"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/telemetry"
)

func (application *Node) InstallCore(ctx context.Context, assetID int64) (result corecache.CoreArtifact, operationErr error) {
	defer func() {
		application.telemetry.RecordOperation(ctx, "core.install", "Core installation", operationErr, telemetry.OperationLogContext{})
	}()
	asset, err := application.cores.CatalogAsset(ctx, assetID)
	if err != nil {
		return corecache.CoreArtifact{}, err
	}
	installer, err := application.coreInstaller()
	if err != nil {
		return corecache.CoreArtifact{}, err
	}
	if err := asset.Validate(); err != nil {
		return corecache.CoreArtifact{}, &artifacts.Failure{Step: artifacts.StepPrepare, Code: "invalid_candidate", Cause: errors.Join(artifacts.ErrArtifactStore, err)}
	}
	installed, err := installer.InstallOfficial(ctx, artifacts.OfficialArchive{
		RepositoryID: asset.RepositoryID, ReleaseID: asset.ReleaseID, AssetID: asset.AssetID,
		DownloadURL: asset.DownloadURL, Size: asset.Size, Version: asset.Version,
		Architecture: asset.Architecture, Variant: asset.Variant,
	})
	if err != nil {
		return corecache.CoreArtifact{}, err
	}
	return application.PersistInstalledCore(ctx, installed)
}
func (application *Node) ImportCore(ctx context.Context, input CoreImportRequest) (result corecache.CoreArtifact, operationErr error) {
	defer func() {
		application.telemetry.RecordOperation(ctx, "core.import", "Core import", operationErr, telemetry.OperationLogContext{})
	}()
	if !filepath.IsAbs(input.SourcePath) || filepath.Clean(input.SourcePath) != input.SourcePath {
		return corecache.CoreArtifact{}, errors.New("core import path must be absolute and clean")
	}
	if input.DeleteSource {
		if err := application.cores.ValidateStagedUpload(input.SourcePath); err != nil {
			return corecache.CoreArtifact{}, err
		}
		defer func() {
			if err := application.cores.RemoveStagedUpload(input.SourcePath); err != nil {
				application.cores.RecordUploadCleanupWarning(ctx, "core_upload.cleanup_failed", "The imported upload could not be removed", map[string]any{})
			}
		}()
	}
	version, err := coreartifact.ParseExactVersion(input.ExactVersion)
	if err != nil || version.IsZero() {
		return corecache.CoreArtifact{}, errors.New("core import exact version is invalid")
	}
	variant := coreartifact.Variant(input.Variant)
	if variant == "" {
		variant = coreartifact.VariantMusl
	}
	_, err = coreartifact.NewUserSource(input.SourceDescription)
	if err != nil {
		return corecache.CoreArtifact{}, err
	}
	if variant != coreartifact.VariantMusl || (input.Architecture != "amd64" && input.Architecture != "arm64") {
		return corecache.CoreArtifact{}, errors.New("core import requires Linux amd64 or arm64 musl")
	}
	installer, err := application.coreInstaller()
	if err != nil {
		return corecache.CoreArtifact{}, err
	}
	installed, err := installer.ImportLocal(ctx, artifacts.ImportRequest{SourcePath: input.SourcePath, SourceDescription: input.SourceDescription, ExpectedVersion: version, ExpectedArchitecture: coreartifact.Architecture(input.Architecture), Variant: variant})
	if err != nil {
		return corecache.CoreArtifact{}, err
	}
	return application.PersistInstalledCore(ctx, installed)
}
func (application *Node) SetArtifactInstaller(installer ArtifactInstaller) {
	application.artifacts = installer
}
func (application *Node) coreInstaller() (ArtifactInstaller, error) {
	if application.artifacts != nil {
		return application.artifacts, nil
	}
	if !filepath.IsAbs(application.settings.DataDir) {
		return nil, errors.New("panel data directory is unavailable")
	}
	return artifacts.New(artifacts.Options{Root: filepath.Join(application.settings.DataDir, "artifacts")})
}
func (application *Node) RemoveCoreArtifact(ctx context.Context, artifactID string) error {
	return application.database.RemoveCoreArtifact(ctx, artifactID)
}
func (application *Node) PersistInstalledCore(ctx context.Context, result artifacts.Result) (corecache.CoreArtifact, error) {
	featureFingerprint, err := result.FeatureFingerprint.CanonicalJSON()
	if err != nil {
		return corecache.CoreArtifact{}, fmt.Errorf("normalize installed core feature fingerprint: %w", err)
	}
	identityJSON, err := json.Marshal(result.Identity)
	if err != nil {
		return corecache.CoreArtifact{}, err
	}
	id := sha256.Sum256(identityJSON)
	source := result.Identity.Source()
	persisted := store.CoreArtifact{
		ID: "core_" + hex.EncodeToString(id[:]), ExactVersion: result.Identity.ReportedVersion().String(),
		OperatingSystem: string(result.Identity.OperatingSystem()), Architecture: string(result.Identity.Architecture()),
		Variant: string(result.Identity.Variant()), ArchiveSHA256: result.Identity.Digest().String(),
		BinarySHA256: result.BinarySHA256.String(), BinaryPath: result.BinaryPath,
		ReportedVersion: result.Identity.ReportedVersion().String(), FeatureFingerprint: featureFingerprint,
		CreatedAt: application.now().UTC(),
	}
	switch source.Kind() {
	case coreartifact.SourceOfficial:
		persisted.SourceKind = store.CoreArtifactSourceOfficial
		persisted.RepositoryID = source.RepositoryID()
		persisted.ReleaseID = source.ReleaseID()
		persisted.AssetID = source.AssetID()
	case coreartifact.SourceUser:
		persisted.SourceKind = store.CoreArtifactSourceUserVerified
		persisted.UserSource = source.UserSource()
	default:
		return corecache.CoreArtifact{}, errors.New("installed core has an unknown source kind")
	}
	stored, err := application.database.UpsertCoreArtifact(ctx, persisted)
	if err != nil {
		return corecache.CoreArtifact{}, err
	}
	return corecache.CoreArtifactFromStore(stored), nil
}

type ArtifactInstaller interface {
	InstallOfficial(context.Context, artifacts.OfficialArchive) (artifacts.Result, error)
	ImportLocal(context.Context, artifacts.ImportRequest) (artifacts.Result, error)
}
type CoreImportRequest struct {
	SourcePath        string
	SourceDescription string
	ExactVersion      string
	Architecture      string
	Variant           string
	DeleteSource      bool
}
