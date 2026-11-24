// SPDX-License-Identifier: GPL-3.0-or-later

package artifacts

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
)

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

// UnsafePathError names a directory above the artifact store that another
// account could change, which would let that account replace the cores the
// node runs.
type UnsafePathError struct {
	Path   string
	Reason string
}

func (unsafe *UnsafePathError) Error() string { return unsafe.Path + " " + unsafe.Reason }

func unsafePath(path, reason string) error {
	return errors.Join(ErrCorruptStore, &UnsafePathError{Path: path, Reason: reason})
}

// trustedDirectory checks one directory above the store: root or the
// effective user owns it, and only its owner can change it unless it is
// sticky. A symbolic link is checked by its owner only.
func trustedDirectory(path string, info os.FileInfo, effectiveUID uint64) error {
	owner, ownerOK := fileOwner(info)
	if !ownerOK {
		return ErrCorruptStore
	}
	if owner != 0 && owner != effectiveUID {
		return unsafePath(path, fmt.Sprintf("belongs to %s, neither root nor %s, which runs the node", accountName(owner), accountName(effectiveUID)))
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return nil
	}
	if !info.IsDir() {
		return unsafePath(path, "is not a directory")
	}
	if info.Mode().Perm()&0o022 != 0 && info.Mode()&os.ModeSticky == 0 {
		return unsafePath(path, fmt.Sprintf("can be changed by other accounts (mode %04o without the sticky bit)", info.Mode().Perm()))
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
		if err != nil {
			return errors.Join(ErrCorruptStore, err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return unsafePath(current, "is a symbolic link")
		}
		if err := trustedDirectory(current, info, effectiveUID); err != nil {
			return err
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
		if err := trustedDirectory(current, info, effectiveUID); err != nil {
			return err
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
	}
	finalInfo, err := os.Lstat(finalPath)
	if err != nil || !finalInfo.Mode().IsRegular() || finalInfo.Mode()&os.ModeSymlink != 0 ||
		finalInfo.Mode().Perm() != expectedMode || !ownedByCurrentProcess(finalInfo) {
		return fail(StepPublish, "unsafe_file", errors.Join(ErrCorruptStore, err))
	}
	return nil
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

// copyBounded copies at most maximumBytes and fails with ErrTooLarge beyond
// them, checking ctx between reads.
func copyBounded(ctx context.Context, destination io.Writer, source io.Reader, maximumBytes int64) (int64, error) {
	buffer := make([]byte, 32<<10)
	var total int64
	for {
		if err := ctx.Err(); err != nil {
			return total, err
		}
		remaining := maximumBytes - total
		if remaining < 0 {
			return total, ErrTooLarge
		}
		readSize := len(buffer)
		if int64(readSize) > remaining+1 {
			readSize = int(remaining + 1)
		}
		count, readErr := source.Read(buffer[:readSize])
		if count > 0 {
			if total+int64(count) > maximumBytes {
				return total, ErrTooLarge
			}
			written, writeErr := destination.Write(buffer[:count])
			total += int64(written)
			if writeErr != nil {
				return total, writeErr
			}
			if written != count {
				return total, io.ErrShortWrite
			}
		}
		if readErr == io.EOF {
			return total, nil
		}
		if readErr != nil {
			return total, readErr
		}
		if count == 0 {
			return total, io.ErrNoProgress
		}
	}
}

// accountName names uid by its account, or by its number when it has none.
func accountName(uid uint64) string {
	id := strconv.FormatUint(uid, 10)
	if found, err := user.LookupId(id); err == nil {
		return "the " + found.Username + " account"
	}
	return "uid " + id
}
