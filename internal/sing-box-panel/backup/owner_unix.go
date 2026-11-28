// SPDX-License-Identifier: GPL-3.0-or-later

//go:build unix

package backup

import (
	"os"
	"syscall"
)

// matchOwner gives path the owner of reference when root writes it, so that
// a backup or restore the command line makes as root stays usable by the
// service account that owns the data directory.
func matchOwner(path, reference string) error {
	if os.Geteuid() != 0 {
		return nil
	}
	info, err := os.Stat(reference)
	if err != nil {
		return err
	}
	if stat, ok := info.Sys().(*syscall.Stat_t); ok {
		return os.Lchown(path, int(stat.Uid), int(stat.Gid))
	}
	return nil
}
