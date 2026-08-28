// SPDX-License-Identifier: GPL-3.0-or-later

package subscription

import (
	"errors"
	"reflect"
	"testing"
)

type fakeConverter struct{ version string }

func (converter fakeConverter) ExactVersion() string { return converter.version }

func (fakeConverter) Convert(InboundRequest) (InboundResult, error) { return InboundResult{}, nil }

func TestRegistryDispatchesOnlyExactRegisteredVersions(t *testing.T) {
	registry := MustNewInboundRegistry(fakeConverter{version: "1.13.19"})
	if got, want := registry.Versions(), []string{"1.13.19"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("Versions = %v, want %v", got, want)
	}
	if _, err := registry.Convert("1.13.19", InboundRequest{}); err != nil {
		t.Fatalf("Convert(registered): %v", err)
	}
	for _, version := range []string{"1.13.20", "1.13", "v1.13.19"} {
		if _, err := registry.Convert(version, InboundRequest{}); !errors.Is(err, ErrUnsupportedCoreVersion) {
			t.Fatalf("Convert(%q) error = %v, want ErrUnsupportedCoreVersion", version, err)
		}
	}
}

func TestMustNewRegistryRejectsInvalidComposition(t *testing.T) {
	tests := []struct {
		name       string
		converters []InboundConverter
	}{
		{name: "nil", converters: []InboundConverter{nil}},
		{name: "invalid", converters: []InboundConverter{fakeConverter{version: "1.13"}}},
		{name: "duplicate", converters: []InboundConverter{fakeConverter{version: "1.13.19"}, fakeConverter{version: "1.13.19"}}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Fatal("MustNewInboundRegistry() did not panic")
				}
			}()
			MustNewInboundRegistry(test.converters...)
		})
	}
}
