// SPDX-License-Identifier: GPL-3.0-or-later

package subscription

// Render extracts publishable outbounds from finalStartupJSON, applies exact
// channel exclusions, and emits one deterministic client format. It never
// reads canonical state, mutable runtime state, the network, or a database.
func Render(finalStartupJSON []byte, channel RenderChannel) (RenderResult, error) {
	return render(finalStartupJSON, channel, nil)
}

func render(finalStartupJSON []byte, channel RenderChannel, publicationNodes []Node) (RenderResult, error) {
	exclusions, err := validateChannel(channel)
	if err != nil {
		return RenderResult{}, err
	}
	nodes, diagnostics, err := parseStartup(finalStartupJSON, channel.Format)
	if err != nil {
		return RenderResult{}, err
	}
	nodes.values = applyFilter(nodes.values, exclusions)
	for i := range nodes.values {
		value := &nodes.values[i]
		if value.collection == CollectionOutbounds && value.index < len(publicationNodes) {
			value.nodeID = PublicationID(publicationNodes[value.index])
		}
	}
	orderChannelOutbounds(nodes.values, channel.NodeOrder)

	var result RenderResult
	switch channel.Format {
	case RenderFormatSingBox:
		result, diagnostics = renderSingBox(nodes, diagnostics)
	case RenderFormatMihomo:
		result, diagnostics = renderMihomo(nodes.values, diagnostics)
	case RenderFormatLoon:
		result, diagnostics = renderLoon(nodes.values, diagnostics)
	}
	sortDiagnostics(diagnostics)
	result.Diagnostics = diagnostics
	result.PreviewDiagnostics = previewDiagnostics(diagnostics, nodes.values)
	return result, nil
}

// RenderNodes renders only the explicitly supplied normalized nodes.
func RenderNodes(nodes []Node, channel RenderChannel) (RenderResult, error) {
	document, err := PublicationDocument(nodes)
	if err != nil {
		return RenderResult{}, invalidStartup("invalid_normalized_nodes")
	}
	ordered := orderedPublicationNodes(nodes)
	result, err := render(document, channel, ordered)
	if err != nil {
		return RenderResult{}, err
	}
	// PublicationDocument defines these positions before filtering and tag sorting.
	for index := range result.PreviewDiagnostics {
		issue := &result.PreviewDiagnostics[index]
		if issue.Collection == CollectionOutbounds && issue.ItemIndex < len(ordered) {
			node := ordered[issue.ItemIndex]
			issue.NodeID, issue.NodeName, issue.NodeType = PublicationID(node), node.Tag, node.Type
		}
	}
	return result, nil
}
