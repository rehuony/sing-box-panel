// SPDX-License-Identifier: GPL-3.0-or-later

package render

import "github.com/rehuony/sing-box-panel/internal/subscription/node"

// RenderNodes renders only the explicitly supplied normalized nodes.
func RenderNodes(nodes []node.Node, channel Channel) (Result, error) {
	document, err := node.PublicationDocument(nodes)
	if err != nil {
		return Result{}, invalidStartup("invalid_normalized_nodes")
	}
	return Render(document, channel)
}
