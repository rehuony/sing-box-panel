// SPDX-License-Identifier: GPL-3.0-or-later

package subscription

import (
	"encoding/base64"
	"encoding/hex"
	"maps"
)

// The options below have native Mihomo equivalents. Unknown fields still fail
// closed; converting one supported option must not silently discard another.
func mihomoMappedOptions(value outbound) (outbound, []yamlField, DiagnosticCode) {
	value.value = maps.Clone(value.value)
	extra := []yamlField{}
	if raw, exists := value.value["transport"]; exists {
		if !oneOf(value.typeID, "vless", "vmess", "trojan") {
			return value, nil, DiagnosticUnsupportedTransport
		}
		transport, ok := raw.(map[string]any)
		if !ok {
			return value, nil, DiagnosticUnsupportedTransport
		}
		fields, code := mihomoTransport(transport)
		if code != "" {
			return value, nil, code
		}
		extra = append(extra, fields...)
		delete(value.value, "transport")
	}
	if raw, exists := value.value["multiplex"]; exists {
		if !oneOf(value.typeID, "shadowsocks", "vless", "vmess", "trojan") {
			return value, nil, DiagnosticUnsupportedOption
		}
		mux, ok := raw.(map[string]any)
		if !ok || !validMuxOptions(mux) {
			return value, nil, DiagnosticUnsupportedOption
		}
		mapped := map[string]any{}
		keys := map[string]string{"enabled": "enabled", "protocol": "protocol", "max_connections": "max-connections", "min_streams": "min-streams", "max_streams": "max-streams", "padding": "padding", "brutal": "brutal-opts"}
		for key, value := range mux {
			name, exists := keys[key]
			if !exists {
				return outbound{}, nil, DiagnosticUnsupportedOption
			}
			if key == "brutal" {
				brutal, ok := value.(map[string]any)
				if !ok {
					return outbound{}, nil, DiagnosticUnsupportedOption
				}
				converted := map[string]any{}
				for k, v := range brutal {
					name := map[string]string{"enabled": "enabled", "up_mbps": "up", "down_mbps": "down"}[k]
					if name == "" {
						return outbound{}, nil, DiagnosticUnsupportedOption
					}
					converted[name] = v
				}
				value = converted
			}
			mapped[name] = value
		}
		extra = append(extra, yamlField{"smux", mapped})
		delete(value.value, "multiplex")
	}
	if tls, ok := value.value["tls"].(map[string]any); ok {
		tls = maps.Clone(tls)
		value.value["tls"] = tls
		if raw, exists := tls["utls"]; exists {
			if !oneOf(value.typeID, "http", "vless", "vmess", "trojan", "anytls") {
				return value, nil, DiagnosticUnsupportedTLS
			}
			utls, ok := raw.(map[string]any)
			if !ok || unsupportedFields(utls, map[string]struct{}{"enabled": {}, "fingerprint": {}}) != "" {
				return value, nil, DiagnosticUnsupportedTLS
			}
			enabled, valid := optionalBool(utls, "enabled")
			if !valid {
				return value, nil, DiagnosticUnsupportedTLS
			}
			if enabled {
				fingerprint, valid := optionalString(utls, "fingerprint")
				if !valid || tls["enabled"] != true {
					return value, nil, DiagnosticUnsupportedTLS
				}
				if fingerprint == "" {
					fingerprint = "chrome"
				}
				extra = append(extra, yamlField{"client-fingerprint", fingerprint})
			}
			delete(tls, "utls")
		}
		if raw, exists := tls["reality"]; exists {
			if value.typeID != "vless" {
				return value, nil, DiagnosticUnsupportedTLS
			}
			reality, ok := raw.(map[string]any)
			if !ok || unsupportedFields(reality, map[string]struct{}{"enabled": {}, "public_key": {}, "short_id": {}}) != "" {
				return value, nil, DiagnosticUnsupportedTLS
			}
			enabled, valid := optionalBool(reality, "enabled")
			if !valid {
				return value, nil, DiagnosticUnsupportedTLS
			}
			if enabled {
				key, valid := requiredString(reality, "public_key")
				short, shortValid := optionalString(reality, "short_id")
				if !valid || !shortValid || tls["enabled"] != true {
					return value, nil, DiagnosticUnsupportedTLS
				}
				keyBytes, keyErr := base64.RawURLEncoding.DecodeString(key)
				_, shortErr := hex.DecodeString(short)
				if keyErr != nil || len(keyBytes) != 32 || shortErr != nil || len(short) > 16 {
					return value, nil, DiagnosticUnsupportedTLS
				}
				extra = append(extra, yamlField{"reality-opts", map[string]any{"public-key": key, "short-id": short}})
			}
			delete(tls, "reality")
		}
	}
	return value, extra, ""
}

func mihomoTransport(value map[string]any) ([]yamlField, DiagnosticCode) {
	typeID, _ := value["type"].(string)
	options := map[string]any{}
	var output string
	switch typeID {
	case "ws":
		output = "ws-opts"
		keys := map[string]string{"path": "path", "headers": "headers", "max_early_data": "max-early-data", "early_data_header_name": "early-data-header-name"}
		for key, value := range value {
			if key == "type" {
				continue
			}
			name := keys[key]
			if name == "" {
				return nil, DiagnosticUnsupportedTransport
			}
			if key == "headers" {
				headers, ok := value.(map[string]any)
				if !ok {
					return nil, DiagnosticUnsupportedTransport
				}
				for _, header := range headers {
					if _, ok := header.(string); !ok {
						return nil, DiagnosticUnsupportedTransport
					}
				}
			} else if key == "max_early_data" {
				if _, ok := integer(value, 0, 2147483647); !ok {
					return nil, DiagnosticUnsupportedTransport
				}
			} else if _, ok := value.(string); !ok {
				return nil, DiagnosticUnsupportedTransport
			}
			options[name] = value
		}
	case "grpc":
		output = "grpc-opts"
		for key, value := range value {
			if key == "type" {
				continue
			}
			if key != "service_name" {
				return nil, DiagnosticUnsupportedTransport
			}
			if _, ok := value.(string); !ok {
				return nil, DiagnosticUnsupportedTransport
			}
			options["grpc-service-name"] = value
		}
	default:
		return nil, DiagnosticUnsupportedTransport
	}
	return []yamlField{{"network", typeID}, {output, options}}, ""
}

func validMuxOptions(value map[string]any) bool {
	for _, key := range []string{"enabled", "padding"} {
		if _, ok := optionalBool(value, key); !ok {
			return false
		}
	}
	protocol, ok := optionalString(value, "protocol")
	if !ok || !oneOf(protocol, "", "smux", "yamux", "h2mux") {
		return false
	}
	maxConnections, a := optionalInteger(value, "max_connections", 0, 2147483647)
	minStreams, b := optionalInteger(value, "min_streams", 0, 2147483647)
	maxStreams, c := optionalInteger(value, "max_streams", 0, 2147483647)
	if !a || !b || !c || (maxStreams > 0 && (maxConnections > 0 || minStreams > 0)) {
		return false
	}
	if raw, exists := value["brutal"]; exists {
		brutal, ok := raw.(map[string]any)
		if !ok {
			return false
		}
		if _, ok := optionalBool(brutal, "enabled"); !ok {
			return false
		}
		for _, key := range []string{"up_mbps", "down_mbps"} {
			if _, ok := optionalInteger(brutal, key, 0, 2147483647); !ok {
				return false
			}
		}
	}
	return true
}
