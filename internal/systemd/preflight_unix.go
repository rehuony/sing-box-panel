// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux || darwin

package systemd

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"
)

func writableParent(path string) error {
	return writableDirectory(filepath.Dir(path))
}

// An existing directory needs no write access to its parent. Missing directories
// require a writable ancestor instead. Actual writes still check races and ACLs.
func writableDirectory(directory string) error {
	for {
		info, err := os.Lstat(directory)
		if errors.Is(err, os.ErrNotExist) {
			directory = filepath.Dir(directory)
			continue
		}
		if err != nil {
			return err
		}
		if !info.IsDir() {
			return fmt.Errorf("%w: destination directory %q must be a physical directory", ErrInvalid, directory)
		}
		if err := unix.Access(directory, unix.W_OK|unix.X_OK); err != nil {
			return fmt.Errorf("destination directory %q is not writable: %w", directory, err)
		}
		return nil
	}
}
