// SPDX-License-Identifier: GPL-3.0-or-later

//go:build !linux && !darwin

package server

import "errors"

// restartSupported reports that the panel cannot re-execute itself here.
const restartSupported = false

func execSelf(string, []string, []string) error {
	return errors.New("restarting the panel needs Linux or macOS")
}
