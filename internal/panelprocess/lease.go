// SPDX-License-Identifier: GPL-3.0-or-later

package panelprocess

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

const LeaseFileName = "runtime-executor.lock"

var (
	ErrLeaseHeld        = errors.New("runtime executor lease is already held")
	ErrLeaseUnsupported = errors.New("runtime executor lease is unsupported on this platform")
)

// Lease is held for the complete server lifetime. The lock is
// deliberately outside SQLite: runtime generations fence committed state, while
// this lease guarantees that only one OS process can own the sing-box runtime
// manager for a data directory.
type Lease struct {
	file *os.File
	once sync.Once
	err  error
}

func AcquireLease(dataDirectory string) (*Lease, error) {
	path := filepath.Join(dataDirectory, LeaseFileName)
	file, err := openAndLockRuntimeExecutorLease(path)
	if err != nil {
		if errors.Is(err, ErrLeaseHeld) {
			return nil, fmt.Errorf(
				"acquire runtime executor lease %q: another sing-box-panel process owns this data directory: %w",
				path,
				err,
			)
		}
		return nil, fmt.Errorf("acquire runtime executor lease %q: %w", path, err)
	}
	return &Lease{file: file}, nil
}

func (lease *Lease) Close() error {
	if lease == nil || lease.file == nil {
		return nil
	}
	lease.once.Do(func() {
		lease.err = unlockAndCloseRuntimeExecutorLease(lease.file)
	})
	return lease.err
}
