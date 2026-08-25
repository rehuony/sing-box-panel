// SPDX-License-Identifier: GPL-3.0-or-later

package coreartifact

import (
	"errors"
	"testing"
)

func TestParseExactVersion(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		value string
		want  string
		valid bool
	}{
		{name: "stable", value: "1.13.19", want: "1.13.19", valid: true},
		{name: "zero components", value: "10.0.2", want: "10.0.2", valid: true},
		{name: "leading v", value: "v1.13.19"},
		{name: "prerelease", value: "1.14.0-beta.1"},
		{name: "build metadata", value: "1.13.19+linux"},
		{name: "missing patch", value: "1.13"},
		{name: "extra component", value: "1.13.19.0"},
		{name: "leading zero", value: "1.013.19"},
		{name: "negative", value: "1.-1.0"},
		{name: "overflow", value: "18446744073709551616.0.1"},
		{name: "empty", value: ""},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			version, err := ParseExactVersion(test.value)
			if !test.valid {
				if !errors.Is(err, ErrInvalidExactVersion) {
					t.Fatalf("ParseExactVersion(%q) error = %v, want ErrInvalidExactVersion", test.value, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseExactVersion(%q): %v", test.value, err)
			}
			if got := version.String(); got != test.want {
				t.Fatalf("ParseExactVersion(%q).String() = %q, want %q", test.value, got, test.want)
			}
		})
	}
}

func TestExactVersionCompare(t *testing.T) {
	t.Parallel()

	if got := NewExactVersion(1, 13, 19).Compare(NewExactVersion(1, 14, 0)); got >= 0 {
		t.Fatalf("1.13.19.Compare(1.14.0) = %d, want a negative result", got)
	}
	if got := NewExactVersion(2, 0, 0).Compare(NewExactVersion(1, 99, 99)); got <= 0 {
		t.Fatalf("2.0.0.Compare(1.99.99) = %d, want a positive result", got)
	}
	if got := NewExactVersion(1, 13, 19).Compare(NewExactVersion(1, 13, 19)); got != 0 {
		t.Fatalf("equal version comparison = %d, want 0", got)
	}
}

func FuzzParseExactVersionCanonicalRoundTrip(f *testing.F) {
	for _, seed := range []string{"1.13.19", "0.1.0", "v1.2.3", "1.2.3-beta.1", "01.2.3", ""} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, input string) {
		version, err := ParseExactVersion(input)
		if err != nil {
			return
		}
		canonical := version.String()
		reparsed, err := ParseExactVersion(canonical)
		if err != nil {
			t.Fatalf("ParseExactVersion accepted %q but rejected canonical %q: %v", input, canonical, err)
		}
		if reparsed != version {
			t.Fatalf("canonical round trip for %q = %v, want %v", input, reparsed, version)
		}
	})
}
