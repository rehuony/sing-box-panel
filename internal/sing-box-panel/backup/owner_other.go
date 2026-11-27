// SPDX-License-Identifier: GPL-3.0-or-later

//go:build !unix

package backup

func matchOwner(string, string) error { return nil }
