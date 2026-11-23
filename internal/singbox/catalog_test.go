// SPDX-License-Identifier: GPL-3.0-or-later

package singbox

import (
	"reflect"
	"testing"
)

func TestCatalogLookupRequiresExactVersion(t *testing.T) {
	t.Parallel()
	for _, want := range Versions() {
		version, found := Lookup(want.ExactVersion)
		if !found || !reflect.DeepEqual(version, want) {
			t.Fatalf("Lookup(%s) = %+v, %v; want %+v", want.ExactVersion, version, found, want)
		}
	}
	for _, unreviewed := range []string{"1.14", "99.0.0", "v1.14.1", "1.14.1-beta.1"} {
		if _, found := Lookup(unreviewed); found {
			t.Fatalf("Lookup(%s) accepted an unreviewed version", unreviewed)
		}
	}
}

func TestCatalogAccessorsReturnDefensiveCopies(t *testing.T) {
	t.Parallel()
	versions := Versions()
	profile := versions[0].Profiles[ArchitectureARM64]
	profile.Features[0] = "changed"
	versions[0].Profiles[ArchitectureARM64] = profile
	again, ok := Lookup("1.13.19")
	if !ok {
		t.Fatal("compiled version disappeared")
	}
	if again.Profiles[ArchitectureARM64].Features[0] == "changed" {
		t.Fatal("catalog accessor exposed mutable compiled storage")
	}
}
