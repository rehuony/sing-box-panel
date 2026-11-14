// SPDX-License-Identifier: GPL-3.0-or-later

package selfupdate

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/rehuony/sing-box-panel/internal/atomicfile"
	releasecontract "github.com/rehuony/sing-box-panel/internal/release"
	"golang.org/x/mod/semver"
)

// PreviousSuffix names the binary an install replaced, kept beside the
// executable so a failed update can be undone.
const PreviousSuffix = ".previous"

// Staged is a verified release binary in a staging directory.
type Staged struct {
	Version string
	// Path is the binary in the staging directory beside the release's
	// checksum manifest and signature.
	Path   string
	SHA256 [sha256.Size]byte
	Size   int64
}

// Verify checks the release files staged in dir for this updater's binary
// and platform: the signature over version and the checksum manifest with
// the verification key, the binary's checksum, its Go build identity and
// platform, and a version strictly newer than currentVersion. A release of
// currentVersion is ErrUpToDate and an older one ErrVersionNotNewer.
func (updater *Updater) Verify(dir, version, currentVersion string) (Staged, error) {
	if err := updater.ready(currentVersion); err != nil {
		return Staged{}, err
	}
	if err := releasecontract.ValidateVersion(version); err != nil {
		return Staged{}, fmt.Errorf("%w: %q is not a release version", ErrReleaseInvalid, version)
	}
	switch comparison := semver.Compare(version, currentVersion); {
	case comparison == 0:
		return Staged{}, fmt.Errorf("%w: %s", ErrUpToDate, version)
	case comparison < 0:
		return Staged{}, fmt.Errorf("%w: %s is older than %s", ErrVersionNotNewer, version, currentVersion)
	}
	checksums, err := readStaged(filepath.Join(dir, ChecksumsName), MaxChecksumBytes)
	if err != nil {
		return Staged{}, err
	}
	signature, err := readStaged(filepath.Join(dir, SignatureName), MaxSignatureBytes)
	if err != nil {
		return Staged{}, err
	}
	if err := releasecontract.Verify(updater.publicKey, version, checksums, signature); err != nil {
		return Staged{}, fmt.Errorf("%w: %v", ErrSignatureInvalid, err)
	}
	name := updater.AssetName()
	expected, err := parseChecksum(checksums, name)
	if err != nil {
		return Staged{}, err
	}
	path := filepath.Join(dir, name)
	digest, size, err := stagedDigest(path)
	if err != nil {
		return Staged{}, err
	}
	if digest != expected {
		return Staged{}, fmt.Errorf("%w: %s expected %x, got %x", ErrChecksumInvalid, name, expected, digest)
	}
	if err := updater.validateStagedExecutable(path, updater.binary, updater.goos, updater.goarch); err != nil {
		return Staged{}, err
	}
	return Staged{Version: version, Path: path, SHA256: digest, Size: size}, nil
}

// readStaged reads a regular staged file of at most maximum bytes.
func readStaged(path string, maximum int64) ([]byte, error) {
	file, err := openStaged(path, maximum)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	data, err := readBounded(file, maximum)
	if err != nil {
		return nil, fmt.Errorf("%w: %s: %v", ErrStagedFileInvalid, filepath.Base(path), err)
	}
	return data, nil
}

func stagedDigest(path string) ([sha256.Size]byte, int64, error) {
	var digest [sha256.Size]byte
	file, err := openStaged(path, MaxBinaryBytes)
	if err != nil {
		return digest, 0, err
	}
	defer file.Close()
	hash := sha256.New()
	size, err := io.Copy(hash, io.LimitReader(file, MaxBinaryBytes+1))
	if err != nil || size > MaxBinaryBytes {
		return digest, 0, fmt.Errorf("%w: %s", ErrStagedFileInvalid, filepath.Base(path))
	}
	copy(digest[:], hash.Sum(nil))
	return digest, size, nil
}

