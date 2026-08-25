// SPDX-License-Identifier: GPL-3.0-or-later

package subscription

import (
	"encoding/json"
	"sort"
	"strconv"
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
	number, ok := value.(json.Number)
	if !ok {
		return 0, false
	}
	parsed, err := strconv.ParseInt(number.String(), 10, 64)
	return parsed, err == nil && parsed >= minimum && parsed <= maximum
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
	raw, exists := value["tls"]
	if !exists {
		if required {
			return tlsOptions{}, DiagnosticInvalidRequiredField
		}
		return tlsOptions{}, ""
	}
	tlsValue, ok := raw.(map[string]any)
	if !ok {
		return tlsOptions{}, DiagnosticUnsupportedTLS
	}
	allowed := map[string]struct{}{
		"alpn":        {},
		"enabled":     {},
		"insecure":    {},
		"server_name": {},
	}
	if code := unsupportedFields(tlsValue, allowed); code != "" {
		return tlsOptions{}, DiagnosticUnsupportedTLS
	}
	enabled, ok := optionalBool(tlsValue, "enabled")
	if !ok || (required && !enabled) {
		return tlsOptions{}, DiagnosticUnsupportedTLS
	}
	serverName, ok := optionalString(tlsValue, "server_name")
	if !ok || len(serverName) > 2048 {
		return tlsOptions{}, DiagnosticUnsupportedTLS
	}
	insecure, ok := optionalBool(tlsValue, "insecure")
	if !ok {
		return tlsOptions{}, DiagnosticUnsupportedTLS
	}
	alpn, ok := stringList(tlsValue, "alpn", 16)
	if !ok {
		return tlsOptions{}, DiagnosticUnsupportedTLS
	}
	return tlsOptions{
		present:    true,
		enabled:    enabled,
		serverName: serverName,
		insecure:   insecure,
		alpn:       alpn,
	}, ""
}

func stringList(value map[string]any, key string, maximum int) ([]string, bool) {
	raw, exists := value[key]
	if !exists {
		return nil, true
	}
	values, ok := raw.([]any)
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
