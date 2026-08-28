// SPDX-License-Identifier: GPL-3.0-or-later

package artifactstore

import (
	"context"
	"crypto/sha256"
	"errors"
	"io"
	"os"
	"path/filepath"

	"github.com/rehuony/sing-box-panel/internal/catalog"
	"github.com/rehuony/sing-box-panel/internal/coreartifact"
)

func (store *Store) InstallOfficial(ctx context.Context, asset catalog.Asset) (Result, error) {
	if store == nil || ctx == nil {
		return Result{}, fail(StepPrepare, "invalid_request", nil)
	}
	if err := asset.Validate(); err != nil {
		return Result{}, fail(StepPrepare, "invalid_candidate", err)
	}
	expectedDigest, err := asset.TrustedDigest()
	if err != nil {
		return Result{}, fail(StepDigest, "untrusted", err)
	}
	if asset.Size > store.limits.MaximumArchiveBytes {
		return Result{}, fail(StepDownload, "declared_size", ErrTooLarge)
	}
	source, err := coreartifact.NewOfficialSource(asset.RepositoryID, asset.ReleaseID, asset.AssetID)
	if err != nil {
		return Result{}, fail(StepPrepare, "source_identity", err)
	}
	return store.install(ctx, installRequest{
		source: source, expectedDigest: expectedDigest, expectedSize: asset.Size,
		version: asset.Version, architecture: asset.Architecture, variant: asset.Variant,
		writeArchive: func(operationContext context.Context, destination io.Writer) (int64, error) {
			return store.downloader.Download(operationContext, asset.DownloadURL, destination, store.limits.MaximumArchiveBytes)
		},
	})
}

func (store *Store) ImportLocal(ctx context.Context, request ImportRequest) (Result, error) {
	if store == nil || ctx == nil || request.ExpectedSHA256.IsZero() {
		return Result{}, fail(StepPrepare, "invalid_request", ErrDigest)
	}
	source, err := coreartifact.NewUserSource(request.SourceDescription)
	if err != nil {
		return Result{}, fail(StepPrepare, "source_identity", err)
	}
	if request.SourcePath == "" || !filepath.IsAbs(request.SourcePath) || filepath.Clean(request.SourcePath) != request.SourcePath {
		return Result{}, fail(StepPrepare, "invalid_source_path", ErrUnsafeSource)
	}
	pathInfo, err := os.Lstat(request.SourcePath)
	if err != nil || !pathInfo.Mode().IsRegular() || pathInfo.Mode()&os.ModeSymlink != 0 {
		return Result{}, fail(StepPrepare, "unsafe_source_file", errors.Join(ErrUnsafeSource, err))
	}
	return store.install(ctx, installRequest{
		source: source, expectedDigest: request.ExpectedSHA256,
		version: request.ExpectedVersion, architecture: request.ExpectedArchitecture, variant: request.Variant,
		writeArchive: func(operationContext context.Context, destination io.Writer) (int64, error) {
			file, err := os.Open(request.SourcePath)
			if err != nil {
				return 0, err
			}
			defer file.Close()
			info, err := file.Stat()
			currentPathInfo, pathErr := os.Lstat(request.SourcePath)
			if err != nil || pathErr != nil {
				return 0, errors.Join(err, pathErr)
			}
			if !info.Mode().IsRegular() || !currentPathInfo.Mode().IsRegular() ||
				currentPathInfo.Mode()&os.ModeSymlink != 0 || !os.SameFile(pathInfo, info) ||
				!os.SameFile(info, currentPathInfo) {
				return 0, ErrUnsafeSource
			}
			if info.Size() > store.limits.MaximumArchiveBytes {
				return 0, ErrTooLarge
			}
			return copyBounded(operationContext, destination, file, store.limits.MaximumArchiveBytes)
		},
	})
}

type installRequest struct {
	source         coreartifact.Source
	expectedDigest coreartifact.SHA256
	expectedSize   int64
	version        coreartifact.ExactVersion
	architecture   coreartifact.Architecture
	variant        coreartifact.Variant
	writeArchive   func(context.Context, io.Writer) (int64, error)
}

