// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux || darwin

package server

import "syscall"

// restartSupported reports that the panel can re-execute itself here.
const restartSupported = true

func execSelf(path string, args, env []string) error {
	return syscall.Exec(path, args, env)
}
