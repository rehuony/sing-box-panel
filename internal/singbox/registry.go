// SPDX-License-Identifier: GPL-3.0-or-later

package singbox

import (
	"fmt"
	"strings"

	"github.com/rehuony/sing-box-panel/internal/configuration"
	"github.com/rehuony/sing-box-panel/internal/subscription"
)

type projector func(string, configuration.ProjectionRequest) (configuration.ProjectionResult, error)

type compiledAdapter struct {
	version   Version
	projector projector
}

func (value compiledAdapter) ID() string {
	return "sing-box/v" + strings.ReplaceAll(value.version.ExactVersion, ".", "_") + "/official-linux-plain"
}

func (value compiledAdapter) Revision() string     { return value.version.AdapterRevision }
func (value compiledAdapter) ExactVersion() string { return value.version.ExactVersion }
func (value compiledAdapter) Provenance() configuration.AdapterProvenance {
	return configuration.AdapterProvenance{
		UpstreamTag:    value.version.Upstream.Tag,
		UpstreamCommit: value.version.Upstream.Commit,
		Source:         "github.com/SagerNet/sing-box/option at the exact upstream tag",
	}
}

func (value compiledAdapter) Supports(profile configuration.CoreProfile) bool {
	expected, ok := value.version.Profiles[profile.Architecture]
	return ok && configuration.MatchesOfficialLinuxPlain(profile, value.version.ExactVersion, expected.Features)
}

func (value compiledAdapter) Project(request configuration.ProjectionRequest) (configuration.ProjectionResult, error) {
	return value.projector(value.version.ExactVersion, request)
}

func NewConfigurationRegistry() *configuration.AdapterRegistry {
	adapters := make([]configuration.Adapter, 0, len(generatedVersions))
	for _, version := range generatedVersions {
		project, ok := familyProjector(version.Family)
		if !ok {
			panic("sing-box support: unknown configuration family " + version.Family)
		}
		adapters = append(adapters, compiledAdapter{version: cloneVersion(version), projector: project})
	}
	return configuration.MustNewAdapterRegistry(adapters...)
}

func NewInboundRegistry() *subscription.InboundRegistry {
	converters := make([]subscription.InboundConverter, 0, len(generatedVersions))
	for _, version := range generatedVersions {
		var options inboundOptions
		switch version.Family {
		case "1.11":
		case "1.12":
			options.anyTLS = true
		case "1.13":
			options.anyTLS = true
			options.naive = true
		default:
			panic("sing-box support: unknown inbound family " + version.Family)
		}
		converters = append(converters, newInboundConverter(version.ExactVersion, options))
	}
	return subscription.MustNewInboundRegistry(converters...)
}

func familyProjector(family string) (projector, bool) {
	switch family {
	case "1.11":
		return projectV111, true
	case "1.12":
		return projectV112, true
	case "1.13":
		return projectV113, true
	default:
		return nil, false
	}
}

func ValidateFamilies() error {
	return validateCompiledFamilies(generatedVersions)
}

func validateCompiledFamilies(versions []Version) error {
	for _, version := range versions {
		if _, ok := familyProjector(version.Family); !ok {
			return fmt.Errorf("version %s references unknown configuration family %s", version.ExactVersion, version.Family)
		}
		switch version.Family {
		case "1.11", "1.12", "1.13":
		default:
			return fmt.Errorf("version %s references unknown inbound family %s", version.ExactVersion, version.Family)
		}
	}
	return nil
}
