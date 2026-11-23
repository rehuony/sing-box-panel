// SPDX-License-Identifier: GPL-3.0-or-later

//go:build unix

package atomicfile

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"
)

// PreserveOwner gives file, the temporary file that replaces path, the owner
// of path, or of its directory when path does not exist yet. A root command
// line writer then keeps a service-owned file readable by that service;
// unprivileged writers keep their own owner. Use it as Options.Prepare.
func PreserveOwner(file *os.File, path string) error {
	if os.Geteuid() != 0 {
		return nil
	}
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		info, err = os.Stat(filepath.Dir(path))
	}
	if err != nil {
		return err
	}
	if stat, ok := info.Sys().(*syscall.Stat_t); ok {
		return file.Chown(int(stat.Uid), int(stat.Gid))
	}
	return nil
}
