// SPDX-License-Identifier: GPL-3.0-or-later

// Package buildinfo exposes linker-injected release metadata.
package buildinfo

import "runtime/debug"

var (
	version = "devel"
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
	if result.Version != "devel" {
		return result
	}
	if details, ok := debug.ReadBuildInfo(); ok {
		if details.Main.Version != "" && details.Main.Version != "(devel)" {
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
	}
	return result
}
