// SPDX-License-Identifier: GPL-3.0-or-later

//go:build unix

package artifactstore

import (
	"os"
	"syscall"
)

func fileOwner(info os.FileInfo) (uint64, bool) {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, false
	}
	return uint64(stat.Uid), true
}

func effectiveUserID() (uint64, bool) {
	return uint64(os.Geteuid()), true
}
