// SPDX-License-Identifier: GPL-3.0-or-later

package corecache

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/rehuony/sing-box-panel/internal/atomicfile"
	"github.com/rehuony/sing-box-panel/internal/coreartifact"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/store"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/telemetry"
)

// MaximumArchiveBytes bounds a cached core archive.
const MaximumArchiveBytes = 128 << 20

var ErrArchiveDigest = errors.New("the downloaded archive does not match the digest GitHub reports")

// CoreUpload describes an administrator-verified archive uploaded to the
// panel. The node that installs it checks that the binary reports the
// declared exact version.
type CoreUpload struct {
	SourceDescription string
	ExactVersion      string
	Architecture      string
	Variant           string
}

// Downloader fetches an official release asset.
type Downloader interface {
	Download(ctx context.Context, rawURL string, destination io.Writer, maximumBytes int64) (int64, error)
}

// CacheOfficial downloads a catalog asset into the cache, verifying its
// declared size and, when GitHub reports one, its digest. A cached asset is
// not downloaded again. The download runs to its own deadline even when the
// caller stops waiting, so an install the browser gave up on still leaves
// its archive cached for the next attempt.
func (application *Service) CacheOfficial(ctx context.Context, assetID int64) (result store.CachedArtifact, operationErr error) {
	ctx = context.WithoutCancel(ctx)
	startedAt := time.Now()
	defer func() {
		application.telemetry.RecordOperation(ctx, "core.cache", "Core download", operationErr, telemetry.OperationLogContext{CoreID: result.ID, StartedAt: startedAt})
	}()
	asset, err := application.CatalogAsset(ctx, assetID)
	if err != nil {
		return store.CachedArtifact{}, err
	}
	if err := asset.Validate(); err != nil {
		return store.CachedArtifact{}, err
	}
	source, err := coreartifact.NewOfficialSource(asset.RepositoryID, asset.ReleaseID, asset.AssetID)
	if err != nil {
		return store.CachedArtifact{}, err
	}
	if cached, reused, err := application.reuseOfficial(ctx, source, asset); err != nil || reused {
		return cached, err
	}
	return application.storeArchive(func(destination io.Writer) (int64, error) {
		downloader, err := application.archiveDownloader()
		if err != nil {
			return 0, err
		}
		return downloader.Download(ctx, asset.DownloadURL, destination, MaximumArchiveBytes)
	}, asset.Size, func(digest coreartifact.SHA256) (store.CachedArtifact, error) {
		if asset.HasAPIDigest && digest != asset.APIDigest {
			return store.CachedArtifact{}, ErrArchiveDigest
		}
		return application.record(ctx, source, digest, asset.Size, asset.Architecture, asset.Variant, asset.Version)
	})
}

// reuseOfficial marks and returns the cached copy of an official asset when
// its archive is still on disk with the recorded size and digest. A missing or
// altered archive is not reused, so the caller downloads it again.
func (application *Service) reuseOfficial(ctx context.Context, source coreartifact.Source, asset Asset) (store.CachedArtifact, bool, error) {
	existing, err := application.database.OfficialCachedArtifact(ctx, source.RepositoryID(), source.ReleaseID(), source.AssetID())
	if errors.Is(err, store.ErrCachedArtifactNotFound) {
		return store.CachedArtifact{}, false, nil
	}
	if err != nil {
		return store.CachedArtifact{}, false, err
	}
	application.cacheMu.Lock()
	defer application.cacheMu.Unlock()
	digest, err := archiveDigest(application.archivePath(existing.SHA256), asset.Size)
	if err != nil || digest.String() != existing.SHA256 || (asset.HasAPIDigest && digest != asset.APIDigest) {
		return store.CachedArtifact{}, false, nil
	}
	cached, err := application.record(ctx, source, digest, asset.Size, asset.Architecture, asset.Variant, asset.Version)
	return cached, err == nil, err
}

// archiveDigest hashes the archive at path, which must hold exactly size bytes.
func archiveDigest(path string, size int64) (coreartifact.SHA256, error) {
	file, err := os.Open(path)
	if err != nil {
		return coreartifact.SHA256{}, err
	}
	defer file.Close()
	hash := sha256.New()
	written, err := io.Copy(hash, io.LimitReader(file, size+1))
	if err != nil {
		return coreartifact.SHA256{}, err
	}
	if written != size {
		return coreartifact.SHA256{}, fmt.Errorf("cached core archive holds %d bytes, not %d", written, size)
	}
	var sum [32]byte
	copy(sum[:], hash.Sum(nil))
	return coreartifact.NewSHA256(sum), nil
}

