//go:build unix

// SPDX-License-Identifier: GPL-3.0-or-later

package corelogs

import (
	"golang.org/x/sys/unix"
	"os"
)

func openOutputFile(path string) (*os.File, os.FileInfo, error) {
	before, err := os.Lstat(path)
	if err != nil {
		return nil, nil, err
	}
	if !before.Mode().IsRegular() {
		return nil, nil, ErrInvalidFile
	}
	fd, err := unix.Open(path, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, nil, err
	}
	file := os.NewFile(uintptr(fd), path)
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || !os.SameFile(before, info) {
		_ = file.Close()
		return nil, nil, ErrInvalidFile
	}
	return file, info, nil
}
