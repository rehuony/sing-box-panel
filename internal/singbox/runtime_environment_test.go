// SPDX-License-Identifier: GPL-3.0-or-later

package singbox

import "testing"

func TestRuntimeCompatibilityEnvironmentIsExactVersionScoped(t *testing.T) {
	var reviewed []string
	for _, version := range Versions() {
		if version.SchemaSource == SchemaSourceReviewed113 {
			reviewed = append(reviewed, version.ExactVersion)
		}
	}
	if len(reviewed) == 0 {
		t.Fatal("no reviewed 1.13 versions in the catalog")
	}
	for _, version := range reviewed {
		environment := RuntimeCompatibilityEnvironment(version)
		if len(environment) != 5 {
			t.Fatalf("%s compatibility switches = %v", version, environment)
		}
		environment[0] = "modified"
		if RuntimeCompatibilityEnvironment(version)[0] == "modified" {
			t.Fatal("environment shares mutable backing storage")
		}
	}
	for _, version := range []string{"1.13.18", "1.13.22", "1.14.0", "v1.13.21", ""} {
		if env := RuntimeCompatibilityEnvironment(version); len(env) != 0 {
			t.Fatalf("%s received legacy switches: %v", version, env)
		}
	}
}
