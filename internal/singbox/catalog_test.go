// SPDX-License-Identifier: GPL-3.0-or-later

package singbox

import (
	"reflect"
	"testing"
)

func TestCatalogAccessorsReturnDefensiveCopies(t *testing.T) {
	t.Parallel()
	versions := Versions()
	profile := versions[0].Profiles[ArchitectureARM64]
	profile.Features[0] = "changed"
	versions[0].Profiles[ArchitectureARM64] = profile
	again, ok := Lookup("1.11.15")
	if !ok {
		t.Fatal("compiled version disappeared")
	}
	if again.Profiles[ArchitectureARM64].Features[0] == "changed" {
		t.Fatal("catalog accessor exposed mutable compiled storage")
	}
}

func TestCompiledRegistriesMatchCatalog(t *testing.T) {
	t.Parallel()
	if err := ValidateFamilies(); err != nil {
		t.Fatal(err)
	}
	versions := Versions()
	want := make([]string, len(versions))
	for index, version := range versions {
		want[index] = version.ExactVersion
	}
	if got := NewConfigurationRegistry().Versions(); !reflect.DeepEqual(got, want) {
		t.Fatalf("configuration versions = %v, want %v", got, want)
	}
	if got := NewInboundRegistry().Versions(); !reflect.DeepEqual(got, want) {
		t.Fatalf("inbound versions = %v, want %v", got, want)
	}
}

func TestUnknownBehaviorFamilyIsRejected(t *testing.T) {
	t.Parallel()
	versions := Versions()
	versions[0].Family = "1.11-r2"
	if err := validateCompiledFamilies(versions); err == nil {
		t.Fatal("unknown behavior family was accepted")
	}
}
