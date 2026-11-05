// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux

package installation

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

func TestDataMoveAcrossFilesystems(t *testing.T) {
	path, source := fixture(t)
	target, err := os.MkdirTemp("/dev/shm", "sbp-data-move-")
	if err != nil {
		t.Skip("second filesystem unavailable", err)
	}
	t.Cleanup(func() { os.RemoveAll(target) })
	from, _ := os.Stat(source)
	to, _ := os.Stat(target)
	if from.Sys().(*syscall.Stat_t).Dev == to.Sys().(*syscall.Stat_t).Dev {
		t.Skip("fixture needs two filesystems")
	}
	putFile(t, filepath.Join(source, "logs/core/example.log"))
	selectMovedDataDir(t, path, target)
	if _, err := PrepareDataLocation(t.Context(), path); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(target, "logs/core/example.log"))
	if err != nil || string(data) != "fixture" {
		t.Fatal("cross-filesystem move lost data", err)
	}
}
