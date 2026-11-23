// SPDX-License-Identifier: GPL-3.0-or-later

package procctl

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

const LeaseFileName = "process.lock"

var (
	ErrLeaseHeld        = errors.New("data directory lease is already held")
	ErrLeaseUnsupported = errors.New("data directory lease is unsupported on this platform")
)

// Lease is held for the complete lifetime of a panel or node process. The
// lock is deliberately outside SQLite: on a node, runtime generations fence
// committed state, while this lease guarantees that only one OS process owns
// a data directory and, there, the sing-box runtime manager.
type Lease struct {
	file *os.File
	once sync.Once
	err  error
}

func AcquireLease(dataDirectory string) (*Lease, error) {
	path := filepath.Join(dataDirectory, LeaseFileName)
	file, err := openAndLockLease(path)
	if err != nil {
		if errors.Is(err, ErrLeaseHeld) {
			return nil, fmt.Errorf(
				"acquire data directory lease %q: another process owns this data directory: %w",
				path,
				err,
			)
		}
		return nil, fmt.Errorf("acquire data directory lease %q: %w", path, err)
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

// LeaseHeld reports whether a process owns the data directory now. It never
// creates the lease file, so a command run as another user leaves no file
// the owner cannot open.
func LeaseHeld(dataDirectory string) (bool, error) {
	return leaseHeld(filepath.Join(dataDirectory, LeaseFileName))
}
