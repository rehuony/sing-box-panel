// SPDX-License-Identifier: GPL-3.0-or-later

package artifactstore

import (
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
)

var ErrLegacyLayout = errors.New("legacy sharded core storage is unsupported; reinitialize the instance data directory and reinstall cores (existing data is not migrated)")

// Reject the old layout before installing anything into a mixed store. This is
// an explicit storage break, not an implicit migration or cleanup operation.
func checkContentLayout(root string) error {
	directory := filepath.Join(root, "sha256")
	info, err := os.Lstat(directory)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return errors.Join(ErrCorruptStore, err)
	}
	entries, err := os.ReadDir(directory)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if len(entry.Name()) == 2 {
			if _, err := hex.DecodeString(entry.Name()); err == nil {
				return ErrLegacyLayout
			}
		}
	}
	return nil
}