func (store *Store) install(ctx context.Context, request installRequest) (Result, error) {
	diagnostics := []Diagnostic{{Step: StepPrepare, Code: "identity_resolved", Message: "artifact identity inputs were resolved"}}
	identity, err := coreartifact.NewIdentity(
		request.source,
		request.expectedDigest,
		coreartifact.OperatingSystemLinux,
		request.architecture,
		request.variant,
		request.version,
	)
	if err != nil {
		return Result{}, fail(StepPrepare, "identity", err)
	}
	if err := verifyStoreDirectory(store.root, 0o700); err != nil {
		return Result{}, fail(StepPrepare, "unsafe_root", err)
	}
	if err := verifyTrustedAncestors(store.root); err != nil {
		return Result{}, fail(StepPrepare, "unsafe_root_ancestors", err)
	}
	if err := ensureStoreDirectory(filepath.Join(store.root, "sha256"), 0o700); err != nil {
		return Result{}, fail(StepPrepare, "unsafe_content_root", err)
	}
	stageDirectory, err := os.MkdirTemp(store.root, ".staging-")
	if err != nil {
		return Result{}, fail(StepPrepare, "staging_create", err)
	}
	defer os.RemoveAll(stageDirectory)
	if err := os.Chmod(stageDirectory, 0o700); err != nil {
		return Result{}, fail(StepPrepare, "staging_mode", err)
	}
	archivePath := filepath.Join(stageDirectory, "artifact.tar.gz")
	archive, err := os.OpenFile(archivePath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return Result{}, fail(StepPrepare, "archive_create", err)
	}
	hash := sha256.New()
	bounded := &boundedArchiveWriter{destination: io.MultiWriter(archive, hash), maximum: store.limits.MaximumArchiveBytes}
	downloadContext, cancelDownload := context.WithTimeout(ctx, store.limits.DownloadTimeout)
	written, writeErr := request.writeArchive(downloadContext, bounded)
	cancelDownload()
	syncErr := archive.Sync()
	closeErr := archive.Close()
	if writeErr != nil || syncErr != nil || closeErr != nil {
		return Result{}, fail(StepDownload, "archive_write", errors.Join(writeErr, syncErr, closeErr))
	}
	if written != bounded.written {
		return Result{}, fail(StepDownload, "reported_size_mismatch", ErrDigest)
	}
	if bounded.written <= 0 || bounded.written > store.limits.MaximumArchiveBytes {
		return Result{}, fail(StepDownload, "archive_size", ErrTooLarge)
	}
	if request.expectedSize > 0 && bounded.written != request.expectedSize {
		return Result{}, fail(StepDownload, "size_mismatch", ErrDigest)
	}
	diagnostics = append(diagnostics, Diagnostic{Step: StepDownload, Code: "bytes_received", Message: "artifact archive was received within configured limits"})
	var actualSum [32]byte
	copy(actualSum[:], hash.Sum(nil))
	actualDigest := coreartifact.NewSHA256(actualSum)
	if actualDigest != request.expectedDigest {
		return Result{}, fail(StepDigest, "mismatch", ErrDigest)
	}
	diagnostics = append(diagnostics, Diagnostic{Step: StepDigest, Code: "sha256_verified", Message: "artifact archive SHA-256 matched trusted evidence"})

	binaryPath := filepath.Join(stageDirectory, "sing-box")
	if err := extractSingBox(ctx, archivePath, binaryPath, store.limits); err != nil {
		return Result{}, err
	}
	diagnostics = append(diagnostics, Diagnostic{Step: StepArchive, Code: "safe_archive", Message: "archive paths, types, counts, and expanded sizes were verified"})
	if err := verifyELF(binaryPath, request.architecture); err != nil {
		return Result{}, err
	}
	initialBinaryDigest, err := digestFile(ctx, binaryPath, store.limits.MaximumFileBytes)
	if err != nil {
		return Result{}, fail(StepELF, "binary_digest", err)
	}
	diagnostics = append(diagnostics, Diagnostic{Step: StepELF, Code: "platform_verified", Message: "binary is a Linux-compatible ELF for the requested architecture"})
	executionPath, executionDirectory, err := prepareExecutionCopy(ctx, binaryPath, initialBinaryDigest, store.limits.MaximumFileBytes)
	if err != nil {
		return Result{}, err
	}
	versionContext, cancelVersion := context.WithTimeout(ctx, store.limits.VersionTimeout)
	report, inspectErr := store.inspector.Inspect(versionContext, executionPath, store.limits.MaximumVersionOutput)
	cancelVersion()
	cleanupErr := os.RemoveAll(executionDirectory)
	if inspectErr != nil || cleanupErr != nil {
		return Result{}, fail(StepVersion, "inspection", errors.Join(inspectErr, cleanupErr))
	}
	if report.Version != request.version {
		return Result{}, fail(StepVersion, "mismatch", ErrVersion)
	}
	featureFingerprint, err := report.FeatureFingerprint.normalized()
	if err != nil {
		return Result{}, fail(StepVersion, "feature_fingerprint", ErrVersion)
	}
	diagnostics = append(diagnostics, Diagnostic{Step: StepVersion, Code: "exact_version_verified", Message: "the real binary reported the requested exact version"})
	if err := verifyFileDigest(ctx, archivePath, request.expectedDigest, store.limits.MaximumArchiveBytes); err != nil {
		return Result{}, fail(StepDigest, "post_execution_mismatch", err)
	}
	verifiedBinaryPath := filepath.Join(stageDirectory, "verified-sing-box")
	if err := extractSingBox(ctx, archivePath, verifiedBinaryPath, store.limits); err != nil {
		return Result{}, err
	}
	if err := verifyELF(verifiedBinaryPath, request.architecture); err != nil {
		return Result{}, err
	}
	verifiedBinaryDigest, err := digestFile(ctx, verifiedBinaryPath, store.limits.MaximumFileBytes)
	if err != nil {
		return Result{}, fail(StepELF, "verified_binary_digest", err)
	}
	if verifiedBinaryDigest != initialBinaryDigest {
		return Result{}, fail(StepDigest, "binary_changed", ErrDigest)
	}
	if err := verifyFileDigest(ctx, archivePath, request.expectedDigest, store.limits.MaximumArchiveBytes); err != nil {
		return Result{}, fail(StepDigest, "pre_publish_mismatch", err)
	}
	publishedPath, publishedArchivePath, err := store.publish(verifiedBinaryPath, archivePath, request.expectedDigest)
	if err != nil {
		return Result{}, err
	}
	diagnostics = append(diagnostics, Diagnostic{Step: StepPublish, Code: "content_addressed", Message: "verified binary was atomically published by archive digest"})
	return Result{
		Identity: identity, BinarySHA256: verifiedBinaryDigest,
		BinaryPath: publishedPath, ArchivePath: publishedArchivePath,
		FeatureFingerprint: featureFingerprint, Diagnostics: diagnostics,
	}, nil
}
