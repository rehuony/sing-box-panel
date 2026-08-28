// SPDX-License-Identifier: GPL-3.0-or-later

package artifactstore

import (
	"path/filepath"

	"github.com/rehuony/sing-box-panel/internal/coreartifact"
)

func (store *Store) publish(stagedBinaryPath, stagedArchivePath string, digest coreartifact.SHA256) (string, string, error) {
	digestText := digest.String()
	parent := filepath.Join(store.root, "sha256", digestText[:2])
	finalDirectory := filepath.Join(parent, digestText)
	finalBinaryPath := filepath.Join(finalDirectory, "sing-box")
	finalArchivePath := filepath.Join(finalDirectory, "artifact.tar.gz")
	for _, directory := range []string{filepath.Join(store.root, "sha256"), parent, finalDirectory} {
		if err := ensureStoreDirectory(directory, 0o700); err != nil {
			return "", "", fail(StepPublish, "unsafe_directory", err)
		}
	}
	if err := publishFile(stagedArchivePath, finalArchivePath, 0o600); err != nil {
		return "", "", err
	}
	// Persist the archive directory entry before publishing the executable.
	// A crash may leave archive-only state, which an idempotent retry repairs,
	// but a durably visible executable never precedes its source archive.
	if err := syncDirectory(finalDirectory); err != nil {
		return "", "", fail(StepPublish, "sync", err)
	}
	if err := publishFile(stagedBinaryPath, finalBinaryPath, 0o700); err != nil {
		return "", "", err
	}
	if err := syncDirectory(finalDirectory); err != nil {
		return "", "", fail(StepPublish, "sync", err)
	}
	if err := syncDirectory(parent); err != nil {
		return "", "", fail(StepPublish, "sync", err)
	}
	if err := syncDirectory(filepath.Join(store.root, "sha256")); err != nil {
		return "", "", fail(StepPublish, "sync", err)
	}
	if err := syncDirectory(store.root); err != nil {
		return "", "", fail(StepPublish, "sync", err)
	}
	return finalBinaryPath, finalArchivePath, nil
}
