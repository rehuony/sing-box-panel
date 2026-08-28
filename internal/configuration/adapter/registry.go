// SPDX-License-Identifier: GPL-3.0-or-later

package adapter

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/rehuony/sing-box-panel/internal/coreartifact"
)

var adapterIdentifierPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._/-]*$`)

type Registry struct {
	byVersion map[string]Adapter
}

// MustNewRegistry constructs an immutable compile-time registry. Invalid or
// duplicate registrations are programmer errors and fail during composition.
func MustNewRegistry(adapters ...Adapter) *Registry {
	byVersion := make(map[string]Adapter, len(adapters))
	for _, current := range adapters {
		if current == nil {
			panic("configuration adapter registry: nil adapter")
		}
		version, err := coreartifact.ParseExactVersion(current.ExactVersion())
		if err != nil || version.IsZero() || version.String() != current.ExactVersion() {
			panic(fmt.Sprintf("configuration adapter registry: invalid exact version %q", current.ExactVersion()))
		}
		if !adapterIdentifierPattern.MatchString(current.ID()) || strings.TrimSpace(current.Revision()) == "" {
			panic(fmt.Sprintf("configuration adapter registry: invalid identity %q@%q", current.ID(), current.Revision()))
		}
		provenance := current.Provenance()
		if provenance.UpstreamTag != "v"+current.ExactVersion() || provenance.UpstreamCommit == "" || provenance.Source == "" {
			panic(fmt.Sprintf("configuration adapter registry: invalid provenance for %q", current.ExactVersion()))
		}
		if _, duplicate := byVersion[current.ExactVersion()]; duplicate {
			panic(fmt.Sprintf("configuration adapter registry: duplicate exact version %q", current.ExactVersion()))
		}
		byVersion[current.ExactVersion()] = current
	}
	return &Registry{byVersion: byVersion}
}

func (registry *Registry) Resolve(profile Profile) (Adapter, error) {
	normalized, err := ValidateProfile(profile)
	if err != nil {
		return nil, err
	}
	if registry == nil {
		return nil, fmt.Errorf("%w: %s", ErrUnsupportedCoreProfile, normalized.ExactVersion)
	}
	resolved, found := registry.byVersion[normalized.ExactVersion]
	if !found || !resolved.Supports(normalized) {
		return nil, fmt.Errorf("%w: %s %s/%s %s", ErrUnsupportedCoreProfile,
			normalized.ExactVersion, normalized.OperatingSystem, normalized.Architecture, normalized.Variant)
	}
	return resolved, nil
}

func (registry *Registry) Project(profile Profile, request Request) (Result, error) {
	resolved, err := registry.Resolve(profile)
	if err != nil {
		return Result{}, err
	}
	return resolved.Project(request)
}

func (registry *Registry) Versions() []string {
	if registry == nil {
		return []string{}
	}
	versions := make([]string, 0, len(registry.byVersion))
	for version := range registry.byVersion {
		versions = append(versions, version)
	}
	sort.Slice(versions, func(left, right int) bool {
		leftVersion, _ := coreartifact.ParseExactVersion(versions[left])
		rightVersion, _ := coreartifact.ParseExactVersion(versions[right])
		return leftVersion.Compare(rightVersion) < 0
	})
	return versions
}
