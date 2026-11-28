// SPDX-License-Identifier: GPL-3.0-or-later

package artifacts

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestNewRejectsSymlinkedContentDirectory(t *testing.T) {
	root := filepath.Join(t.TempDir(), "artifacts")
	if err := os.Mkdir(root, 0o700); err != nil {
		t.Fatal(err)
	}
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, "sha256")); err != nil {
		t.Fatal(err)
	}
	if _, err := New(Options{Root: root}); !errors.Is(err, ErrCorruptStore) {
		t.Fatalf("opened symlinked content store: %v", err)
	}
	if entries, err := os.ReadDir(outside); err != nil || len(entries) != 0 {
		t.Fatalf("symlink target modified: %v, %v", entries, err)
	}
}
