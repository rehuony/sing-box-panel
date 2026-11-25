// SPDX-License-Identifier: GPL-3.0-or-later

//go:build darwin || linux

package selfupdate

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/sys/unix"
)

func directoryWritable(path string) bool {
	return unix.Access(path, unix.W_OK) == nil
}

// HelperOptions are what the install step of a system service's unit needs
// besides its updater.
type HelperOptions struct {
	// DataDir is the service's data directory, as the unit names it.
	DataDir string
	// Current is the version of the installed executable: the step's own.
	Current string
	Now     func() time.Time
}

// ApplyStaged installs the release a system service staged before it exited
// for its unit to start it again; the unit runs it as root before the
// service. The service account owns the staging directory, so the step
// follows no symbolic link inside the data directory, reads only regular
// files, and copies them beside the executable, where only root writes,
// before reading them. It verifies them with the key embedded in the
// installed executable, refuses a version that is not newer, and installs.
// An update goes one way: the replaced binary is not kept. The request is
// validated as a strict release version and is only compared with directory
// entries, never used as a path. The result lands in the updates directory
// for the service that starts next to read; nil means nothing was requested.
func (updater *Updater) ApplyStaged(ctx context.Context, options HelperOptions) (*Attempt, error) {
	if options.Now == nil {
		options.Now = time.Now
	}
	data, err := openDirectoryPath(options.DataDir)
	if err != nil {
		return nil, fmt.Errorf("open the service data directory: %w", err)
	}
	defer unix.Close(data)
	updates, err := unix.Openat(data, UpdatesDir, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if errors.Is(err, unix.ENOENT) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("open the updates directory: %w", err)
	}
	defer unix.Close(updates)
	raw, err := readAt(updates, RequestName, maxRequest)
	if errors.Is(err, unix.ENOENT) {
		return nil, nil
	}
	// The request is consumed before any work, so a failed attempt is not
	// triggered again.
	if removeErr := unix.Unlinkat(updates, RequestName, 0); removeErr != nil && !errors.Is(removeErr, unix.ENOENT) {
		return nil, fmt.Errorf("remove the update request: %w", removeErr)
	}
	attempt := Attempt{}
	// finish publishes the outcome for the service that starts next.
	finish := func(applyErr error) (*Attempt, error) {
		attempt.Succeeded, attempt.Code = applyErr == nil, ""
		if applyErr != nil {
			attempt.Code = FailureCode(applyErr)
		}
		attempt.FinishedAt = options.Now().UTC()
		if err := writeResultAt(updates, attempt); err != nil {
			return &attempt, errors.Join(applyErr, fmt.Errorf("record the update result: %w", err))
		}
		return &attempt, applyErr
	}
	if err != nil {
		return finish(fmt.Errorf("%w: read the update request: %v", ErrStagedFileInvalid, err))
	}
	version, err := parseRequest(raw)
	if err != nil {
		return finish(err)
	}
	attempt.Version = version
	return finish(updater.applyStaged(ctx, updates, version, options))
}

func (updater *Updater) applyStaged(ctx context.Context, updates int, version string, options HelperOptions) error {
	if err := updater.ready(options.Current); err != nil {
		return err
	}
	staged, err := openStagedRelease(updates, version)
	if err != nil {
		return err
	}
	defer unix.Close(staged)
	targetPath, targetMode, err := updater.executableTarget()
	if err != nil {
		return err
	}
	private, err := os.MkdirTemp(filepath.Dir(targetPath), "."+filepath.Base(targetPath)+"-apply-*")
	if err != nil {
		return fmt.Errorf("prepare a private copy of the staged release: %w", err)
	}
	defer func() { _ = os.RemoveAll(private) }()
	for _, file := range []struct {
		name    string
		maximum int64
	}{
		{ChecksumsName, MaxChecksumBytes}, {SignatureName, MaxSignatureBytes}, {updater.AssetName(), MaxBinaryBytes},
	} {
		if err := copyFromStaging(staged, file.name, file.maximum, filepath.Join(private, file.name)); err != nil {
			return err
		}
	}
	verified, err := updater.Verify(private, version, options.Current)
	if err != nil {
		return err
	}
	targetDigest, err := executableDigest(targetPath)
	if err != nil {
		return err
	}
	lock, err := acquireUpdateLock(ctx, targetPath)
	if err != nil {
		return fmt.Errorf("acquire update lock: %w", err)
	}
	defer func() { _ = lock.Close() }()
	return updater.installLocked(ctx, verified, targetPath, targetMode, targetDigest)
}

