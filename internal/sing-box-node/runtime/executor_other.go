//go:build !linux

// SPDX-License-Identifier: GPL-3.0-or-later

package runtime

func newDefaultCommandExecutor() (CommandExecutor, error) {
	return nil, ErrUnavailable
}
