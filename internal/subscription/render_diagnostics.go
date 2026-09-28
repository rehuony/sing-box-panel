// SPDX-License-Identifier: GPL-3.0-or-later

package subscription

import "fmt"

type conversionIssue struct {
	code   DiagnosticCode
	field  string
	reason string
}

func previewDiagnostics(diagnostics []RenderDiagnostic, values []outbound) []PreviewDiagnostic {
	type position struct {
		collection Collection
		index      int
	}
	nodes := make(map[position]outbound, len(values))
	for _, value := range values {
		nodes[position{value.collection, value.index}] = value
	}
	result := make([]PreviewDiagnostic, 0, len(diagnostics))
	for _, diagnostic := range diagnostics {
		item := PreviewDiagnostic{RenderDiagnostic: diagnostic}
		field := ""
		switch diagnostic.Code {
		case DiagnosticUnsupportedType:
			field = "type"
		case DiagnosticUnsupportedTransport:
			field = "transport"
		case DiagnosticUnsupportedTLS:
			field = "tls"
		case DiagnosticUnsupportedNetwork:
			field = "network"
		case DiagnosticUnresolvedDependency:
			field = "detour"
		case DiagnosticDuplicateTag:
			field = "tag"
		}
		if value, exists := nodes[position{diagnostic.Collection, diagnostic.ItemIndex}]; exists {
			item.NodeID, item.NodeName, item.NodeType = value.nodeID, value.tag, value.typeID
			if diagnostic.Code == DiagnosticUnsupportedTLS {
				issue := describeTLSIssue(value, diagnostic.Format)
				if issue.code == diagnostic.Code {
					field, item.Reason = issue.field, issue.reason
				}
			}
		}
		item.FieldPath = fmt.Sprintf("%s[%d]", diagnostic.Collection, diagnostic.ItemIndex)
		if field != "" {
			item.FieldPath += "." + field
		}
		result = append(result, item)
	}
	return result
}

func describeTLSIssue(value outbound, format RenderFormat) conversionIssue {
	if format == RenderFormatMihomo {
		prepared, _, code := mihomoMappedOptions(value)
		if code != "" {
			// Mapping can fail before the common TLS checks; don't misidentify a
			// supported uTLS/Reality option as an unsupported common TLS field.
			return conversionIssue{}
		}
		value = prepared
	}
	required := oneOf(value.typeID, "trojan", "hysteria2", "tuic", "anytls")
	tls, issue := parseTLSIssue(value.value, required)
	if issue.code != "" {
		return issue
	}
	if format == RenderFormatLoon && tls.enabled {
		_, issue = loonALPN(tls.alpn)
	}
	return issue
}
