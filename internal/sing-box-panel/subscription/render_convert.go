// SPDX-License-Identifier: GPL-3.0-or-later

package subscription

import (
	"sort"
)

type tlsOptions struct {
	present    bool
	enabled    bool
	serverName string
	insecure   bool
	alpn       []string
}

func commonServer(value outbound, allowed ...string) (string, int64, bool, DiagnosticCode) {
	allowedKeys := make(map[string]struct{}, len(allowed)+4)
	allowedKeys["type"] = struct{}{}
	allowedKeys["tag"] = struct{}{}
	allowedKeys["server"] = struct{}{}
	allowedKeys["server_port"] = struct{}{}
	for _, key := range allowed {
		allowedKeys[key] = struct{}{}
	}
	if code := unsupportedFields(value.value, allowedKeys); code != "" {
		return "", 0, false, code
	}
	server, ok := requiredString(value.value, "server")
	if !ok || len(server) > 2048 {
		return "", 0, false, DiagnosticInvalidRequiredField
	}
	port, ok := integer(value.value["server_port"], 1, 65535)
	if !ok {
		return "", 0, false, DiagnosticInvalidRequiredField
	}
	udp, code := outboundUDP(value.value)
	if code != "" {
		return "", 0, false, code
	}
	return server, port, udp, ""
}

func unsupportedFields(value map[string]any, allowed map[string]struct{}) DiagnosticCode {
	if _, exists := value["detour"]; exists {
		return DiagnosticUnresolvedDependency
	}
	if _, exists := value["transport"]; exists {
		if _, allowedTransport := allowed["transport"]; !allowedTransport {
			return DiagnosticUnsupportedTransport
		}
	}
	if _, exists := value["tls"]; exists {
		if _, allowedTLS := allowed["tls"]; !allowedTLS {
			return DiagnosticUnsupportedTLS
		}
	}
	keys := make([]string, 0, len(value))
	for key := range value {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if _, ok := allowed[key]; !ok {
			return DiagnosticUnsupportedOption
		}
	}
	return ""
}

func requiredString(value map[string]any, key string) (string, bool) {
	text, ok := value[key].(string)
	return text, ok && text != ""
}

func optionalString(value map[string]any, key string) (string, bool) {
	raw, exists := value[key]
	if !exists {
		return "", true
	}
	text, ok := raw.(string)
	return text, ok
}

func optionalBool(value map[string]any, key string) (bool, bool) {
	raw, exists := value[key]
	if !exists {
		return false, true
	}
	result, ok := raw.(bool)
	return result, ok
}

func optionalInteger(value map[string]any, key string, minimum, maximum int64) (int64, bool) {
	raw, exists := value[key]
	if !exists {
		return 0, true
	}
	return integer(raw, minimum, maximum)
}

func integer(value any, minimum, maximum int64) (int64, bool) {
	return DocumentInteger(value, minimum, maximum)
}

func outboundUDP(value map[string]any) (bool, DiagnosticCode) {
	raw, exists := value["network"]
	if !exists {
		return true, ""
	}
	if network, ok := raw.(string); ok {
		switch network {
		case "":
			return true, ""
		case "tcp":
			return false, ""
		default:
			return false, DiagnosticUnsupportedNetwork
		}
	}
	networks, ok := raw.([]any)
	if !ok || len(networks) == 0 || len(networks) > 2 {
		return false, DiagnosticUnsupportedNetwork
	}
	seen := make(map[string]struct{}, len(networks))
	for _, rawNetwork := range networks {
		network, ok := rawNetwork.(string)
		if !ok || (network != "tcp" && network != "udp") {
			return false, DiagnosticUnsupportedNetwork
		}
		if _, duplicate := seen[network]; duplicate {
			return false, DiagnosticUnsupportedNetwork
		}
		seen[network] = struct{}{}
	}
	if _, tcp := seen["tcp"]; !tcp {
		return false, DiagnosticUnsupportedNetwork
	}
	_, udp := seen["udp"]
	return udp, ""
}

func parseTLS(value map[string]any, required bool) (tlsOptions, DiagnosticCode) {
	options, issue := parseTLSIssue(value, required)
	return options, issue.code
}

// Both conversion and preview use this validation, so explanations cannot drift
// from the checks that actually omitted the node.
func parseTLSIssue(value map[string]any, required bool) (tlsOptions, conversionIssue) {
	raw, exists := value["tls"]
	if !exists {
		if required {
			return tlsOptions{}, conversionIssue{DiagnosticInvalidRequiredField, "tls", "tls_required"}
		}
		return tlsOptions{}, conversionIssue{}
	}
	tlsValue, ok := raw.(map[string]any)
	if !ok {
		return tlsOptions{}, conversionIssue{DiagnosticUnsupportedTLS, "tls", "invalid_field"}
	}
	allowed := map[string]struct{}{
		"alpn": {}, "enabled": {}, "insecure": {}, "server_name": {},
	}
	if code := unsupportedFields(tlsValue, allowed); code != "" {
		// Only known field names may appear in a diagnostic; arbitrary extension
		// keys, like values, could themselves contain credentials.
		field := "tls"
		for _, key := range []string{"certificate", "certificate_path", "cipher_suites", "disable_sni", "ech", "fragment", "fragment_fallback_delay", "kernel_tx", "kernel_rx", "max_version", "min_version", "record_fragment", "reality", "utls"} {
			if _, exists := tlsValue[key]; exists {
				field += "." + key
				break
			}
		}
		return tlsOptions{}, conversionIssue{DiagnosticUnsupportedTLS, field, "unsupported_tls_option"}
	}
	enabled, ok := optionalBool(tlsValue, "enabled")
	if !ok {
		return tlsOptions{}, conversionIssue{DiagnosticUnsupportedTLS, "tls.enabled", "invalid_field"}
	}
	if required && !enabled {
		return tlsOptions{}, conversionIssue{DiagnosticUnsupportedTLS, "tls.enabled", "tls_required"}
	}
	serverName, ok := optionalString(tlsValue, "server_name")
	if !ok || len(serverName) > 2048 {
		return tlsOptions{}, conversionIssue{DiagnosticUnsupportedTLS, "tls.server_name", "invalid_field"}
	}
	insecure, ok := optionalBool(tlsValue, "insecure")
	if !ok {
		return tlsOptions{}, conversionIssue{DiagnosticUnsupportedTLS, "tls.insecure", "invalid_field"}
	}
	alpn, ok := stringList(tlsValue, "alpn", 16)
	if !ok {
		return tlsOptions{}, conversionIssue{DiagnosticUnsupportedTLS, "tls.alpn", "invalid_field"}
	}
	return tlsOptions{
		present: true, enabled: enabled, serverName: serverName, insecure: insecure, alpn: alpn,
	}, conversionIssue{}
}

func stringList(value map[string]any, key string, maximum int) ([]string, bool) {
	raw, exists := value[key]
	if !exists {
		return nil, true
	}
	values, ok := raw.([]any)
	// sing-box Listable fields accept either a scalar or a list. Normalize at
	// the consumer boundary too: imported and older saved nodes may use either.
	if scalar, scalarOK := raw.(string); scalarOK {
		values, ok = []any{scalar}, true
	}
	if !ok || len(values) > maximum {
		return nil, false
	}
	result := make([]string, 0, len(values))
	for _, rawValue := range values {
		text, ok := rawValue.(string)
		if !ok || text == "" || len(text) > 256 {
			return nil, false
		}
		result = append(result, text)
	}
	return result, true
}