// CacheUpload moves a staged upload into the cache as a user-verified
// artifact and removes the staged file.
func (application *Service) CacheUpload(ctx context.Context, stagedPath string, upload CoreUpload) (result store.CachedArtifact, operationErr error) {
	defer func() {
		application.telemetry.RecordOperation(ctx, "core.upload", "Core upload", operationErr, telemetry.OperationLogContext{CoreID: result.ID})
	}()
	if err := application.ValidateStagedUpload(stagedPath); err != nil {
		return store.CachedArtifact{}, err
	}
	defer func() {
		if err := application.RemoveStagedUpload(stagedPath); err != nil {
			application.RecordUploadCleanupWarning(ctx, "core_upload.cleanup_failed", "The uploaded core archive could not be removed", map[string]any{})
		}
	}()
	version, err := coreartifact.ParseExactVersion(upload.ExactVersion)
	if err != nil || version.IsZero() {
		return store.CachedArtifact{}, errors.New("core upload exact version is invalid")
	}
	variant := coreartifact.Variant(upload.Variant)
	if variant == "" {
		variant = coreartifact.VariantMusl
	}
	architecture := coreartifact.Architecture(upload.Architecture)
	if variant != coreartifact.VariantMusl || (architecture != coreartifact.ArchitectureAMD64 && architecture != coreartifact.ArchitectureARM64) {
		return store.CachedArtifact{}, errors.New("core upload requires Linux amd64 or arm64 musl")
	}
	source, err := coreartifact.NewUserSource(upload.SourceDescription)
	if err != nil {
		return store.CachedArtifact{}, err
	}
	info, err := os.Stat(stagedPath)
	if err != nil {
		return store.CachedArtifact{}, err
	}
	return application.storeArchive(func(destination io.Writer) (int64, error) {
		file, err := os.Open(stagedPath)
		if err != nil {
			return 0, err
		}
		defer file.Close()
		return io.Copy(destination, io.LimitReader(file, MaximumArchiveBytes+1))
	}, info.Size(), func(digest coreartifact.SHA256) (store.CachedArtifact, error) {
		return application.record(ctx, source, digest, info.Size(), architecture, variant, version)
	})
}

// CachedArtifact returns one cached artifact.
func (application *Service) CachedArtifact(ctx context.Context, id string) (store.CachedArtifact, error) {
	return application.database.CachedArtifact(ctx, strings.TrimSpace(id))
}

// OpenCachedArtifact opens the bytes of a cached artifact.
func (application *Service) OpenCachedArtifact(ctx context.Context, id string) (*os.File, store.CachedArtifact, error) {
	artifact, err := application.CachedArtifact(ctx, id)
	if err != nil {
		return nil, store.CachedArtifact{}, err
	}
	file, err := os.Open(application.archivePath(artifact.SHA256))
	if err != nil {
		return nil, store.CachedArtifact{}, fmt.Errorf("open cached core archive: %w", err)
	}
	return file, artifact, nil
}

// Identity returns the artifact identity nodes verify an installed copy
// against.
func Identity(artifact store.CachedArtifact) (coreartifact.Identity, error) {
	var source coreartifact.Source
	var err error
	if artifact.SourceKind == store.CachedArtifactOfficial {
		source, err = coreartifact.NewOfficialSource(artifact.RepositoryID, artifact.ReleaseID, artifact.AssetID)
	} else {
		source, err = coreartifact.NewUserSource(artifact.UserSource)
	}
	if err != nil {
		return coreartifact.Identity{}, err
	}
	digest, err := coreartifact.ParseSHA256(artifact.SHA256)
	if err != nil {
		return coreartifact.Identity{}, err
	}
	version, err := coreartifact.ParseExactVersion(artifact.ExactVersion)
	if err != nil {
		return coreartifact.Identity{}, err
	}
	return coreartifact.NewIdentity(source, digest, coreartifact.OperatingSystem(artifact.OS),
		coreartifact.Architecture(artifact.Arch), coreartifact.Variant(artifact.Variant), version)
}

