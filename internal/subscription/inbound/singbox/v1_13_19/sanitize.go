// SPDX-License-Identifier: GPL-3.0-or-later

package singbox11319

import (
	"strings"

	"github.com/rehuony/sing-box-panel/internal/subscription/inbound"
)

func sanitizedClientTLS(raw any, publicHost string) (map[string]any, bool) {
	value, ok := raw.(map[string]any)
	if !ok {
		return nil, false
	}
	enabled, _ := value["enabled"].(bool)
	if !enabled {
		return nil, false
	}
	result := map[string]any{"enabled": true, "insecure": false}
	if !isIPAddress(publicHost) {
		result["server_name"] = publicHost
	}
	if alpn, ok := value["alpn"].([]any); ok && len(alpn) <= 16 {
		clean := make([]any, 0, len(alpn))
		for _, item := range alpn {
			text, ok := item.(string)
			if !ok || text == "" || len(text) > 256 {
				return nil, false
			}
			clean = append(clean, text)
		}
		result["alpn"] = clean
	}
	return result, true
}

func sanitizedClientTransport(raw any) (map[string]any, bool) {
	value, ok := raw.(map[string]any)
	if !ok {
		return nil, false
	}
	allowed := map[string]struct{}{
		"type": {}, "path": {}, "headers": {}, "method": {}, "host": {},
		"service_name": {}, "max_early_data": {}, "early_data_header_name": {},
	}
	result := make(map[string]any)
	for name, item := range value {
		if _, ok := allowed[name]; ok {
			result[name] = item
		}
	}
	if _, ok := result["type"].(string); !ok {
		return nil, false
	}
	return result, true
}

func normalizedPublicHost(value string) (string, error) {
	if value == "" || value != strings.TrimSpace(value) || strings.ContainsAny(value, "/@?#[]") {
		return "", inbound.ErrInvalidPublicHost
	}
	return value, nil
}

func isIPAddress(value string) bool {
	for _, character := range value {
		if (character < '0' || character > '9') && character != '.' && character != ':' &&
			(character < 'a' || character > 'f') && (character < 'A' || character > 'F') {
			return false
		}
	}
	return strings.ContainsAny(value, ".:")
}
