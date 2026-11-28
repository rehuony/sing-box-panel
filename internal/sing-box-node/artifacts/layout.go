// SPDX-License-Identifier: GPL-3.0-or-later

package artifacts

import (
	"errors"
	"os"
	"path/filepath"
)

// checkContentDirectory refuses a content directory that is not a directory
// of its own, such as a symlink, before anything is read from or installed
// into it.
func checkContentDirectory(root string) error {
	info, err := os.Lstat(filepath.Join(root, "sha256"))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil || !info.IsDir() {
		return errors.Join(ErrCorruptStore, err)
	}
	return nil
}
