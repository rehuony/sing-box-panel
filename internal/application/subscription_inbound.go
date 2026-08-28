// SPDX-License-Identifier: GPL-3.0-or-later

package application

import (
	"github.com/rehuony/sing-box-panel/internal/subscription/inbound"
	singbox11115 "github.com/rehuony/sing-box-panel/internal/subscription/inbound/singbox/v1_11_15"
	singbox11225 "github.com/rehuony/sing-box-panel/internal/subscription/inbound/singbox/v1_12_25"
	singbox11319 "github.com/rehuony/sing-box-panel/internal/subscription/inbound/singbox/v1_13_19"
)

var compiledInboundRegistry = inbound.MustNewRegistry(
	singbox11115.New(),
	singbox11225.New(),
	singbox11319.New(),
)

func (application *Application) convertInboundNodes(
	exactVersion string,
	finalStartupJSON []byte,
	publicHost string,
) (inbound.Result, error) {
	return compiledInboundRegistry.Convert(exactVersion, inbound.Request{
		FinalStartupJSON: finalStartupJSON,
		PublicHost:       publicHost,
	})
}
