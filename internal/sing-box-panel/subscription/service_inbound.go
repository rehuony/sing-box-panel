// SPDX-License-Identifier: GPL-3.0-or-later

package subscription

func (application *Service) convertInboundNodes(
	exactVersion string,
	finalStartupJSON []byte,
	publicHost string,
) (InboundResult, error) {
	return convertInbounds(exactVersion, InboundRequest{
		FinalStartupJSON: finalStartupJSON,
		PublicHost:       publicHost,
	})
}
