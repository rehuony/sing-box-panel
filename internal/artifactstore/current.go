// SPDX-License-Identifier: GPL-3.0-or-later

package artifactstore

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/rehuony/sing-box-panel/internal/coreartifact"
)

// SetCurrent publishes a relative convenience link to the selected binary.
// Runtime execution checks the selected file and its exact reported version.
func (store *Store) SetCurrent(ctx context.Context, binaryPath string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := verifyTrustedAncestors(store.root); err != nil {
		return err
	}
	current := filepath.Join(store.root, "current")
	info, err := os.Lstat(current)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err == nil && info.Mode()&os.ModeSymlink == 0 {
		return ErrCorruptStore
	}
	if binaryPath == "" {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		if err := os.Remove(current); err != nil {
			return err
		}
		return syncDirectory(store.root)
	}
	relative, err := filepath.Rel(store.root, binaryPath)
	parts := strings.Split(relative, string(filepath.Separator))
	if err != nil || len(parts) != 3 || parts[0] != "sha256" || len(parts[1]) != 64 || parts[2] != "sing-box" {
		return ErrCorruptStore
	}
	if _, err := coreartifact.ParseSHA256(parts[1]); err != nil {
		return err
	}
	if err := verifyTrustedAncestors(filepath.Dir(binaryPath)); err != nil {
		return err
	}
	info, err = os.Lstat(binaryPath)
	if err != nil || !info.Mode().IsRegular() || !ownedByCurrentProcess(info) || info.Mode().Perm()&0o111 == 0 ||
		info.Size() <= 0 || info.Size() > store.limits.MaximumFileBytes {
		return errors.Join(ErrCorruptStore, err)
	}
	if target, err := os.Readlink(current); err == nil && target == relative {
		return nil
	}
	directory, err := os.MkdirTemp(store.root, ".current-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(directory)
	staged := filepath.Join(directory, "current")
	if err := os.Symlink(relative, staged); err != nil {
		return err
	}
	if err := os.Rename(staged, current); err != nil {
		return err
	}
	return syncDirectory(store.root)
}
