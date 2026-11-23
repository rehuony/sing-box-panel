// SPDX-License-Identifier: GPL-3.0-or-later

// Package atomicfile publishes files so that readers see either the previous
// file or the complete new one, and the change survives a crash.
package atomicfile

import (
	"fmt"
	"os"
	"path/filepath"
)

// Options adjusts how Write publishes a file.
type Options struct {
	// Prepare runs on the temporary file before its content is written, such
	// as to set its owner.
	Prepare func(*os.File) error
	// Exclusive publishes the file only if path does not exist yet; otherwise
	// Write fails with an error that matches os.ErrExist.
	Exclusive bool
}

// Write publishes data at path with permissions perm. It writes and syncs a
// temporary file in the same directory, moves it into place and syncs the
// directory. The directory must exist.
func Write(path string, data []byte, perm os.FileMode, options Options) error {
	directory := filepath.Dir(path)
	temporary, err := os.CreateTemp(directory, "."+filepath.Base(path)+".*.tmp")
	if err != nil {
		return fmt.Errorf("create temporary file for %s: %w", path, err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if options.Prepare != nil {
		if err := options.Prepare(temporary); err != nil {
			temporary.Close()
			return err
		}
	}
	if err := temporary.Chmod(perm); err != nil {
		temporary.Close()
		return fmt.Errorf("set permissions of temporary file for %s: %w", path, err)
	}
	if _, err := temporary.Write(data); err != nil {
		temporary.Close()
		return fmt.Errorf("write temporary file for %s: %w", path, err)
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return fmt.Errorf("sync temporary file for %s: %w", path, err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close temporary file for %s: %w", path, err)
	}
	if options.Exclusive {
		// A link publishes the complete file without replacing one that a
		// concurrent writer created first.
		if err := os.Link(temporaryPath, path); err != nil {
			return fmt.Errorf("create %s: %w", path, err)
		}
	} else if err := os.Rename(temporaryPath, path); err != nil {
		return fmt.Errorf("replace %s: %w", path, err)
	}
	return SyncDir(directory)
}

// SyncDir flushes the entries of directory, such as a rename into it, to
// stable storage.
func SyncDir(directory string) error {
	handle, err := os.Open(directory)
	if err != nil {
		return fmt.Errorf("open directory %s: %w", directory, err)
	}
	defer handle.Close()
	if err := handle.Sync(); err != nil {
		return fmt.Errorf("sync directory %s: %w", directory, err)
	}
	return nil
}
