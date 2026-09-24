// SPDX-License-Identifier: GPL-3.0-or-later

package artifactstore

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLegacyContentLayoutRequiresReinitializationWithoutMovingFiles(t *testing.T) {
	for _, shard := range []string{"d9", "00", "ff"} {
		t.Run(shard, func(t *testing.T) {
			root := filepath.Join(t.TempDir(), "artifacts")
			store, err := New(Options{Root: root})
			if err != nil {
				t.Fatal(err)
			}
			digest := strings.Repeat(shard, 32)
			legacy := filepath.Join(store.Root(), "sha256", shard, digest, "sing-box")
			if err := os.MkdirAll(filepath.Dir(legacy), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(legacy, []byte("old core"), 0o700); err != nil {
				t.Fatal(err)
			}
			if _, err := New(Options{Root: root}); !errors.Is(err, ErrLegacyLayout) || !strings.Contains(err.Error(), "reinitialize") {
				t.Fatalf("open legacy store: %v", err)
			}
			asset := officialAsset(artifactVersion(t, "1.13.19"), bytesDigest([]byte("archive")), 7)
			if _, err := store.InstallOfficial(context.Background(), asset); !errors.Is(err, ErrLegacyLayout) {
				t.Fatalf("install into legacy store: %v", err)
			}
			if err := store.SetCurrent(context.Background(), legacy); !errors.Is(err, ErrCorruptStore) {
				t.Fatalf("accepted legacy current target: %v", err)
			}
			if bytes, err := os.ReadFile(legacy); err != nil || string(bytes) != "old core" {
				t.Fatalf("legacy binary modified: %q, %v", bytes, err)
			}
			if _, err := os.Lstat(filepath.Join(store.Root(), "sha256", digest)); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("created a new-layout directory: %v", err)
			}
		})
	}
}

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
