// SPDX-License-Identifier: GPL-3.0-or-later

//go:build darwin || linux

package selfupdate

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"golang.org/x/sys/unix"
)

const updateLockRetryInterval = 50 * time.Millisecond

type updateLock struct {
	file *os.File
}

// acquireUpdateLock locks the executable at targetPath itself, so updates of
// one executable take turns without a lock file beside it. An install
// replaces the path, not the locked file, so an updater that waited compares
// the executable's digest under the lock with the one it started from.
func acquireUpdateLock(ctx context.Context, targetPath string) (*updateLock, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	fileDescriptor, err := unix.Open(targetPath, unix.O_CLOEXEC|unix.O_NOFOLLOW|unix.O_RDONLY, 0)
	if err != nil {
		return nil, fmt.Errorf("open executable to lock it: %w", err)
	}
	file := os.NewFile(uintptr(fileDescriptor), targetPath)
	if file == nil {
		_ = unix.Close(fileDescriptor)
		return nil, errors.New("open executable to lock it: invalid file descriptor")
	}
	ticker := time.NewTicker(updateLockRetryInterval)
	defer ticker.Stop()
	for {
		err = unix.Flock(fileDescriptor, unix.LOCK_EX|unix.LOCK_NB)
		if err == nil {
			return &updateLock{file: file}, nil
		}
		if !errors.Is(err, unix.EWOULDBLOCK) && !errors.Is(err, unix.EAGAIN) {
			_ = file.Close()
			return nil, fmt.Errorf("lock executable: %w", err)
		}
		select {
		case <-ctx.Done():
			_ = file.Close()
			return nil, ctx.Err()
		case <-ticker.C:
		}
	}
}

func (lock *updateLock) Close() error {
	if lock == nil || lock.file == nil {
		return nil
	}
	unlockErr := unix.Flock(int(lock.file.Fd()), unix.LOCK_UN)
	closeErr := lock.file.Close()
	lock.file = nil
	return errors.Join(unlockErr, closeErr)
}