// RemoveCachedArtifact removes a cached archive and, unless another record
// uses the same bytes, its file.
func (application *Service) RemoveCachedArtifact(ctx context.Context, id string) error {
	application.cacheMu.Lock()
	defer application.cacheMu.Unlock()
	artifact, err := application.database.CachedArtifact(ctx, id)
	if err != nil {
		return err
	}
	shared, err := application.database.DeleteCachedArtifact(ctx, id)
	if err != nil || shared {
		return err
	}
	if err := application.removeFile(application.archivePath(artifact.SHA256)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

// OfficialArtifactsUnusedSince lists the official archives no install asked
// for since before.
func (application *Service) OfficialArtifactsUnusedSince(ctx context.Context, before time.Time) ([]store.CachedArtifact, error) {
	return application.database.OfficialArtifactsUnusedSince(ctx, before)
}

func (application *Service) record(ctx context.Context, source coreartifact.Source, digest coreartifact.SHA256, size int64,
	architecture coreartifact.Architecture, variant coreartifact.Variant, version coreartifact.ExactVersion) (store.CachedArtifact, error) {
	identity, err := coreartifact.NewIdentity(source, digest, coreartifact.OperatingSystemLinux, architecture, variant, version)
	if err != nil {
		return store.CachedArtifact{}, err
	}
	id, err := identity.ID()
	if err != nil {
		return store.CachedArtifact{}, err
	}
	artifact := store.CachedArtifact{
		ID: id, SHA256: digest.String(), Size: size, OS: string(coreartifact.OperatingSystemLinux),
		Arch: string(architecture), Variant: string(variant), ExactVersion: version.String(), CreatedAt: application.now().UTC(),
	}
	if source.Kind() == coreartifact.SourceOfficial {
		artifact.SourceKind = store.CachedArtifactOfficial
		artifact.RepositoryID, artifact.ReleaseID, artifact.AssetID = source.RepositoryID(), source.ReleaseID(), source.AssetID()
	} else {
		artifact.SourceKind, artifact.UserSource = store.CachedArtifactUserVerified, source.UserSource()
	}
	return application.database.RecordCachedArtifact(ctx, artifact)
}

// storeArchive writes an archive to the cache under its digest after
// checking its size, and records it. Placing the file and recording it hold
// cacheMu, so a sweep cannot remove the file of a record being written.
func (application *Service) storeArchive(write func(io.Writer) (int64, error), expectedSize int64,
	record func(coreartifact.SHA256) (store.CachedArtifact, error),
) (store.CachedArtifact, error) {
	directory, err := application.cacheDirectory()
	if err != nil {
		return store.CachedArtifact{}, err
	}
	temporary, err := os.CreateTemp(directory, ".download-*")
	if err != nil {
		return store.CachedArtifact{}, err
	}
	defer os.Remove(temporary.Name())
	hash := sha256.New()
	written, writeErr := write(io.MultiWriter(temporary, hash))
	syncErr := temporary.Sync()
	closeErr := temporary.Close()
	if err := errors.Join(writeErr, syncErr, closeErr); err != nil {
		return store.CachedArtifact{}, err
	}
	if written <= 0 || written > MaximumArchiveBytes || written != expectedSize {
		return store.CachedArtifact{}, fmt.Errorf("core archive size %d does not match the declared %d bytes", written, expectedSize)
	}
	var sum [32]byte
	copy(sum[:], hash.Sum(nil))
	digest := coreartifact.NewSHA256(sum)
	application.cacheMu.Lock()
	defer application.cacheMu.Unlock()
	if err := os.Rename(temporary.Name(), application.archivePath(digest.String())); err != nil {
		return store.CachedArtifact{}, err
	}
	if err := atomicfile.SyncDir(directory); err != nil {
		return store.CachedArtifact{}, err
	}
	return record(digest)
}

func (application *Service) cacheDirectory() (string, error) {
	if application.dataDir == "" || !filepath.IsAbs(application.dataDir) {
		return "", errors.New("panel data directory is unavailable for the core cache")
	}
	directory := filepath.Join(application.dataDir, "artifact-cache")
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return "", err
	}
	return directory, nil
}

func (application *Service) archivePath(digest string) string {
	return filepath.Join(application.dataDir, "artifact-cache", digest+".tar.gz")
}

func (application *Service) archiveDownloader() (Downloader, error) {
	if application.downloader != nil {
		return application.downloader, nil
	}
	return NewSafeDownloader(SafeDownloaderOptions{})
}
