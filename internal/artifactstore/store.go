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

type Store struct {
	root       string
	downloader Downloader
	inspector  VersionInspector
	limits     Limits
}

func New(options Options) (*Store, error) {
	if options.Root == "" || !filepath.IsAbs(options.Root) || filepath.Clean(options.Root) != options.Root || options.Root == string(filepath.Separator) {
		return nil, fail(StepPrepare, "invalid_root", nil)
	}
	if options.Limits == (Limits{}) {
		options.Limits = DefaultLimits()
	}
	if err := options.Limits.validate(); err != nil {
		return nil, fail(StepPrepare, "invalid_limits", err)
	}
	lexicalParent := filepath.Dir(options.Root)
	if err := verifyTrustedLexicalPath(lexicalParent); err != nil {
		return nil, fail(StepPrepare, "unsafe_root_ancestors", err)
	}
	resolvedParent, err := filepath.EvalSymlinks(lexicalParent)
	if err != nil {
		return nil, fail(StepPrepare, "root_parent_resolve", err)
	}
	options.Root = filepath.Join(resolvedParent, filepath.Base(options.Root))
	if err := os.Mkdir(options.Root, 0o700); err != nil && !errors.Is(err, os.ErrExist) {
		return nil, fail(StepPrepare, "root_create", err)
	}
	if err := verifyTrustedAncestors(options.Root); err != nil {
		return nil, fail(StepPrepare, "unsafe_root_ancestors", err)
	}
	if err := verifyStoreDirectory(options.Root, 0o700); err != nil {
		return nil, fail(StepPrepare, "root_mode", err)
	}
	if options.Downloader == nil {
		options.Downloader, err = NewSafeDownloader(SafeDownloaderOptions{Timeout: options.Limits.DownloadTimeout})
		if err != nil {
			return nil, err
		}
	}
	if options.Inspector == nil {
		options.Inspector = ExecVersionInspector{Timeout: options.Limits.VersionTimeout}
	}
	return &Store{root: options.Root, downloader: options.Downloader, inspector: options.Inspector, limits: options.Limits}, nil
}

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

func prepareExecutionCopy(
	ctx context.Context,
	sourcePath string,
	expectedDigest coreartifact.SHA256,
	maximumBytes int64,
) (string, string, error) {
	lexicalParent := os.TempDir()
	if err := verifyTrustedLexicalPath(lexicalParent); err != nil {
		return "", "", fail(StepVersion, "unsafe_execution_parent", err)
	}
	resolvedParent, err := filepath.EvalSymlinks(lexicalParent)
	if err != nil {
		return "", "", fail(StepVersion, "execution_parent_resolve", err)
	}
	directory, err := os.MkdirTemp(resolvedParent, "sing-box-panel-version-")
	if err != nil {
		return "", "", fail(StepVersion, "execution_directory", err)
	}
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.RemoveAll(directory)
		}
	}()
	if err := verifyTrustedAncestors(directory); err != nil {
		return "", "", fail(StepVersion, "unsafe_execution_directory", err)
	}
	destinationPath := filepath.Join(directory, "sing-box")
	source, err := os.Open(sourcePath)
	if err != nil {
		return "", "", fail(StepVersion, "execution_source", err)
	}
	destination, err := os.OpenFile(destinationPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		source.Close()
		return "", "", fail(StepVersion, "execution_copy", err)
	}
	written, copyErr := copyBounded(ctx, destination, source, maximumBytes)
	modeErr := destination.Chmod(0o700)
	syncErr := destination.Sync()
	closeDestinationErr := destination.Close()
	closeSourceErr := source.Close()
	if copyErr != nil || written <= 0 || modeErr != nil || syncErr != nil || closeDestinationErr != nil || closeSourceErr != nil {
		return "", "", fail(StepVersion, "execution_copy", errors.Join(copyErr, modeErr, syncErr, closeDestinationErr, closeSourceErr))
	}
	if err := verifyFileDigest(ctx, destinationPath, expectedDigest, maximumBytes); err != nil {
		return "", "", fail(StepVersion, "execution_copy_digest", err)
	}
	cleanup = false
	return destinationPath, directory, nil
}

