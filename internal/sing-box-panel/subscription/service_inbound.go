// SPDX-License-Identifier: GPL-3.0-or-later

package subscription

import ()

var compiledInboundRegistry = NewInboundRegistry()

func (application *Service) convertInboundNodes(
	exactVersion string,
	finalStartupJSON []byte,
	publicHost string,
) (InboundResult, error) {
	return compiledInboundRegistry.Convert(exactVersion, InboundRequest{
		FinalStartupJSON: finalStartupJSON,
		PublicHost:       publicHost,
	})
}