// openStaged opens a staged file that is a regular file, not a symbolic
// link, of at most maximum bytes.
func openStaged(path string, maximum int64) (*os.File, error) {
	before, err := os.Lstat(path)
	if err != nil || !before.Mode().IsRegular() || before.Size() > maximum {
		return nil, fmt.Errorf("%w: %s", ErrStagedFileInvalid, filepath.Base(path))
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("%w: %s: %v", ErrStagedFileInvalid, filepath.Base(path), err)
	}
	after, err := file.Stat()
	if err != nil || !os.SameFile(before, after) {
		_ = file.Close()
		return nil, fmt.Errorf("%w: %s changed while it was opened", ErrStagedFileInvalid, filepath.Base(path))
	}
	return file, nil
}

// Install replaces the current executable with a verified staged binary,
// atomically and under the update lock, keeping the replaced binary beside
// it with PreviousSuffix.
func (updater *Updater) Install(ctx context.Context, staged Staged) (Result, error) {
	targetPath, targetMode, err := updater.executableTarget()
	if err != nil {
		return Result{}, err
	}
	targetDigest, err := executableDigest(targetPath)
	if err != nil {
		return Result{}, err
	}
	lock, err := acquireUpdateLock(ctx, targetPath)
	if err != nil {
		return Result{}, fmt.Errorf("acquire update lock: %w", err)
	}
	defer func() { _ = lock.Close() }()
	if err := requireExecutableDigest(targetPath, targetDigest); err != nil {
		return Result{}, err
	}
	if err := updater.installLocked(ctx, staged, targetPath, targetMode, targetDigest); err != nil {
		return Result{}, err
	}
	return Result{Version: staged.Version, Updated: true, ExecutablePath: targetPath}, nil
}

// installLocked copies the staged binary beside the target, checks the copy
// against the verified digest, and renames it over the target, which must
// still have targetDigest. The caller holds the update lock.
func (updater *Updater) installLocked(ctx context.Context, staged Staged, targetPath string, targetMode os.FileMode, targetDigest [sha256.Size]byte) error {
	source, err := openStaged(staged.Path, MaxBinaryBytes)
	if err != nil {
		return err
	}
	defer source.Close()
	temporary, err := os.CreateTemp(filepath.Dir(targetPath), "."+filepath.Base(targetPath)+"-update-*")
	if err != nil {
		return fmt.Errorf("stage update beside executable: %w", err)
	}
	temporaryPath := temporary.Name()
	closed := false
	defer func() {
		if !closed {
			_ = temporary.Close()
		}
		_ = os.Remove(temporaryPath)
	}()
	hash := sha256.New()
	if _, err := io.Copy(io.MultiWriter(temporary, hash), io.LimitReader(source, MaxBinaryBytes+1)); err != nil {
		return fmt.Errorf("copy staged executable: %w", err)
	}
	var digest [sha256.Size]byte
	copy(digest[:], hash.Sum(nil))
	if digest != staged.SHA256 {
		return fmt.Errorf("%w: the staged binary changed after it was verified", ErrChecksumInvalid)
	}
	if err := temporary.Chmod(targetMode); err != nil {
		return fmt.Errorf("set staged executable permissions: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		return fmt.Errorf("sync staged executable: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close staged executable: %w", err)
	}
	closed = true
	if err := requireExecutableDigest(targetPath, targetDigest); err != nil {
		return err
	}
	// Cancellation is honored until the atomic replacement commit point. Once
	// Rename starts, the update is committed and only durability work remains.
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := keepPrevious(targetPath); err != nil {
		return err
	}
	if err := os.Rename(temporaryPath, targetPath); err != nil {
		return fmt.Errorf("replace current executable: %w", err)
	}
	return atomicfile.SyncDir(filepath.Dir(targetPath))
}

// keepPrevious links the current executable to its previous name, replacing
// an older one, before an install replaces it.
func keepPrevious(targetPath string) error {
	candidate, err := os.CreateTemp(filepath.Dir(targetPath), "."+filepath.Base(targetPath)+"-previous-*")
	if err != nil {
		return fmt.Errorf("keep previous executable: %w", err)
	}
	candidatePath := candidate.Name()
	defer func() { _ = os.Remove(candidatePath) }()
	if err := candidate.Close(); err != nil {
		return err
	}
	if err := os.Remove(candidatePath); err != nil {
		return err
	}
	if err := os.Link(targetPath, candidatePath); err != nil {
		if err := copyFile(targetPath, candidatePath); err != nil {
			return fmt.Errorf("keep previous executable: %w", err)
		}
	}
	if err := os.Rename(candidatePath, targetPath+PreviousSuffix); err != nil {
		return fmt.Errorf("keep previous executable: %w", err)
	}
	return nil
}

func copyFile(source, destination string) error {
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()
	info, err := input.Stat()
	if err != nil {
		return err
	}
	output, err := os.OpenFile(destination, os.O_WRONLY|os.O_CREATE|os.O_EXCL, info.Mode().Perm())
	if err != nil {
		return err
	}
	if _, err := io.Copy(output, input); err != nil {
		_ = output.Close()
		return err
	}
	if err := output.Sync(); err != nil {
		_ = output.Close()
		return err
	}
	return output.Close()
}

// RestorePrevious puts back the binary that the last install replaced,
// under the update lock.
func (updater *Updater) RestorePrevious(ctx context.Context) error {
	targetPath, _, err := updater.executableTarget()
	if err != nil {
		return err
	}
	lock, err := acquireUpdateLock(ctx, targetPath)
	if err != nil {
		return fmt.Errorf("acquire update lock: %w", err)
	}
	defer func() { _ = lock.Close() }()
	previous := targetPath + PreviousSuffix
	info, err := os.Lstat(previous)
	if errors.Is(err, os.ErrNotExist) {
		return ErrNoPreviousExecutable
	}
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("%w: %s is not a regular file", ErrExecutableInvalid, previous)
	}
	if err := os.Rename(previous, targetPath); err != nil {
		return fmt.Errorf("restore previous executable: %w", err)
	}
	return atomicfile.SyncDir(filepath.Dir(targetPath))
}

