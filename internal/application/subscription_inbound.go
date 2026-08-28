// SPDX-License-Identifier: GPL-3.0-or-later

package application

import (
	"github.com/rehuony/sing-box-panel/internal/singbox"
	"github.com/rehuony/sing-box-panel/internal/subscription"
)

var compiledInboundRegistry = singbox.NewInboundRegistry()

func (application *Application) convertInboundNodes(
	exactVersion string,
	finalStartupJSON []byte,
	publicHost string,
) (subscription.InboundResult, error) {
	return compiledInboundRegistry.Convert(exactVersion, subscription.InboundRequest{
		FinalStartupJSON: finalStartupJSON,
		PublicHost:       publicHost,
	})
}
