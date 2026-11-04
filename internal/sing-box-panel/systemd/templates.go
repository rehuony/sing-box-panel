// SPDX-License-Identifier: GPL-3.0-or-later

package systemd

import _ "embed"

// The audited packaging templates. The installer and the release artifacts use
// the same bytes, so their security settings cannot drift apart.
var (
	// systemUnitTemplate is the hardened system service template.
	//go:embed templates/system/sing-box-panel.service
	systemUnitTemplate []byte

	// userUnitTemplate is the hardened per-user service template.
	//go:embed templates/user/sing-box-panel.service
	userUnitTemplate []byte

	// sysusersTemplate is the dedicated system-account declaration.
	//go:embed templates/sysusers.d/sing-box-panel.conf
	sysusersTemplate []byte

	// tmpfilesTemplate is the system configuration/data directory declaration.
	//go:embed templates/tmpfiles.d/sing-box-panel.conf
	tmpfilesTemplate []byte
)
