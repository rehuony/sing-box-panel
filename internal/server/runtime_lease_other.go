// SPDX-License-Identifier: GPL-3.0-or-later

//go:build !darwin && !linux

package server

import "os"

func openAndLockRuntimeExecutorLease(string) (*os.File, error) {
	return nil, errRuntimeExecutorLeaseUnsupported
}

func unlockAndCloseRuntimeExecutorLease(file *os.File) error {
	return file.Close()
}
