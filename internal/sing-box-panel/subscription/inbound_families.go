// SPDX-License-Identifier: GPL-3.0-or-later

package subscription

import (
	"fmt"

	"github.com/rehuony/sing-box-panel/internal/singbox"
)

// inboundConverters holds the converter of each inbound family.
var inboundConverters = map[string]converter{
	"1.13": newInboundConverter(inboundOptions{}),
	"1.14": newInboundConverter(inboundOptions{snell: true}),
}

// convertInbounds converts a node's inbounds with the converter of the
// inbound family of its core's release line.
func convertInbounds(exactVersion string, request InboundRequest) (InboundResult, error) {
	line, found := singbox.LineOf(exactVersion)
	if !found {
		return InboundResult{}, fmt.Errorf("%w: %s", ErrUnsupportedCoreVersion, exactVersion)
	}
	converter, found := inboundConverters[line.InboundFamily]
	if !found {
		return InboundResult{}, fmt.Errorf("%w: %s", ErrUnsupportedCoreVersion, exactVersion)
	}
	return converter.Convert(request)
}