// Writable reports whether the process may replace its executable.
func (updater *Updater) Writable() (bool, error) {
	targetPath, _, err := updater.executableTarget()
	if err != nil {
		return false, err
	}
	return directoryWritable(filepath.Dir(targetPath)), nil
}

// VerifyFiles checks the release files in dir without regard to the running
// version: the signature over version and the checksum manifest, and the
// checksum of each named file. It returns the manifest, its signature and
// each file's digest.
func (updater *Updater) VerifyFiles(dir, version string, names []string) ([]byte, []byte, map[string][sha256.Size]byte, error) {
	if err := updater.keyReady(); err != nil {
		return nil, nil, nil, err
	}
	checksums, err := readStaged(filepath.Join(dir, ChecksumsName), MaxChecksumBytes)
	if err != nil {
		return nil, nil, nil, err
	}
	signature, err := readStaged(filepath.Join(dir, SignatureName), MaxSignatureBytes)
	if err != nil {
		return nil, nil, nil, err
	}
	if err := releasecontract.Verify(updater.publicKey, version, checksums, signature); err != nil {
		return nil, nil, nil, fmt.Errorf("%w: %v", ErrSignatureInvalid, err)
	}
	digests := make(map[string][sha256.Size]byte, len(names))
	for _, name := range names {
		expected, err := parseChecksum(checksums, name)
		if err != nil {
			return nil, nil, nil, err
		}
		digest, _, err := stagedDigest(filepath.Join(dir, name))
		if err != nil {
			return nil, nil, nil, err
		}
		if digest != expected {
			return nil, nil, nil, fmt.Errorf("%w: %s expected %x, got %x", ErrChecksumInvalid, name, expected, digest)
		}
		digests[name] = digest
	}
	return checksums, signature, digests, nil
}
