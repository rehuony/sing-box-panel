// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux || darwin

package settings

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

// Lock serializes panel and CLI writers across atomic file replacements. The
// lock file is retained so a waiting writer never locks an unlinked inode.
func Lock(ctx context.Context, path string) (*os.File, error) { return lockFile(ctx, path, true) }

// TryLock refuses a busy settings writer so cleanup never waits on an owner it
// is about to remove.
func TryLock(ctx context.Context, path string) (*os.File, error) { return lockFile(ctx, path, false) }

func lockFile(ctx context.Context, path string, wait bool) (*os.File, error) {
	fd, err := unix.Open(path+".lock", unix.O_RDWR|unix.O_CREAT|unix.O_CLOEXEC|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0600)
	if err != nil {
		return nil, fmt.Errorf("open settings lock: %w", err)
	}
	file := os.NewFile(uintptr(fd), path+".lock")
	if err := preserveOwner(file, path); err != nil {
		file.Close()
		return nil, err
	}
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		file.Close()
		return nil, errors.New("settings lock must be a regular file")
	}
	for {
		if err := ctx.Err(); err != nil {
			file.Close()
			return nil, err
		}
		err := unix.Flock(fd, unix.LOCK_EX|unix.LOCK_NB)
		if err == nil {
			current, statErr := os.Lstat(path + ".lock")
			if statErr != nil || !os.SameFile(info, current) {
				file.Close()
				return nil, errors.New("settings lock changed during cleanup; retry after inspecting the instance")
			}
			return file, nil
		}
		if !errors.Is(err, unix.EWOULDBLOCK) && !errors.Is(err, unix.EAGAIN) {
			file.Close()
			return nil, err
		}
		if !wait {
			file.Close()
			return nil, errors.New("settings file is busy with another writer")
		}
		select {
		case <-ctx.Done():
			file.Close()
			return nil, ctx.Err()
		case <-time.After(10 * time.Millisecond):
		}
	}
}

// Root CLI writes must keep a service-owned settings file readable by that
// service after atomic replacement. Unprivileged writers keep their own owner.
func preserveOwner(file *os.File, path string) error {
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
