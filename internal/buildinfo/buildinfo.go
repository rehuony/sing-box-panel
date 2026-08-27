// SPDX-License-Identifier: GPL-3.0-or-later

// Package buildinfo exposes linker-injected release metadata with VCS
// fallbacks for local development builds.
package buildinfo

import "runtime/debug"

var (
	version = "dev"
	commit  = "unknown"
	date    = "unknown"
)

type Info struct {
	Version string `json:"version"`
	Commit  string `json:"commit"`
	Date    string `json:"date"`
}

func Current() Info {
	result := Info{Version: version, Commit: commit, Date: date}
	if details, ok := debug.ReadBuildInfo(); ok {
		result = resolve(result, details)
	}
	return result
}

func resolve(result Info, details *debug.BuildInfo) Info {
	if result.Version == "dev" && details.Main.Version != "" && details.Main.Version != "(devel)" {
		result.Version = details.Main.Version
	}
	for _, setting := range details.Settings {
		switch setting.Key {
		case "vcs.revision":
			if result.Commit == "unknown" {
				result.Commit = setting.Value
			}
		case "vcs.time":
			if result.Date == "unknown" {
				result.Date = setting.Value
			}
		}
	}
	return result
}
