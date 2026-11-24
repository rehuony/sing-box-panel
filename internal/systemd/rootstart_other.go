// SPDX-License-Identifier: GPL-3.0-or-later

//go:build !linux && !darwin

package systemd

// CheckRootStart has no file owners to compare where systemd is unsupported.
func CheckRootStart(string, ...string) error { return nil }
