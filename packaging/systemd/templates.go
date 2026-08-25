// SPDX-License-Identifier: GPL-3.0-or-later

// Package systemdassets exposes the audited systemd packaging templates to
// the built-in installer. Keeping the installer and release artifacts on the
// same bytes prevents their security settings from drifting apart.
package systemdassets

import _ "embed"

var (
	// SystemUnit is the hardened system service template.
	//go:embed system/sing-box-panel.service
	SystemUnit []byte

	// UserUnit is the hardened per-user service template.
	//go:embed user/sing-box-panel.service
	UserUnit []byte

	// Sysusers is the dedicated system-account declaration.
	//go:embed sysusers.d/sing-box-panel.conf
	Sysusers []byte

	// Tmpfiles is the system configuration/data directory declaration.
	//go:embed tmpfiles.d/sing-box-panel.conf
	Tmpfiles []byte
)
