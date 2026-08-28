// SPDX-License-Identifier: GPL-3.0-or-later

//go:build darwin || linux

package selfupdate

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/sys/unix"
)

const updateLockRetryInterval = 50 * time.Millisecond

type updateLock struct {
	file *os.File
}

func acquireUpdateLock(ctx context.Context, targetPath string) (*updateLock, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	lockPath := filepath.Join(filepath.Dir(targetPath), "."+filepath.Base(targetPath)+".update.lock")
	fileDescriptor, err := unix.Open(
		lockPath,
		unix.O_CLOEXEC|unix.O_CREAT|unix.O_NOFOLLOW|unix.O_RDWR,
		0o600,
	)
	if err != nil {
		return nil, fmt.Errorf("open lock file: %w", err)
	}
	file := os.NewFile(uintptr(fileDescriptor), lockPath)
	if file == nil {
		_ = unix.Close(fileDescriptor)
		return nil, errors.New("open lock file: invalid file descriptor")
	}
	closeOnError := func(err error) (*updateLock, error) {
		_ = file.Close()
		return nil, err
	}
	info, err := file.Stat()
	if err != nil {
		return closeOnError(fmt.Errorf("inspect lock file: %w", err))
	}
	if !info.Mode().IsRegular() {
		return closeOnError(errors.New("lock path is not a regular file"))
	}
	if err := file.Chmod(0o600); err != nil {
		return closeOnError(fmt.Errorf("secure lock file permissions: %w", err))
	}

	ticker := time.NewTicker(updateLockRetryInterval)
	defer ticker.Stop()
	for {
		err = unix.Flock(fileDescriptor, unix.LOCK_EX|unix.LOCK_NB)
		if err == nil {
			return &updateLock{file: file}, nil
		}
		if !errors.Is(err, unix.EWOULDBLOCK) && !errors.Is(err, unix.EAGAIN) {
			return closeOnError(fmt.Errorf("lock executable: %w", err))
		}
		select {
		case <-ctx.Done():
			return closeOnError(ctx.Err())
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
