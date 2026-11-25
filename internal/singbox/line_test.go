// SPDX-License-Identifier: GPL-3.0-or-later

package singbox

import "testing"

func TestLineOfResolvesByReleaseLine(t *testing.T) {
	t.Parallel()
	for version, want := range map[string]string{
		"1.13.19": "1.13", "1.13.22": "1.13", "1.14.0": "1.14", "1.14.4": "1.14", "1.15.0": "1.14",
		"1.13.18": "", "1.12.25": "", "1.14": "", "v1.14.1": "", "1.14.1-beta.1": "", "": "",
	} {
		line, found := LineOf(version)
		if line.Name != want || found != (want != "") {
			t.Errorf("LineOf(%q) = %q, %v; want %q", version, line.Name, found, want)
		}
	}
}

func TestRuntimeCompatibilityEnvironmentFollowsTheLine(t *testing.T) {
	t.Parallel()
	for _, version := range []string{"1.13.19", "1.13.22"} {
		if environment := RuntimeCompatibilityEnvironment(version); len(environment) != 5 {
			t.Fatalf("%s compatibility switches = %v", version, environment)
		}
	}
	for _, version := range []string{"1.13.18", "1.14.0", "1.14.4", "v1.13.21", ""} {
		if environment := RuntimeCompatibilityEnvironment(version); len(environment) != 0 {
			t.Fatalf("%s received legacy switches: %v", version, environment)
		}
	}
}
