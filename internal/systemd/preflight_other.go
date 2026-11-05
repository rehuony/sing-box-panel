// SPDX-License-Identifier: GPL-3.0-or-later

//go:build !linux && !darwin

package systemd

func writableParent(string) error { return ErrUnsupportedOS }

func writableDirectory(string) error { return ErrUnsupportedOS }
