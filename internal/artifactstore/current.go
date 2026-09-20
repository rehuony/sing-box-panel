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
// Runtime execution continues to verify and open the immutable binary itself.
func (store *Store) SetCurrent(ctx context.Context, binaryPath, binarySHA256 string) error {
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
	if err != nil || len(parts) != 4 || parts[0] != "sha256" || len(parts[2]) != 64 || parts[1] != parts[2][:2] || parts[3] != "sing-box" {
		return ErrCorruptStore
	}
	if _, err := coreartifact.ParseSHA256(parts[2]); err != nil {
		return err
	}
	if err := verifyTrustedAncestors(filepath.Dir(binaryPath)); err != nil {
		return err
	}
	info, err = os.Lstat(binaryPath)
	if err != nil || !info.Mode().IsRegular() || !ownedByCurrentProcess(info) {
		return errors.Join(ErrCorruptStore, err)
	}
	digest, err := coreartifact.ParseSHA256(binarySHA256)
	if err != nil {
		return err
	}
	if err := verifyFileDigest(ctx, binaryPath, digest, store.limits.MaximumFileBytes); err != nil {
		return err
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
