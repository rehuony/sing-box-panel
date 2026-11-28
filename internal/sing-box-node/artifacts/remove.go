// SPDX-License-Identifier: GPL-3.0-or-later

package artifacts

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/rehuony/sing-box-panel/internal/atomicfile"
	"github.com/rehuony/sing-box-panel/internal/coreartifact"
)

// Remove deletes an installed core's files: the directory its archive's
// digest names, holding the archive and the binary. A directory already gone
// counts as removed; anything else in its place means the store is corrupt.
func (store *Store) Remove(digest coreartifact.SHA256) error {
	parent := filepath.Join(store.root, "sha256")
	directory := filepath.Join(parent, digest.String())
	info, err := os.Lstat(directory)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("inspect core files: %w", err)
	}
	if !info.IsDir() {
		return ErrCorruptStore
	}
	if err := os.RemoveAll(directory); err != nil {
		return fmt.Errorf("delete core files: %w", err)
	}
	return atomicfile.SyncDir(parent)
}
