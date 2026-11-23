// SPDX-License-Identifier: GPL-3.0-or-later

package subscription

// Render extracts publishable outbounds from finalStartupJSON and emits one
// deterministic client format. It never reads canonical state, mutable
// runtime state, the network, or a database.
func Render(finalStartupJSON []byte, channel RenderChannel) (RenderResult, error) {
	if err := validateChannel(channel); err != nil {
		return RenderResult{}, err
	}
	nodes, diagnostics, err := parseStartup(finalStartupJSON, channel.Format)
	if err != nil {
		return RenderResult{}, err
	}
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

// Carries reports whether the rendered channel delivers the pool node id.
func (result RenderResult) Carries(id string) bool {
	return result.carried[id]
}
