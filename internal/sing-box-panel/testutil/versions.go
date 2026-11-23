// SPDX-License-Identifier: GPL-3.0-or-later

package testutil

import (
	"testing"

	"github.com/rehuony/sing-box-panel/internal/singbox"
)

// Reviewed113Versions returns the catalog's exact versions that use the
// reviewed 1.13 Schema.
func Reviewed113Versions(t *testing.T) []string {
	t.Helper()
	var versions []string
	for _, version := range singbox.Versions() {
		if version.SchemaSource == singbox.SchemaSourceReviewed113 {
			versions = append(versions, version.ExactVersion)
		}
	}
	if len(versions) == 0 {
		t.Fatal("no reviewed 1.13 versions in the catalog")
	}
	return versions
}
