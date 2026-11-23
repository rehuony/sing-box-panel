// SPDX-License-Identifier: GPL-3.0-or-later

package atomicfile

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestWriteReplacesOrCreatesExclusively(t *testing.T) {
	path := filepath.Join(t.TempDir(), "file")
	if err := Write(path, []byte("first"), 0o600, Options{Exclusive: true}); err != nil {
		t.Fatal(err)
	}
	if err := Write(path, []byte("second"), 0o600, Options{Exclusive: true}); !errors.Is(err, os.ErrExist) {
		t.Fatalf("exclusive write over an existing file: %v", err)
	}
	if err := Write(path, []byte("third"), 0o644, Options{}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "third" {
		t.Fatalf("content = %q, %v", data, err)
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0o644 {
		t.Fatalf("mode = %v, %v", info.Mode(), err)
	}
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil || len(entries) != 1 {
		t.Fatalf("temporary files left behind: %v %v", entries, err)
	}
}
