//go:build !webdist

// SPDX-License-Identifier: GPL-3.0-or-later

// Package webui exposes the embedded panel user interface.
package webui

import (
	"embed"
	"io/fs"
)

//go:embed fallback/*
var fallbackFiles embed.FS

// Assets returns a minimal development fallback. Release builds use the
// `webdist` build tag after Vite has produced web/dist.
func Assets() fs.FS {
	assets, err := fs.Sub(fallbackFiles, "fallback")
	if err != nil {
		panic(err)
	}
	return assets
}
