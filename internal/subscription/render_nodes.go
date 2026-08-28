// SPDX-License-Identifier: GPL-3.0-or-later

package subscription

// RenderNodes renders only the explicitly supplied normalized nodes.
func RenderNodes(nodes []Node, channel RenderChannel) (RenderResult, error) {
	document, err := PublicationDocument(nodes)
	if err != nil {
		return RenderResult{}, invalidStartup("invalid_normalized_nodes")
	}
	return Render(document, channel)
}
