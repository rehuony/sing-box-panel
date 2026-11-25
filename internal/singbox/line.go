// SPDX-License-Identifier: GPL-3.0-or-later

// Package singbox owns the sing-box support policy: the release lines the
// panel and node support and what each line needs from them.
package singbox

import "github.com/rehuony/sing-box-panel/internal/coreartifact"

// Line is a supported sing-box release line.
type Line struct {
	// Name is the line's major.minor, such as 1.14.
	Name string
	// NativeSchema reports that the line's binaries print their own
	// configuration JSON Schema with `sing-box schema`. A line without it
	// uses the reviewed schema embedded in the panel.
	NativeSchema bool
	// InboundFamily names the subscription inbound converter of the line.
	InboundFamily string
	// LegacyDNSEnvironment reports that the line needs the environment
	// switches that keep its deprecated DNS forms working.
	LegacyDNSEnvironment bool

	first coreartifact.ExactVersion
}

// lines are ordered by first version. A line covers its first version and
// every later version up to the next line's first.
var lines = []Line{
	{Name: "1.13", InboundFamily: "1.13", LegacyDNSEnvironment: true, first: coreartifact.NewExactVersion(1, 13, 19)},
	{Name: "1.14", NativeSchema: true, InboundFamily: "1.14", first: coreartifact.NewExactVersion(1, 14, 0)},
}

// LineOf returns the line of a canonical major.minor.patch version: the
// newest line whose first version is not above it. A version below the
// first line has none.
func LineOf(exactVersion string) (Line, bool) {
	version, err := coreartifact.ParseExactVersion(exactVersion)
	if err != nil {
		return Line{}, false
	}
	for index := len(lines) - 1; index >= 0; index-- {
		if version.Compare(lines[index].first) >= 0 {
			return lines[index], true
		}
	}
	return Line{}, false
}
