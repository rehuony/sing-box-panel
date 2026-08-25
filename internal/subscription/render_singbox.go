// SPDX-License-Identifier: GPL-3.0-or-later

package subscription

import (
	"bytes"
	"encoding/json"
)

func renderSingBox(nodes startupNodes, diagnostics []Diagnostic) (Result, []Diagnostic) {
	values := nodes.values
	eligible := make(map[string]bool, len(values))
	byTag := make(map[string]outbound, len(values))
	for _, value := range values {
		eligible[value.tag] = true
		byTag[value.tag] = value
	}
	diagnosticCodes := make(map[string]DiagnosticCode)
	for _, value := range values {
		if !hasRemoteCoordinate(value.value) {
			eligible[value.tag] = false
			diagnosticCodes[value.tag] = DiagnosticInvalidRequiredField
			continue
		}
		if detour, exists := value.value["detour"]; exists {
			tag, ok := detour.(string)
			if !ok || tag == "" {
				eligible[value.tag] = false
				diagnosticCodes[value.tag] = DiagnosticInvalidRequiredField
			} else if _, available := byTag[tag]; !available {
				eligible[value.tag] = false
				diagnosticCodes[value.tag] = DiagnosticUnresolvedDependency
			}
		}
	}
	for changed := true; changed; {
		changed = false
		for _, value := range values {
			if !eligible[value.tag] {
				continue
			}
			detour, exists := value.value["detour"].(string)
			if exists && !eligible[detour] {
				eligible[value.tag] = false
				diagnosticCodes[value.tag] = DiagnosticUnresolvedDependency
				changed = true
			}
		}
	}
	for _, value := range values {
		if eligible[value.tag] {
			continue
		}
		if code := diagnosticCodes[value.tag]; code != "" {
			diagnostics = append(diagnostics, diagnostic(FormatSingBox, value.collection, value.index, code))
		}
	}
	outbounds := make([]map[string]any, 0, len(values))
	endpoints := make([]map[string]any, 0, len(values))
	for _, value := range values {
		if !eligible[value.tag] {
			continue
		}
		if value.collection == CollectionEndpoints {
			endpoints = append(endpoints, value.value)
		} else {
			outbounds = append(outbounds, value.value)
		}
	}
	root := map[string]any{"outbounds": outbounds}
	if nodes.endpointsPresent {
		root["endpoints"] = endpoints
	}
	content, err := json.Marshal(root)
	if err != nil {
		panic(err)
	}
	content = append(bytes.Clone(content), '\n')
	return Result{
		Format:    FormatSingBox,
		MediaType: "application/json",
		Content:   content,
		NodeCount: len(outbounds) + len(endpoints),
	}, diagnostics
}

func hasRemoteCoordinate(value map[string]any) bool {
	if server, ok := value["server"].(string); ok && server != "" {
		return true
	}
	_, realm := value["realm"].(map[string]any)
	if realm {
		return true
	}
	peers, peersOK := value["peers"].([]any)
	return peersOK && len(peers) > 0
}
