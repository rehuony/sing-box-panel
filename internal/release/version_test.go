// SPDX-License-Identifier: GPL-3.0-or-later

package release

import (
	"errors"
	"testing"
)

func TestValidate(t *testing.T) {
	t.Parallel()
	tests := []struct {
		value string
		valid bool
	}{
		{value: "v0.1.0", valid: true},
		{value: "v12.34.56-rc.1+linux.amd64", valid: true},
		{value: "v1.0.0-alpha-beta", valid: true},
		{value: ""},
		{value: "dev"},
		{value: "ci"},
		{value: "1.2.3"},
		{value: "v1.2"},
		{value: "v01.2.3"},
		{value: "v1.02.3"},
		{value: "v1.2.03"},
		{value: "v1.2.3-01"},
		{value: "v1.2.3-"},
		{value: "v1.2.3+"},
		{value: "v1.2.3-alpha..1"},
		{value: "v1.2.3+linux_amd64"},
		{value: "v1.2.3+build+second"},
		{value: "v1.2.3\nci"},
	}
	for _, test := range tests {
		t.Run(test.value, func(t *testing.T) {
			t.Parallel()
			err := ValidateVersion(test.value)
			if test.valid && err != nil {
				t.Fatalf("ValidateVersion(%q): %v", test.value, err)
			}
			if !test.valid && !errors.Is(err, ErrInvalidReleaseVersion) {
				t.Fatalf("ValidateVersion(%q) error = %v, want ErrInvalidReleaseVersion", test.value, err)
			}
		})
	}
}
