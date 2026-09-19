// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux || darwin

package store

import (
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

// Lock the directory inode itself, so maintenance coordination creates no
// persistent lock file. Every database owner holds a shared lock until Close.
func lockDataDirectory(path string, exclusive bool) (*os.File, error) {
	fd, err := unix.Open(path, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(fd), path)
	operation := unix.LOCK_SH | unix.LOCK_NB
	if exclusive {
		operation = unix.LOCK_EX | unix.LOCK_NB
	}
	if err := unix.Flock(fd, operation); err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("instance data directory is busy with another command or cleanup: %w", err)
	}
	return file, nil
}
