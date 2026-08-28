// SPDX-License-Identifier: GPL-3.0-or-later

package inbound

import (
	"fmt"
	"sort"

	"github.com/rehuony/sing-box-panel/internal/coreartifact"
)

type Registry struct {
	converters map[string]Converter
}

func (registry *Registry) Versions() []string {
	if registry == nil {
		return []string{}
	}
	versions := make([]string, 0, len(registry.converters))
	for version := range registry.converters {
		versions = append(versions, version)
	}
	sort.Slice(versions, func(left, right int) bool {
		leftVersion, _ := coreartifact.ParseExactVersion(versions[left])
		rightVersion, _ := coreartifact.ParseExactVersion(versions[right])
		return leftVersion.Compare(rightVersion) < 0
	})
	return versions
}

// MustNewRegistry builds the immutable compiled-in converter registry. Invalid
// or duplicate registrations are programmer errors, not runtime input errors.
func MustNewRegistry(converters ...Converter) *Registry {
	values := make(map[string]Converter, len(converters))
	for _, converter := range converters {
		if converter == nil {
			panic("subscription inbound registry: nil converter")
		}
		version, err := coreartifact.ParseExactVersion(converter.ExactVersion())
		if err != nil || version.IsZero() || version.String() != converter.ExactVersion() {
			panic(fmt.Sprintf("subscription inbound registry: invalid exact version %q", converter.ExactVersion()))
		}
		if _, duplicate := values[version.String()]; duplicate {
			panic(fmt.Sprintf("subscription inbound registry: duplicate exact version %q", version.String()))
		}
		values[version.String()] = converter
	}
	return &Registry{converters: values}
}

func (registry *Registry) Convert(exactVersion string, request Request) (Result, error) {
	version, err := coreartifact.ParseExactVersion(exactVersion)
	if err != nil || version.IsZero() || version.String() != exactVersion {
		return Result{}, fmt.Errorf("%w: %s", ErrUnsupportedCoreVersion, exactVersion)
	}
	if registry == nil {
		return Result{}, fmt.Errorf("%w: %s", ErrUnsupportedCoreVersion, exactVersion)
	}
	converter, ok := registry.converters[exactVersion]
	if !ok {
		return Result{}, fmt.Errorf("%w: %s", ErrUnsupportedCoreVersion, exactVersion)
	}
	return converter.Convert(request)
}
