// SPDX-License-Identifier: GPL-3.0-or-later

// Package releaseevidence exposes the immutable evidence ledger embedded in
// the release-readiness command. It deliberately contains no policy logic;
// internal/releasegate owns validation and the list of required evidence.
package releaseevidence

import "embed"

// files includes the ledger and all repository-reviewed evidence documents.
//
//go:embed evidence.json evidence
var files embed.FS

func Manifest() ([]byte, error) {
	return files.ReadFile("evidence.json")
}

func Read(path string) ([]byte, error) {
	return files.ReadFile(path)
}