// openDirectoryPath opens the service's data directory. Its parent belongs
// to root and may be reached through symbolic links; from there on no
// component is followed if it is one.
func openDirectoryPath(path string) (int, error) {
	if !filepath.IsAbs(path) || filepath.Clean(path) != path || path == "/" {
		return -1, fmt.Errorf("%q must be an absolute, normalized path", path)
	}
	parent, err := filepath.EvalSymlinks(filepath.Dir(path))
	if err != nil {
		return -1, err
	}
	current, err := unix.Open("/", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	if err != nil {
		return -1, err
	}
	resolved := filepath.Join(parent, filepath.Base(path))
	for _, component := range strings.Split(strings.TrimPrefix(resolved, "/"), "/") {
		if component == "" {
			continue
		}
		next, err := unix.Openat(current, component, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
		_ = unix.Close(current)
		if err != nil {
			return -1, err
		}
		current = next
	}
	return current, nil
}

// openStagedRelease opens the staged directory named version, found by
// comparing the updates directory's entries with the validated version.
func openStagedRelease(updates int, version string) (int, error) {
	listing, err := unix.Dup(updates)
	if err != nil {
		return -1, err
	}
	directory := os.NewFile(uintptr(listing), UpdatesDir)
	names, err := directory.Readdirnames(-1)
	_ = directory.Close()
	if err != nil {
		return -1, fmt.Errorf("list the updates directory: %w", err)
	}
	for _, name := range names {
		if name != version {
			continue
		}
		staged, err := unix.Openat(updates, name, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
		if err != nil {
			return -1, fmt.Errorf("%w: the staged release %s: %v", ErrStagedFileInvalid, version, err)
		}
		return staged, nil
	}
	return -1, fmt.Errorf("%w: no staged release %s", ErrStagedFileInvalid, version)
}

// openRegularAt opens a regular file of at most maximum bytes in directory,
// refusing symbolic links and every other kind of file without blocking on
// them.
func openRegularAt(directory int, name string, maximum int64) (*os.File, error) {
	fd, err := unix.Openat(directory, name, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, err
	}
	var stat unix.Stat_t
	if err := unix.Fstat(fd, &stat); err != nil {
		_ = unix.Close(fd)
		return nil, err
	}
	if stat.Mode&unix.S_IFMT != unix.S_IFREG || stat.Size > maximum {
		_ = unix.Close(fd)
		return nil, fmt.Errorf("%w: %s", ErrStagedFileInvalid, name)
	}
	return os.NewFile(uintptr(fd), name), nil
}

func readAt(directory int, name string, maximum int64) ([]byte, error) {
	file, err := openRegularAt(directory, name, maximum)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	return readBounded(file, maximum)
}

func copyFromStaging(staged int, name string, maximum int64, destination string) error {
	source, err := openRegularAt(staged, name, maximum)
	if err != nil {
		if errors.Is(err, ErrStagedFileInvalid) {
			return err
		}
		return fmt.Errorf("%w: %s: %v", ErrStagedFileInvalid, name, err)
	}
	defer source.Close()
	output, err := os.OpenFile(destination, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	written, err := io.Copy(output, io.LimitReader(source, maximum+1))
	closeErr := output.Close()
	if err != nil {
		return err
	}
	if written > maximum {
		return fmt.Errorf("%w: %s grew while it was copied", ErrStagedFileInvalid, name)
	}
	return closeErr
}

// writeResultAt publishes the attempt as the result in the updates
// directory through its descriptor, creating a new file and renaming it,
// so no path inside the service's directory is followed.
func writeResultAt(updates int, attempt Attempt) error {
	data, err := json.Marshal(attempt)
	if err != nil {
		return err
	}
	suffix := make([]byte, 8)
	if _, err := rand.Read(suffix); err != nil {
		return err
	}
	temporary := ".result-" + hex.EncodeToString(suffix)
	fd, err := unix.Openat(updates, temporary, unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0o644)
	if err != nil {
		return err
	}
	file := os.NewFile(uintptr(fd), temporary)
	_, writeErr := file.Write(append(data, '\n'))
	if writeErr == nil {
		writeErr = file.Chmod(0o644)
	}
	if writeErr == nil {
		writeErr = file.Sync()
	}
	if err := errors.Join(writeErr, file.Close()); err != nil {
		_ = unix.Unlinkat(updates, temporary, 0)
		return err
	}
	if err := unix.Renameat(updates, temporary, updates, resultName); err != nil {
		_ = unix.Unlinkat(updates, temporary, 0)
		return err
	}
	return unix.Fsync(updates)
}
