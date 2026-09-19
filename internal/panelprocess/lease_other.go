// SPDX-License-Identifier: GPL-3.0-or-later

//go:build !darwin && !linux

package panelprocess

import "os"

func openAndLockRuntimeExecutorLease(string) (*os.File, error) {
	return nil, ErrLeaseUnsupported
}

func unlockAndCloseRuntimeExecutorLease(file *os.File) error {
	return file.Close()
}
