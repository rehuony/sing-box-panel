// SPDX-License-Identifier: GPL-3.0-or-later

package testutil

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rehuony/sing-box-panel/internal/coreartifact"
)

// WriteCoreUpload stages a placeholder core upload named name in directory.
func WriteCoreUpload(t *testing.T, directory, name string) string {
	t.Helper()
	path := filepath.Join(directory, name)
	if err := os.WriteFile(path, []byte("archive"), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// AssertPathExists fails the test unless path exists.
func AssertPathExists(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Lstat(path); err != nil {
		t.Fatalf("expected %s to exist: %v", path, err)
	}
}

// AssertPathMissing fails the test unless path is absent.
func AssertPathMissing(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected %s to be absent, got %v", path, err)
	}
}

// Digest returns the SHA-256 whose hexadecimal form repeats pair 32 times.
func Digest(t *testing.T, pair string) coreartifact.SHA256 {
	t.Helper()
	digest, err := coreartifact.ParseSHA256(strings.Repeat(pair, 32))
	if err != nil {
		t.Fatal(err)
	}
	return digest
}
