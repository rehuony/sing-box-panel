// SPDX-License-Identifier: GPL-3.0-or-later

package release

import (
	"errors"
	"fmt"
	"strings"
)

var ErrInvalidReleaseVersion = errors.New("invalid release version")

// ValidateVersion accepts strict, v-prefixed SemVer release identifiers. dev and ci
// are build-script modes, not release versions.
func ValidateVersion(value string) error {
	if len(value) < 2 || len(value) > 128 || value[0] != 'v' {
		return fmt.Errorf("%w: expected v-prefixed SemVer", ErrInvalidReleaseVersion)
	}
	version := value[1:]
	coreAndPrerelease, build, hasBuild := strings.Cut(version, "+")
	if hasBuild {
		if build == "" || strings.Contains(build, "+") || !validIdentifiers(build, false) {
			return fmt.Errorf("%w: invalid build metadata", ErrInvalidReleaseVersion)
		}
	}
	core, prerelease, hasPrerelease := strings.Cut(coreAndPrerelease, "-")
	if hasPrerelease {
		if prerelease == "" || !validIdentifiers(prerelease, true) {
			return fmt.Errorf("%w: invalid prerelease", ErrInvalidReleaseVersion)
		}
	}
	parts := strings.Split(core, ".")
	if len(parts) != 3 {
		return fmt.Errorf("%w: expected major.minor.patch", ErrInvalidReleaseVersion)
	}
	for _, part := range parts {
		if !validCoreNumber(part) {
			return fmt.Errorf("%w: invalid core version", ErrInvalidReleaseVersion)
		}
	}
	return nil
}

func validCoreNumber(value string) bool {
	if value == "" || len(value) > 1 && value[0] == '0' {
		return false
	}
	for _, character := range value {
		if character < '0' || character > '9' {
			return false
		}
	}
	return true
}

func validIdentifiers(value string, rejectNumericLeadingZero bool) bool {
	for _, identifier := range strings.Split(value, ".") {
		if identifier == "" || rejectNumericLeadingZero && len(identifier) > 1 && identifier[0] == '0' && numeric(identifier) {
			return false
		}
		for _, character := range identifier {
			if (character >= 'A' && character <= 'Z') ||
				(character >= 'a' && character <= 'z') ||
				(character >= '0' && character <= '9') || character == '-' {
				continue
			}
			return false
		}
	}
	return true
}

func numeric(value string) bool {
	for _, character := range value {
		if character < '0' || character > '9' {
			return false
		}
	}
	return value != ""
}
