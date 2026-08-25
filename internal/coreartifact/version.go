// SPDX-License-Identifier: GPL-3.0-or-later

package coreartifact

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

var ErrInvalidExactVersion = errors.New("invalid exact semantic version")

// ExactVersion is a stable semantic version without a leading v, prerelease,
// or build metadata. Its fields are intentionally private so a parsed value
// cannot subsequently be made invalid.
type ExactVersion struct {
	major uint64
	minor uint64
	patch uint64
}

func NewExactVersion(major, minor, patch uint64) ExactVersion {
	return ExactVersion{major: major, minor: minor, patch: patch}
}

func ParseExactVersion(value string) (ExactVersion, error) {
	parts := strings.Split(value, ".")
	if len(parts) != 3 {
		return ExactVersion{}, fmt.Errorf("%w: %q must contain major.minor.patch", ErrInvalidExactVersion, value)
	}

	numbers := make([]uint64, 3)
	for index, part := range parts {
		if part == "" || (len(part) > 1 && part[0] == '0') {
			return ExactVersion{}, fmt.Errorf("%w: %q has an empty or zero-padded component", ErrInvalidExactVersion, value)
		}
		for _, character := range part {
			if character < '0' || character > '9' {
				return ExactVersion{}, fmt.Errorf("%w: %q contains a non-numeric component", ErrInvalidExactVersion, value)
			}
		}
		number, err := strconv.ParseUint(part, 10, 64)
		if err != nil {
			return ExactVersion{}, fmt.Errorf("%w: %q: %v", ErrInvalidExactVersion, value, err)
		}
		numbers[index] = number
	}

	return NewExactVersion(numbers[0], numbers[1], numbers[2]), nil
}

func (version ExactVersion) Major() uint64 { return version.major }

func (version ExactVersion) Minor() uint64 { return version.minor }

func (version ExactVersion) Patch() uint64 { return version.patch }

func (version ExactVersion) IsZero() bool {
	return version.major == 0 && version.minor == 0 && version.patch == 0
}

func (version ExactVersion) String() string {
	return strconv.FormatUint(version.major, 10) + "." +
		strconv.FormatUint(version.minor, 10) + "." +
		strconv.FormatUint(version.patch, 10)
}

func (version ExactVersion) Compare(other ExactVersion) int {
	left := [...]uint64{version.major, version.minor, version.patch}
	right := [...]uint64{other.major, other.minor, other.patch}
	for index := range left {
		if left[index] < right[index] {
			return -1
		}
		if left[index] > right[index] {
			return 1
		}
	}
	return 0
}

func (version ExactVersion) MarshalText() ([]byte, error) {
	return []byte(version.String()), nil
}

func (version *ExactVersion) UnmarshalText(text []byte) error {
	parsed, err := ParseExactVersion(string(text))
	if err != nil {
		return err
	}
	*version = parsed
	return nil
}

func (version ExactVersion) MarshalJSON() ([]byte, error) {
	return json.Marshal(version.String())
}

func (version *ExactVersion) UnmarshalJSON(data []byte) error {
	var value string
	if err := json.Unmarshal(data, &value); err != nil {
		return fmt.Errorf("%w: expected a string: %v", ErrInvalidExactVersion, err)
	}
	return version.UnmarshalText([]byte(value))
}
