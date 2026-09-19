// SPDX-License-Identifier: GPL-3.0-or-later

package singbox

import (
	"crypto/ecdh"
	"encoding/base64"
	"errors"
	"net/netip"
)

// Only client-facing TLS fields cross the publication boundary. In particular,
// certificate paths, private keys, ACME credentials and handshake dial settings
// belong to the server and must never be copied to a subscription.
func sanitizedClientTLS(raw any, publicHost string) (map[string]any, error) {
	value, ok := raw.(map[string]any)
	if !ok || value["enabled"] != true {
		return nil, nil
	}
	result := map[string]any{"enabled": true, "insecure": false}
	serverName := firstNonEmptyString(value, "server_name")
	if serverName == "" {
		if _, err := netip.ParseAddr(publicHost); err != nil {
			serverName = publicHost
		}
	}
	if alpn, exists := value["alpn"]; exists {
		switch list := alpn.(type) {
		case string:
			result["alpn"] = list
		case []any:
			if len(list) > 16 {
				return nil, errors.New("invalid TLS ALPN")
			}
			for _, item := range list {
				if text, ok := item.(string); !ok || len(text) == 0 || len(text) > 256 {
					return nil, errors.New("invalid TLS ALPN")
				}
			}
			result["alpn"] = list
		default:
			return nil, errors.New("invalid TLS ALPN")
		}
	}
	if reality, ok := value["reality"].(map[string]any); ok && reality["enabled"] == true {
		encoded := firstNonEmptyString(reality, "private_key")
		keyBytes, err := base64.RawURLEncoding.DecodeString(encoded)
		if err != nil {
			return nil, errors.New("invalid Reality server key")
		}
		privateKey, err := ecdh.X25519().NewPrivateKey(keyBytes)
		if err != nil {
			return nil, errors.New("invalid Reality server key")
		}
		client := map[string]any{"enabled": true, "public_key": base64.RawURLEncoding.EncodeToString(privateKey.PublicKey().Bytes())}
		switch shortID := reality["short_id"].(type) {
		case string:
			client["short_id"] = shortID
		case []any:
			if len(shortID) > 0 {
				client["short_id"] = shortID[0]
			}
		}
		if firstNonEmptyString(value, "server_name") == "" {
			if handshake, ok := reality["handshake"].(map[string]any); ok {
				serverName = firstNonEmptyString(handshake, "server")
			}
		}
		if serverName == "" {
			return nil, errors.New("missing Reality handshake server name")
		}
		result["reality"] = client
		result["utls"] = map[string]any{"enabled": true, "fingerprint": "chrome"}
	}
	if serverName != "" {
		result["server_name"] = serverName
	}
	return result, nil
}
