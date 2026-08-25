//go:build webdist

// SPDX-License-Identifier: GPL-3.0-or-later

package webui

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var distributionFiles embed.FS

// Assets returns the immutable Vite production distribution.
func Assets() fs.FS {
	assets, err := fs.Sub(distributionFiles, "dist")
	if err != nil {
		panic(err)
	}
	return assets
}
