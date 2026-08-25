// SPDX-License-Identifier: GPL-3.0-or-later

//go:build darwin || linux

package server

import (
	"errors"
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

func openAndLockRuntimeExecutorLease(path string) (*os.File, error) {
	fd, err := unix.Open(path, unix.O_CREAT|unix.O_RDWR|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0o600)
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(fd), path)
	if file == nil {
		_ = unix.Close(fd)
		return nil, errors.New("construct runtime executor lease file")
	}
	if err := unix.Flock(fd, unix.LOCK_EX|unix.LOCK_NB); err != nil {
		_ = file.Close()
		if errors.Is(err, unix.EWOULDBLOCK) || errors.Is(err, unix.EAGAIN) {
			return nil, errRuntimeExecutorLeaseHeld
		}
		return nil, err
	}
	if err := file.Chmod(0o600); err != nil {
		_ = unix.Flock(fd, unix.LOCK_UN)
		_ = file.Close()
		return nil, err
	}
	return file, nil
}

func unlockAndCloseRuntimeExecutorLease(file *os.File) error {
	unlockErr := unix.Flock(int(file.Fd()), unix.LOCK_UN)
	closeErr := file.Close()
	if unlockErr != nil || closeErr != nil {
		return fmt.Errorf("release runtime executor lease: %w", errors.Join(unlockErr, closeErr))
	}
	return nil
}
