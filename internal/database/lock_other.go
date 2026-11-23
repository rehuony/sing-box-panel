// SPDX-License-Identifier: GPL-3.0-or-later

//go:build !linux && !darwin

package database

import (
	"errors"
	"os"
)

func lockDirectory(_ string, exclusive bool) (*os.File, error) {
	if exclusive {
		return nil, errors.New("instance cleanup requires Linux or macOS directory locking")
	}
	return nil, nil
}
