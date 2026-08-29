// SPDX-License-Identifier: GPL-3.0-or-later

package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/rehuony/sing-box-panel/internal/artifactstore"
	"github.com/rehuony/sing-box-panel/internal/catalog"
	"github.com/rehuony/sing-box-panel/internal/coreartifact"
	"github.com/rehuony/sing-box-panel/internal/jsonstrict"
	"github.com/rehuony/sing-box-panel/internal/store"
)

func (application *Application) QueueCoreInstall(ctx context.Context, assetID int64) (Task, error) {
	asset, err := application.catalogAsset(ctx, assetID)
	if err != nil {
		return Task{}, err
	}
	digest, err := asset.TrustedDigest()
	if err != nil {
		return Task{}, err
	}
	payload, err := json.Marshal(coreInstallPayload{Asset: asset})
	if err != nil {
		return Task{}, err
	}
	key := fmt.Sprintf("core-install:%d:%d:%d:%s", asset.RepositoryID, asset.ReleaseID, asset.AssetID, digest.String())
	return application.queueMaintenanceTask(ctx, store.TaskKindCoreInstall, payload, key)
}

func (application *Application) QueueCoreImport(ctx context.Context, request CoreImportRequest) (Task, error) {
	if !filepath.IsAbs(request.SourcePath) || filepath.Clean(request.SourcePath) != request.SourcePath {
		return Task{}, errors.New("core import path must be absolute and clean")
	}
	digest, err := coreartifact.ParseSHA256(request.SHA256)
	if err != nil || digest.IsZero() {
		return Task{}, errors.New("core import SHA-256 is invalid")
	}
	version, err := coreartifact.ParseExactVersion(request.ExactVersion)
	if err != nil || version.IsZero() {
		return Task{}, errors.New("core import exact version is invalid")
	}
	architecture := coreartifact.Architecture(request.Architecture)
	if architecture != coreartifact.ArchitectureAMD64 && architecture != coreartifact.ArchitectureARM64 {
		return Task{}, errors.New("core import architecture must be amd64 or arm64")
	}
	variant := coreartifact.Variant(request.Variant)
	if variant == "" {
		variant = coreartifact.VariantPlain
	}
	source, err := coreartifact.NewUserSource(request.SourceDescription)
	if err != nil {
		return Task{}, err
	}
	if _, err := coreartifact.NewIdentity(source, digest, coreartifact.OperatingSystemLinux, architecture, variant, version); err != nil {
		return Task{}, err
	}
	payload, err := json.Marshal(coreImportPayload{
		SourcePath: request.SourcePath, SourceDescription: request.SourceDescription,
		SHA256: digest.String(), ExactVersion: version.String(), Architecture: string(architecture), Variant: string(variant),
		DeleteSource: request.DeleteSource,
	})
	if err != nil {
		return Task{}, err
	}
	key := "core-import:" + digest.String() + ":" + version.String() + ":" + string(architecture) + ":" + string(variant)
	if request.DeleteSource {
		pathDigest := sha256.Sum256([]byte(request.SourcePath))
		key += ":upload:" + hex.EncodeToString(pathDigest[:16])
	}
	return application.queueMaintenanceTask(ctx, store.TaskKindCoreImport, payload, key)
}

func (application *Application) queueMaintenanceTask(ctx context.Context, kind store.TaskKind, payload json.RawMessage, idempotencyKey string) (Task, error) {
	taskID, err := application.newID("task")
	if err != nil {
		return Task{}, err
	}
	queued, err := application.database.EnqueueTask(ctx, store.EnqueueTaskInput{
		ID: taskID, IdempotencyKey: idempotencyKey, Lane: store.TaskLaneMaintenance,
		Kind: kind, Payload: payload, CreatedAt: application.now().UTC(),
	})
	if err != nil {
		return Task{}, err
	}
	return applicationTask(queued), nil
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
		SourceKind: filter.SourceKind, VerificationState: filter.VerificationState,
		Cursor: cursor, Limit: filter.Limit,
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

func (application *Application) RestrictCoreArtifactVerification(ctx context.Context, artifactID string, verificationState store.CoreArtifactVerificationState) (CoreArtifact, error) {
	artifact, err := application.database.RestrictCoreArtifactVerification(ctx, artifactID, verificationState, application.now().UTC())
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
		VerificationState: store.CoreArtifactVerified, CreatedAt: application.now().UTC(),
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
	if stored.VerificationState != store.CoreArtifactVerified {
		return CoreArtifact{}, fmt.Errorf("%w: %s remains %s", ErrCoreArtifactVerificationBlocked, stored.ID, stored.VerificationState)
	}
	return coreArtifact(stored), nil
}

func (application *Application) ExecuteCoreArtifactTask(ctx context.Context, kind store.TaskKind, payload json.RawMessage, installer ArtifactInstaller, beforePersist func(context.Context) error) (CoreArtifact, error) {
	if installer == nil {
		return CoreArtifact{}, errors.New("artifact installer is unavailable")
	}
	var result artifactstore.Result
	switch kind {
	case store.TaskKindCoreInstall:
		var input coreInstallPayload
		if err := jsonstrict.Decode(payload, 128<<10, &input); err != nil {
			return CoreArtifact{}, fmt.Errorf("decode core install task: %w", err)
		}
		if err := input.Asset.Validate(); err != nil {
			return CoreArtifact{}, err
		}
		if _, err := input.Asset.TrustedDigest(); err != nil {
			return CoreArtifact{}, err
		}
		installed, err := installer.InstallOfficial(ctx, input.Asset)
		if err != nil {
			return CoreArtifact{}, err
		}
		result = installed
	case store.TaskKindCoreImport:
		var input coreImportPayload
		if err := jsonstrict.Decode(payload, 128<<10, &input); err != nil {
			return CoreArtifact{}, fmt.Errorf("decode core import task: %w", err)
		}
		if input.DeleteSource {
			if !application.isPrivateUploadedCore(input.SourcePath) {
				return CoreArtifact{}, errors.New("core import task upload path is outside the private staging directory")
			}
			defer func() { _ = os.Remove(input.SourcePath) }()
		}
		digest, err := coreartifact.ParseSHA256(input.SHA256)
		if err != nil || digest.IsZero() {
			return CoreArtifact{}, errors.New("core import task digest is invalid")
		}
		version, err := coreartifact.ParseExactVersion(input.ExactVersion)
		if err != nil || version.IsZero() {
			return CoreArtifact{}, errors.New("core import task version is invalid")
		}
		imported, err := installer.ImportLocal(ctx, artifactstore.ImportRequest{
			SourcePath: input.SourcePath, SourceDescription: input.SourceDescription,
			ExpectedSHA256: digest, ExpectedVersion: version,
			ExpectedArchitecture: coreartifact.Architecture(input.Architecture), Variant: coreartifact.Variant(input.Variant),
		})
		if err != nil {
			return CoreArtifact{}, err
		}
		result = imported
	default:
		return CoreArtifact{}, fmt.Errorf("unsupported core artifact task %q", kind)
	}
	if beforePersist != nil {
		if err := beforePersist(ctx); err != nil {
			return CoreArtifact{}, err
		}
	}
	return application.PersistInstalledCore(ctx, result)
}

func (application *Application) isPrivateUploadedCore(path string) bool {
	if application.settings.DataDir == "" || !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return false
	}
	directory := filepath.Join(application.settings.DataDir, "imports")
	relative, err := filepath.Rel(directory, path)
	return err == nil && relative != "." && !filepath.IsAbs(relative) && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}
