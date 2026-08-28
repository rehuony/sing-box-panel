// SPDX-License-Identifier: GPL-3.0-or-later

package artifactstore

import (
	"crypto/sha256"
	"errors"
	"io"
	"os"
	"path/filepath"
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
