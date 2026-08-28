// SPDX-License-Identifier: GPL-3.0-or-later

package render

// Render extracts publishable outbounds from finalStartupJSON, applies exact
// channel exclusions, and emits one deterministic client format. It never
// reads canonical state, mutable runtime state, the network, or a database.
func Render(finalStartupJSON []byte, channel Channel) (Result, error) {
	exclusions, err := validateChannel(channel)
	if err != nil {
		return Result{}, err
	}
	nodes, diagnostics, err := parseStartup(finalStartupJSON, channel.Format)
	if err != nil {
		return Result{}, err
	}
	nodes.values = applyFilter(nodes.values, exclusions)

	var result Result
	switch channel.Format {
	case FormatSingBox:
		result, diagnostics = renderSingBox(nodes, diagnostics)
	case FormatMihomo:
		result, diagnostics = renderMihomo(nodes.values, diagnostics)
	case FormatLoon:
		result, diagnostics = renderLoon(nodes.values, diagnostics)
	}
	sortDiagnostics(diagnostics)
	result.Diagnostics = diagnostics
	return result, nil
}