func verifyFileDigest(ctx context.Context, path string, expected coreartifact.SHA256, maximumBytes int64) error {
	actual, err := digestFile(ctx, path, maximumBytes)
	if err != nil {
		return err
	}
	if actual != expected {
		return ErrDigest
	}
	return nil
}

func digestFile(ctx context.Context, path string, maximumBytes int64) (coreartifact.SHA256, error) {
	file, err := os.Open(path)
	if err != nil {
		return coreartifact.SHA256{}, err
	}
	defer file.Close()
	hash := sha256.New()
	written, err := copyBounded(ctx, hash, file, maximumBytes)
	if err != nil || written <= 0 {
		return coreartifact.SHA256{}, errors.Join(err, ErrDigest)
	}
	var sum [32]byte
	copy(sum[:], hash.Sum(nil))
	return coreartifact.NewSHA256(sum), nil
}

func (store *Store) publish(stagedBinaryPath, stagedArchivePath string, digest coreartifact.SHA256) (string, string, error) {
	digestText := digest.String()
	parent := filepath.Join(store.root, "sha256", digestText[:2])
	finalDirectory := filepath.Join(parent, digestText)
	finalBinaryPath := filepath.Join(finalDirectory, "sing-box")
	finalArchivePath := filepath.Join(finalDirectory, "artifact.tar.gz")
	for _, directory := range []string{filepath.Join(store.root, "sha256"), parent, finalDirectory} {
		if err := ensureStoreDirectory(directory, 0o700); err != nil {
			return "", "", fail(StepPublish, "unsafe_directory", err)
		}
	}
	if err := publishFile(stagedArchivePath, finalArchivePath, 0o600); err != nil {
		return "", "", err
	}
	// Persist the archive directory entry before publishing the executable.
	// A crash may leave archive-only state, which an idempotent retry repairs,
	// but a durably visible executable never precedes its source archive.
	if err := syncDirectory(finalDirectory); err != nil {
		return "", "", fail(StepPublish, "sync", err)
	}
	if err := publishFile(stagedBinaryPath, finalBinaryPath, 0o700); err != nil {
		return "", "", err
	}
	if err := syncDirectory(finalDirectory); err != nil {
		return "", "", fail(StepPublish, "sync", err)
	}
	if err := syncDirectory(parent); err != nil {
		return "", "", fail(StepPublish, "sync", err)
	}
	if err := syncDirectory(filepath.Join(store.root, "sha256")); err != nil {
		return "", "", fail(StepPublish, "sync", err)
	}
	if err := syncDirectory(store.root); err != nil {
		return "", "", fail(StepPublish, "sync", err)
	}
	return finalBinaryPath, finalArchivePath, nil
}

func ensureStoreDirectory(path string, mode os.FileMode) error {
	if err := os.Mkdir(path, mode); err != nil && !errors.Is(err, os.ErrExist) {
		return errors.Join(ErrCorruptStore, err)
	}
	return verifyStoreDirectory(path, mode)
}

func verifyStoreDirectory(path string, mode os.FileMode) error {
	pathInfo, err := os.Lstat(path)
	if err != nil || !pathInfo.IsDir() || pathInfo.Mode()&os.ModeSymlink != 0 || !ownedByCurrentProcess(pathInfo) {
		return errors.Join(ErrCorruptStore, err)
	}
	directory, err := os.Open(path)
	if err != nil {
		return errors.Join(ErrCorruptStore, err)
	}
	defer directory.Close()
	openedInfo, err := directory.Stat()
	if err != nil || !openedInfo.IsDir() || !os.SameFile(pathInfo, openedInfo) || !ownedByCurrentProcess(openedInfo) {
		return errors.Join(ErrCorruptStore, err)
	}
	if err := directory.Chmod(mode); err != nil {
		return errors.Join(ErrCorruptStore, err)
	}
	finalInfo, err := os.Lstat(path)
	if err != nil || !finalInfo.IsDir() || finalInfo.Mode()&os.ModeSymlink != 0 ||
		finalInfo.Mode().Perm() != mode || !os.SameFile(openedInfo, finalInfo) || !ownedByCurrentProcess(finalInfo) {
		return errors.Join(ErrCorruptStore, err)
	}
	return nil
}

