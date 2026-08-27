// SPDX-License-Identifier: GPL-3.0-or-later

package buildinfo

import (
	"runtime/debug"
	"testing"
)

func TestCurrentUsesInjectedMetadata(t *testing.T) {
	previousVersion, previousCommit, previousDate := version, commit, date
	version = "v1.2.3"
	commit = "0123456789abcdef"
	date = "2026-08-27T00:00:00Z"
	t.Cleanup(func() {
		version, commit, date = previousVersion, previousCommit, previousDate
	})

	want := Info{Version: version, Commit: commit, Date: date}
	if got := Current(); got != want {
		t.Fatalf("Current() = %+v, want %+v", got, want)
	}
}

func TestResolve(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		initial Info
		details *debug.BuildInfo
		want    Info
	}{
		{
			name:    "development metadata from VCS",
			initial: Info{Version: "dev", Commit: "unknown", Date: "unknown"},
			details: &debug.BuildInfo{
				Main: debug.Module{Version: "(devel)"},
				Settings: []debug.BuildSetting{
					{Key: "vcs.revision", Value: "0123456789abcdef"},
					{Key: "vcs.time", Value: "2026-08-27T00:00:00Z"},
				},
			},
			want: Info{Version: "dev", Commit: "0123456789abcdef", Date: "2026-08-27T00:00:00Z"},
		},
		{
			name:    "module version fills development version",
			initial: Info{Version: "dev", Commit: "unknown", Date: "unknown"},
			details: &debug.BuildInfo{Main: debug.Module{Version: "v1.2.3"}},
			want:    Info{Version: "v1.2.3", Commit: "unknown", Date: "unknown"},
		},
		{
			name:    "linker metadata wins",
			initial: Info{Version: "v2.0.0", Commit: "release-commit", Date: "release-date"},
			details: &debug.BuildInfo{
				Main: debug.Module{Version: "v1.2.3"},
				Settings: []debug.BuildSetting{
					{Key: "vcs.revision", Value: "vcs-commit"},
					{Key: "vcs.time", Value: "vcs-date"},
				},
			},
			want: Info{Version: "v2.0.0", Commit: "release-commit", Date: "release-date"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if got := resolve(test.initial, test.details); got != test.want {
				t.Fatalf("resolve() = %+v, want %+v", got, test.want)
			}
		})
	}
}
