// SPDX-License-Identifier: GPL-3.0-or-later

package subscription

import (
	"fmt"
	"sort"

	"github.com/rehuony/sing-box-panel/internal/coreartifact"
)

type InboundRegistry struct {
	converters map[string]InboundConverter
}

func (registry *InboundRegistry) Versions() []string {
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

// MustNewInboundRegistry builds the immutable compiled-in converter registry. Invalid
// or duplicate registrations are programmer errors, not runtime input errors.
func MustNewInboundRegistry(converters ...InboundConverter) *InboundRegistry {
	values := make(map[string]InboundConverter, len(converters))
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
	return &InboundRegistry{converters: values}
}

func (registry *InboundRegistry) Convert(exactVersion string, request InboundRequest) (InboundResult, error) {
	version, err := coreartifact.ParseExactVersion(exactVersion)
	if err != nil || version.IsZero() || version.String() != exactVersion {
		return InboundResult{}, fmt.Errorf("%w: %s", ErrUnsupportedCoreVersion, exactVersion)
	}
	if registry == nil {
		return InboundResult{}, fmt.Errorf("%w: %s", ErrUnsupportedCoreVersion, exactVersion)
	}
	converter, ok := registry.converters[exactVersion]
	if !ok {
		return InboundResult{}, fmt.Errorf("%w: %s", ErrUnsupportedCoreVersion, exactVersion)
	}
	return converter.Convert(request)
}