func verifyTrustedAncestors(path string) error {
	effectiveUID, ok := effectiveUserID()
	if !ok {
		return ErrCorruptStore
	}
	for current := path; ; current = filepath.Dir(current) {
		info, err := os.Lstat(current)
		if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return errors.Join(ErrCorruptStore, err)
		}
		owner, ownerOK := fileOwner(info)
		if !ownerOK || (owner != 0 && owner != effectiveUID) {
			return ErrCorruptStore
		}
		if info.Mode().Perm()&0o022 != 0 && info.Mode()&os.ModeSticky == 0 {
			return ErrCorruptStore
		}
		parent := filepath.Dir(current)
		if parent == current {
			return nil
		}
	}
}

func verifyTrustedLexicalPath(path string) error {
	effectiveUID, ok := effectiveUserID()
	if !ok {
		return ErrCorruptStore
	}
	for current := path; ; current = filepath.Dir(current) {
		info, err := os.Lstat(current)
		if err != nil {
			return errors.Join(ErrCorruptStore, err)
		}
		owner, ownerOK := fileOwner(info)
		if !ownerOK || (owner != 0 && owner != effectiveUID) {
			return ErrCorruptStore
		}
		if info.Mode()&os.ModeSymlink == 0 {
			if !info.IsDir() || (info.Mode().Perm()&0o022 != 0 && info.Mode()&os.ModeSticky == 0) {
				return ErrCorruptStore
			}
		}
		parent := filepath.Dir(current)
		if parent == current {
			return nil
		}
	}
}

func ownedByCurrentProcess(info os.FileInfo) bool {
	owner, ownerOK := fileOwner(info)
	effectiveUID, effectiveOK := effectiveUserID()
	return ownerOK && effectiveOK && owner == effectiveUID
}

func publishFile(stagedPath, finalPath string, expectedMode os.FileMode) error {
	if err := os.Link(stagedPath, finalPath); err != nil {
		if !errors.Is(err, os.ErrExist) {
			return fail(StepPublish, "link", err)
		}
		equal, compareErr := equalFiles(stagedPath, finalPath)
		if compareErr != nil || !equal {
			return fail(StepPublish, "existing_mismatch", errors.Join(ErrCorruptStore, compareErr))
		}
	}
	finalInfo, err := os.Lstat(finalPath)
	if err != nil || !finalInfo.Mode().IsRegular() || finalInfo.Mode()&os.ModeSymlink != 0 ||
		finalInfo.Mode().Perm() != expectedMode || !ownedByCurrentProcess(finalInfo) {
		return fail(StepPublish, "unsafe_file", errors.Join(ErrCorruptStore, err))
	}
	return nil
}

func equalFiles(leftPath, rightPath string) (bool, error) {
	leftInfo, err := os.Lstat(leftPath)
	if err != nil || !leftInfo.Mode().IsRegular() || leftInfo.Mode()&os.ModeSymlink != 0 {
		return false, errors.Join(ErrCorruptStore, err)
	}
	left, err := os.Open(leftPath)
	if err != nil {
		return false, err
	}
	defer left.Close()
	rightInfo, err := os.Lstat(rightPath)
	if err != nil || !rightInfo.Mode().IsRegular() || rightInfo.Mode()&os.ModeSymlink != 0 || rightInfo.Size() != leftInfo.Size() {
		return false, errors.Join(ErrCorruptStore, err)
	}
	right, err := os.Open(rightPath)
	if err != nil {
		return false, err
	}
	defer right.Close()
	leftHash := sha256.New()
	rightHash := sha256.New()
	if _, err := io.Copy(leftHash, left); err != nil {
		return false, err
	}
	if _, err := io.Copy(rightHash, right); err != nil {
		return false, err
	}
	return string(leftHash.Sum(nil)) == string(rightHash.Sum(nil)), nil
}

func syncDirectory(path string) error {
	directory, err := os.Open(path)
	if err != nil {
		return err
	}
	defer directory.Close()
	return directory.Sync()
}

func (store *Store) Root() string {
	if store == nil {
		return ""
	}
	return store.root
}

type boundedArchiveWriter struct {
	destination io.Writer
	written     int64
	maximum     int64
}

func (writer *boundedArchiveWriter) Write(data []byte) (int, error) {
	if int64(len(data)) > writer.maximum-writer.written {
		return 0, ErrTooLarge
	}
	written, err := writer.destination.Write(data)
	writer.written += int64(written)
	return written, err
}
