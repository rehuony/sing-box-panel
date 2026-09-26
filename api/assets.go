// SPDX-License-Identifier: GPL-3.0-or-later

// Package apiassets owns configuration presentation contracts shared by the web
// editor and subscription renderer. Explicit user templates remain authoritative.
package apiassets

import _ "embed"

//go:embed templates/sing-box.json
var SingBoxTemplate string

//go:embed templates/mihomo.yaml
var MihomoTemplate string

//go:embed templates/loon.conf
var LoonTemplate string

//go:embed configuration-order.json
var ConfigurationOrder []byte
