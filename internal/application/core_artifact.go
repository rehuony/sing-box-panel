// SPDX-License-Identifier: GPL-3.0-or-later

package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/rehuony/sing-box-panel/internal/artifactstore"
	"github.com/rehuony/sing-box-panel/internal/catalog"
	"github.com/rehuony/sing-box-panel/internal/coreartifact"
	"github.com/rehuony/sing-box-panel/internal/store"
)

func (application *Application) InstallCore(ctx context.Context, assetID int64) (result CoreArtifact, operationErr error) {
	defer func() {
		application.RecordOperation(ctx, "core.install", "Core installation", operationErr, OperationLogContext{})
	}()
	asset, err := application.catalogAsset(ctx, assetID)
	if err != nil {
		return CoreArtifact{}, err
	}
	installer, err := application.coreInstaller()
	if err != nil {
		return CoreArtifact{}, err
	}
	installed, err := installer.InstallOfficial(ctx, asset)
	if err != nil {
		return CoreArtifact{}, err
	}
	return application.PersistInstalledCore(ctx, installed)
}

func (application *Application) ImportCore(ctx context.Context, input CoreImportRequest) (result CoreArtifact, operationErr error) {
	defer func() {
		application.RecordOperation(ctx, "core.import", "Core import", operationErr, OperationLogContext{})
	}()
	if !filepath.IsAbs(input.SourcePath) || filepath.Clean(input.SourcePath) != input.SourcePath {
		return CoreArtifact{}, errors.New("core import path must be absolute and clean")
	}
	if input.DeleteSource {
		if err := application.validatePrivateUploadedCoreFile(input.SourcePath); err != nil {
			return CoreArtifact{}, err
		}
		defer func() {
			if err := application.removePrivateUploadedCore(input.SourcePath); err != nil {
				application.recordCoreUploadCleanupWarning(ctx, "core_upload.cleanup_failed", "The imported upload could not be removed", map[string]any{})
			}
		}()
	}
	version, err := coreartifact.ParseExactVersion(input.ExactVersion)
	if err != nil || version.IsZero() {
		return CoreArtifact{}, errors.New("core import exact version is invalid")
	}
	variant := coreartifact.Variant(input.Variant)
	if variant == "" {
		variant = coreartifact.VariantMusl
	}
	_, err = coreartifact.NewUserSource(input.SourceDescription)
	if err != nil {
		return CoreArtifact{}, err
	}
	if variant != coreartifact.VariantMusl || (input.Architecture != "amd64" && input.Architecture != "arm64") {
		return CoreArtifact{}, errors.New("core import requires Linux amd64 or arm64 musl")
	}
	installer, err := application.coreInstaller()
	if err != nil {
		return CoreArtifact{}, err
	}
	installed, err := installer.ImportLocal(ctx, artifactstore.ImportRequest{SourcePath: input.SourcePath, SourceDescription: input.SourceDescription, ExpectedVersion: version, ExpectedArchitecture: coreartifact.Architecture(input.Architecture), Variant: variant})
	if err != nil {
		return CoreArtifact{}, err
	}
	return application.PersistInstalledCore(ctx, installed)
}

func (application *Application) SetArtifactInstaller(installer ArtifactInstaller) {
	application.artifacts = installer
}
func (application *Application) coreInstaller() (ArtifactInstaller, error) {
	if application.artifacts != nil {
		return application.artifacts, nil
	}
	if !filepath.IsAbs(application.settings.DataDir) {
		return nil, errors.New("panel data directory is unavailable")
	}
	return artifactstore.New(artifactstore.Options{Root: filepath.Join(application.settings.DataDir, "artifacts")})
}

func (application *Application) catalogAsset(ctx context.Context, assetID int64) (catalog.Asset, error) {
	if assetID <= 0 {
		return catalog.Asset{}, errors.New("catalog asset id must be positive")
	}
	snapshot, err := application.Catalog(ctx)
	if err != nil {
		return catalog.Asset{}, err
	}
	for _, asset := range snapshot.Catalog.Assets() {
		if asset.AssetID == assetID {
			return asset, nil
		}
	}
	return catalog.Asset{}, errors.New("catalog asset not found")
}

func (application *Application) ListCoreArtifacts(ctx context.Context, filter CoreArtifactListFilter) (CoreArtifactPage, error) {
	var cursor *store.CreatedAtCursor
	if filter.Cursor != nil {
		cursor = &store.CreatedAtCursor{CreatedAt: filter.Cursor.CreatedAt, ID: strings.TrimSpace(filter.Cursor.ID)}
	}
	page, err := application.database.ListCoreArtifacts(ctx, store.CoreArtifactListFilter{
		ExactVersion: filter.ExactVersion, Architecture: filter.Architecture, Variant: filter.Variant,
		SourceKind: filter.SourceKind,
		Cursor:     cursor, Limit: filter.Limit,
	})
	if err != nil {
		return CoreArtifactPage{}, err
	}
	result := CoreArtifactPage{Items: make([]CoreArtifact, len(page.Items))}
	for index, artifact := range page.Items {
		result.Items[index] = coreArtifact(artifact)
	}
	if page.Next != nil {
		result.Next = &CoreArtifactCursor{CreatedAt: page.Next.CreatedAt, ID: page.Next.ID}
	}
	return result, nil
}

func (application *Application) CoreArtifact(ctx context.Context, artifactID string) (CoreArtifact, error) {
	artifact, err := application.database.GetCoreArtifact(ctx, artifactID)
	if err != nil {
		return CoreArtifact{}, err
	}
	return coreArtifact(artifact), nil
}

func (application *Application) RemoveCoreArtifact(ctx context.Context, artifactID string) error {
	return application.database.RemoveCoreArtifact(ctx, artifactID)
}

func (application *Application) PersistInstalledCore(ctx context.Context, result artifactstore.Result) (CoreArtifact, error) {
	featureFingerprint, err := result.FeatureFingerprint.CanonicalJSON()
	if err != nil {
		return CoreArtifact{}, fmt.Errorf("normalize installed core feature fingerprint: %w", err)
	}
	identityJSON, err := json.Marshal(result.Identity)
	if err != nil {
		return CoreArtifact{}, err
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
		return CoreArtifact{}, errors.New("installed core has an unknown source kind")
	}
	stored, err := application.database.UpsertCoreArtifact(ctx, persisted)
	if err != nil {
		return CoreArtifact{}, err
	}
	return coreArtifact(stored), nil
}
