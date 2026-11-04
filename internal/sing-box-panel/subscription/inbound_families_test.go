// SPDX-License-Identifier: GPL-3.0-or-later

package subscription

import (
	"reflect"
	"testing"

	"github.com/rehuony/sing-box-panel/internal/singbox"
)

func TestCompiledInboundRegistryMatchesDeclaredCapability(t *testing.T) {
	t.Parallel()
	if err := ValidateFamilies(); err != nil {
		t.Fatal(err)
	}
	versions := singbox.Versions()
	want := make([]string, 0, len(versions))
	for _, version := range versions {
		if version.InboundFamily != "" {
			want = append(want, version.ExactVersion)
		}
	}
	if got := NewInboundRegistry().Versions(); !reflect.DeepEqual(got, want) {
		t.Fatalf("inbound versions = %v, want %v", got, want)
	}
}

func TestUnknownInboundFamilyIsRejected(t *testing.T) {
	t.Parallel()
	versions := singbox.Versions()
	versions[0].InboundFamily = "1.11-r2"
	if err := validateCompiledFamilies(versions); err == nil {
		t.Fatal("unknown behavior family was accepted")
	}
}
