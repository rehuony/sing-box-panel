// SPDX-License-Identifier: GPL-3.0-or-later

package releasegate

import "testing"

func TestSQLiteReleaseGateReflectsEmbeddedVersion(t *testing.T) {
	status := SQLite()
	if status.Current == "" || status.Minimum != "3.53.4" {
		t.Fatalf("SQLite() = %+v", status)
	}
	if status.Ready != (compareVersions(status.Current, status.Minimum) >= 0) {
		t.Fatalf("SQLite() readiness is inconsistent: %+v", status)
	}
}

func TestCompareVersions(t *testing.T) {
	tests := []struct {
		left, right string
		want        int
	}{
		{"3.53.3", "3.53.4", -1},
		{"3.53.4", "3.53.4", 0},
		{"3.54.0", "3.53.4", 1},
		{"unknown", "3.53.4", -1},
		{"03.54.0", "3.53.4", -1},
		{"3.54.0", "invalid", -1},
	}
	for _, test := range tests {
		got := compareVersions(test.left, test.right)
		if got != test.want {
			t.Errorf("compareVersions(%q, %q) = %d, want %d", test.left, test.right, got, test.want)
		}
	}
}
