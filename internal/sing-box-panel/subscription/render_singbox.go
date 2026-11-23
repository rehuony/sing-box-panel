// SPDX-License-Identifier: GPL-3.0-or-later

package subscription

import (
	"bytes"
	"encoding/json"
)

func renderSingBox(nodes startupNodes, diagnostics []RenderDiagnostic) (RenderResult, []RenderDiagnostic) {
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
	// Resolve each dependency chain once; cycles and chains reaching an invalid
	// node are ineligible. Iteration avoids recursion on untrusted long chains.
	resolved := make(map[string]bool, len(values))
	for _, value := range values {
		if resolved[value.tag] {
			continue
		}
		path := []string{}
		visiting := map[string]bool{}
		tag, valid := value.tag, true
		for {
			if resolved[tag] || !eligible[tag] {
				valid = eligible[tag]
				break
			}
			if visiting[tag] {
				valid = false
				break
			}
			visiting[tag] = true
			path = append(path, tag)
			detour, exists := byTag[tag].value["detour"].(string)
			if !exists {
				break
			}
			tag = detour
		}
		for _, tag := range path {
			resolved[tag] = true
			eligible[tag] = valid
			if !valid {
				diagnosticCodes[tag] = DiagnosticUnresolvedDependency
			}
		}
	}
	for _, value := range values {
		if eligible[value.tag] {
			continue
		}
		if code := diagnosticCodes[value.tag]; code != "" {
			diagnostics = append(diagnostics, diagnostic(RenderFormatSingBox, value.collection, value.index, code))
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
	return RenderResult{
		Format:    RenderFormatSingBox,
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
