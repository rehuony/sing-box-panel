// SPDX-License-Identifier: GPL-3.0-or-later

package singbox

import (
	"fmt"

	"github.com/rehuony/sing-box-panel/internal/subscription"
)

func NewInboundRegistry() *subscription.InboundRegistry {
	converters := make([]subscription.InboundConverter, 0, len(generatedVersions))
	for _, version := range generatedVersions {
		var options inboundOptions
		switch version.InboundFamily {
		case "":
			continue
		case "1.14":
			options.snell = true
		case "1.13":
		default:
			panic("sing-box support: unknown inbound family " + version.InboundFamily)
		}
		converters = append(converters, newInboundConverter(version.ExactVersion, options))
	}
	return subscription.MustNewInboundRegistry(converters...)
}

func ValidateFamilies() error {
	return validateCompiledFamilies(generatedVersions)
}

func validateCompiledFamilies(versions []Version) error {
	for _, version := range versions {
		switch version.InboundFamily {
		case "", "1.13", "1.14":
		default:
			return fmt.Errorf("version %s references unknown inbound family %s", version.ExactVersion, version.InboundFamily)
		}
	}
	return nil
}
