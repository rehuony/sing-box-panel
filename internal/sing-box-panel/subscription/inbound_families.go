// SPDX-License-Identifier: GPL-3.0-or-later

package subscription

import (
	"fmt"

	"github.com/rehuony/sing-box-panel/internal/singbox"
)

func NewInboundRegistry() *InboundRegistry {
	converters := make([]InboundConverter, 0, len(singbox.Versions()))
	for _, version := range singbox.Versions() {
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
	return MustNewInboundRegistry(converters...)
}

func ValidateFamilies() error {
	return validateCompiledFamilies(singbox.Versions())
}

func validateCompiledFamilies(versions []singbox.Version) error {
	for _, version := range versions {
		switch version.InboundFamily {
		case "", "1.13", "1.14":
		default:
			return fmt.Errorf("version %s references unknown inbound family %s", version.ExactVersion, version.InboundFamily)
		}
	}
	return nil
}
