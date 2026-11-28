// SPDX-License-Identifier: GPL-3.0-or-later

package artifacts

import (
	"context"
	"crypto/sha256"
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"

	"github.com/rehuony/sing-box-panel/internal/coreartifact"
)

// Candidate is a core archive the panel verified and streams to this node.
type Candidate struct {
	Source coreartifact.Source
	// SHA256 is the archive digest the panel recorded; the received bytes
	// must match it.
	SHA256       coreartifact.SHA256
	Size         int64
	Version      coreartifact.ExactVersion
	Architecture coreartifact.Architecture
	Variant      coreartifact.Variant
	// Write streams the archive into destination and returns the byte count.
	Write func(ctx context.Context, destination io.Writer) (int64, error)
}

// Install receives, verifies and publishes candidate: the archive must have
// the declared size and digest, hold a safe sing-box binary for the declared
// platform, and that binary must report the declared exact version.
func (store *Store) Install(ctx context.Context, candidate Candidate) (Result, error) {
	if store == nil || ctx == nil || candidate.Write == nil {
		return Result{}, fail(StepPrepare, "invalid_request", nil)
	}
	if candidate.Size <= 0 || candidate.Version.IsZero() || candidate.SHA256.IsZero() {
		return Result{}, fail(StepPrepare, "invalid_candidate", nil)
	}
	if candidate.Size > store.limits.MaximumArchiveBytes {
		return Result{}, fail(StepDownload, "declared_size", ErrTooLarge)
	}
	return store.install(ctx, installRequest{
		source: candidate.Source, expectedSize: candidate.Size, expectedDigest: candidate.SHA256,
		version: candidate.Version, architecture: candidate.Architecture, variant: candidate.Variant,
		writeArchive: candidate.Write,
	})
}

type installRequest struct {
	source         coreartifact.Source
	expectedSize   int64
	expectedDigest coreartifact.SHA256
	version        coreartifact.ExactVersion
	architecture   coreartifact.Architecture
	variant        coreartifact.Variant
	writeArchive   func(context.Context, io.Writer) (int64, error)
}

func (store *Store) install(ctx context.Context, request installRequest) (Result, error) {
	diagnostics := []Diagnostic{{Step: StepPrepare, Code: "identity_resolved", Message: "artifact identity inputs were resolved"}}
	if request.version.IsZero() || request.variant != coreartifact.VariantMusl ||
		(request.architecture != coreartifact.ArchitectureAMD64 && request.architecture != coreartifact.ArchitectureARM64) {
		return Result{}, fail(StepPrepare, "invalid_platform_or_version", coreartifact.ErrInvalidIdentity)
	}
	if err := verifyStoreDirectory(store.root, 0o700); err != nil {
		return Result{}, fail(StepPrepare, "unsafe_root", err)
	}
	if err := verifyTrustedAncestors(store.root); err != nil {
		return Result{}, fail(StepPrepare, "unsafe_root_ancestors", err)
	}
	if err := checkContentDirectory(store.root); err != nil {
		return Result{}, err
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
		return Result{}, fail(StepDownload, "reported_size_mismatch", ErrArchive)
	}
	if bounded.written <= 0 || bounded.written > store.limits.MaximumArchiveBytes {
		return Result{}, fail(StepDownload, "archive_size", ErrTooLarge)
	}
	if request.expectedSize > 0 && bounded.written != request.expectedSize {
		return Result{}, fail(StepDownload, "size_mismatch", ErrArchive)
	}
	diagnostics = append(diagnostics, Diagnostic{Step: StepDownload, Code: "bytes_received", Message: "artifact archive was received within configured limits"})
	var actualSum [32]byte
	copy(actualSum[:], hash.Sum(nil))
	actualDigest := coreartifact.NewSHA256(actualSum)
	if actualDigest != request.expectedDigest {
		return Result{}, fail(StepDownload, "digest_mismatch", ErrArchive)
	}

	identity, err := coreartifact.NewIdentity(request.source, actualDigest, coreartifact.OperatingSystemLinux, request.architecture, request.variant, request.version)
	if err != nil {
		return Result{}, fail(StepPrepare, "identity", err)
	}

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
	featureFingerprint, err := store.inspectBinary(ctx, binaryPath, request.version)
	if err != nil {
		return Result{}, err
	}
	diagnostics = append(diagnostics, Diagnostic{Step: StepVersion, Code: "exact_version_verified", Message: "the real binary reported the requested exact version"})
	publishedPath, publishedArchivePath, err := store.publish(binaryPath, archivePath, actualDigest)
	if err != nil {
		return Result{}, err
	}
	stagedInfo, stagedErr := os.Stat(binaryPath)
	publishedInfo, publishedErr := os.Stat(publishedPath)
	if stagedErr != nil || publishedErr != nil {
		return Result{}, fail(StepPublish, "inspect_published_file", errors.Join(stagedErr, publishedErr))
	}
	if !os.SameFile(stagedInfo, publishedInfo) {
		// Deduplication may reuse an existing installation. Validate that file's
		// platform and version too, without comparing its recorded content hash.
		if err := verifyELF(publishedPath, request.architecture); err != nil {
			return Result{}, err
		}
		featureFingerprint, err = store.inspectBinary(ctx, publishedPath, request.version)
		if err != nil {
			return Result{}, err
		}
	}
	diagnostics = append(diagnostics, Diagnostic{Step: StepPublish, Code: "content_addressed", Message: "binary was atomically published using its recorded archive hash"})
	return Result{
		Identity: identity, BinarySHA256: initialBinaryDigest,
		BinaryPath: publishedPath, ArchivePath: publishedArchivePath,
		FeatureFingerprint: featureFingerprint, Diagnostics: diagnostics,
	}, nil
}

func (store *Store) inspectBinary(ctx context.Context, binaryPath string, version coreartifact.ExactVersion) (FeatureFingerprint, error) {
	executionPath, executionDirectory, err := prepareExecutionCopy(ctx, binaryPath, store.limits.MaximumFileBytes)
	if err != nil {
		return FeatureFingerprint{}, err
	}
	versionContext, cancelVersion := context.WithTimeout(ctx, store.limits.VersionTimeout)
	report, inspectErr := store.inspector.Inspect(versionContext, executionPath, store.limits.MaximumVersionOutput)
	cancelVersion()
	cleanupErr := os.RemoveAll(executionDirectory)
	if inspectErr != nil || cleanupErr != nil {
		return FeatureFingerprint{}, fail(StepVersion, "inspection", errors.Join(inspectErr, cleanupErr))
	}
	if report.Version != version {
		return FeatureFingerprint{}, fail(StepVersion, "mismatch", ErrVersion)
	}
	featureFingerprint, err := report.FeatureFingerprint.normalized()
	if err != nil {
		return FeatureFingerprint{}, fail(StepVersion, "feature_fingerprint", ErrVersion)
	}
	if !slices.Contains(featureFingerprint.Features, "with_musl") {
		return FeatureFingerprint{}, fail(StepVersion, "musl_build_required", ErrVersion)
	}
	return featureFingerprint, nil
}
