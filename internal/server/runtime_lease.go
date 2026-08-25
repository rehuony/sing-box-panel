// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

const runtimeExecutorLeaseName = "runtime-executor.lock"

var (
	errRuntimeExecutorLeaseHeld        = errors.New("runtime executor lease is already held")
	errRuntimeExecutorLeaseUnsupported = errors.New("runtime executor lease is unsupported on this platform")
)

// runtimeExecutorLease is held for the complete server lifetime. The lock is
// deliberately outside SQLite: task leases coordinate durable work, while
// this lease guarantees that only one OS process can own the sing-box runtime
// manager for a data directory.
type runtimeExecutorLease struct {
	file *os.File
	once sync.Once
	err  error
}

func acquireRuntimeExecutorLease(dataDirectory string) (*runtimeExecutorLease, error) {
	path := filepath.Join(dataDirectory, runtimeExecutorLeaseName)
	file, err := openAndLockRuntimeExecutorLease(path)
	if err != nil {
		if errors.Is(err, errRuntimeExecutorLeaseHeld) {
			return nil, fmt.Errorf(
				"acquire runtime executor lease %q: another sing-box-panel process owns this data directory: %w",
				path,
				err,
			)
		}
		return nil, fmt.Errorf("acquire runtime executor lease %q: %w", path, err)
	}
	return &runtimeExecutorLease{file: file}, nil
}

func (lease *runtimeExecutorLease) Close() error {
	if lease == nil || lease.file == nil {
		return nil
	}
	lease.once.Do(func() {
		lease.err = unlockAndCloseRuntimeExecutorLease(lease.file)
	})
	return lease.err
}
