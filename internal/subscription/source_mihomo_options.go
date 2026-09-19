// SPDX-License-Identifier: GPL-3.0-or-later

package subscription

import "errors"

// Reject options without a proven native equivalent instead of returning a
// usable-looking node with different connection or authentication behavior.
func convertMihomoSourceOptions(result, proxy map[string]any) error {
	typeID, _ := result["type"].(string)
	allowed := map[string]bool{}
	for _, key := range []string{"name", "type", "server", "port", "udp", "tls", "sni", "servername", "peer", "skip-cert-verify", "alpn", "client-fingerprint", "reality-opts", "network", "ws-opts", "grpc-opts", "smux"} {
		allowed[key] = true
	}
	protocolFields := map[string][]string{
		"shadowsocks": {"cipher", "password"}, "socks": {"username", "password"}, "http": {"username", "password"},
		"vmess": {"uuid", "cipher", "alterId", "packet-encoding"}, "vless": {"uuid", "flow", "packet-encoding"},
		"trojan": {"password"}, "hysteria": {"auth-str", "auth"}, "hysteria2": {"password", "up", "down", "obfs", "obfs-password"},
		"tuic": {"uuid", "password", "congestion-controller", "udp-relay-mode", "reduce-rtt"}, "anytls": {"password"},
	}
	for _, key := range protocolFields[typeID] {
		allowed[key] = true
	}
	for key := range proxy {
		if !allowed[key] {
			return errors.New("unsupported YAML proxy option")
		}
	}
	for _, key := range []string{"udp", "tls", "skip-cert-verify"} {
		if _, ok := optionalBool(proxy, key); !ok {
			return errors.New("invalid YAML boolean option")
		}
	}
	for _, key := range []string{"sni", "servername", "peer", "client-fingerprint"} {
		if _, ok := optionalString(proxy, key); !ok {
			return errors.New("invalid YAML TLS value")
		}
	}
	if raw, exists := proxy["alpn"]; exists {
		values, ok := raw.([]any)
		if !ok {
			return errors.New("invalid YAML ALPN")
		}
		for _, value := range values {
			if s, ok := value.(string); !ok || s == "" {
				return errors.New("invalid YAML ALPN")
			}
		}
	}
	if udp, exists := proxy["udp"]; exists && udp == false {
		result["network"] = "tcp"
	}
	copyRenamed(result, proxy, "packet_encoding", "packet-encoding")
	if typeID == "hysteria2" {
		for _, pair := range [][2]string{{"up_mbps", "up"}, {"down_mbps", "down"}} {
			if raw, exists := proxy[pair[1]]; exists {
				value, ok := sourceInteger(raw, 0, 2147483647)
				if !ok {
					return errors.New("unsupported YAML bandwidth unit")
				}
				result[pair[0]] = value
			}
		}
		if raw, exists := proxy["obfs"]; exists {
			if raw != "salamander" {
				return errors.New("unsupported YAML obfuscation")
			}
			password, ok := requiredString(proxy, "obfs-password")
			if !ok {
				return errors.New("invalid YAML obfuscation")
			}
			result["obfs"] = map[string]any{"type": "salamander", "password": password}
		} else if proxy["obfs-password"] != nil {
			return errors.New("invalid YAML obfuscation")
		}
	}
	if raw, exists := proxy["network"]; exists {
		network, ok := raw.(string)
		if !ok || !oneOf(network, "tcp", "ws", "grpc") {
			return errors.New("unsupported YAML transport")
		}
		if network != "tcp" {
			transport := map[string]any{"type": network}
			if raw, exists := proxy[network+"-opts"]; exists {
				options, ok := raw.(map[string]any)
				if !ok {
					return errors.New("invalid YAML transport")
				}
				keys := map[string]string{"path": "path", "headers": "headers", "max-early-data": "max_early_data", "early-data-header-name": "early_data_header_name"}
				if network == "grpc" {
					keys = map[string]string{"grpc-service-name": "service_name"}
				}
				for key, value := range options {
					name := keys[key]
					if name == "" {
						return errors.New("unsupported YAML transport option")
					}
					transport[name] = value
				}
			}
			result["transport"] = transport
		}
	}
	for _, network := range []string{"ws", "grpc"} {
		if proxy[network+"-opts"] != nil && proxy["network"] != network {
			return errors.New("YAML transport options mismatch")
		}
	}
	if raw, exists := proxy["smux"]; exists {
		options, ok := raw.(map[string]any)
		if !ok {
			return errors.New("invalid YAML multiplex options")
		}
		mux := map[string]any{}
		keys := map[string]string{"enabled": "enabled", "protocol": "protocol", "max-connections": "max_connections", "min-streams": "min_streams", "max-streams": "max_streams", "padding": "padding", "brutal-opts": "brutal"}
		for key, value := range options {
			name := keys[key]
			if name == "" {
				return errors.New("unsupported YAML multiplex option")
			}
			if key == "brutal-opts" {
				options, ok := value.(map[string]any)
				if !ok {
					return errors.New("invalid YAML brutal options")
				}
				brutal := map[string]any{}
				for k, v := range options {
					n := map[string]string{"enabled": "enabled", "up": "up_mbps", "down": "down_mbps"}[k]
					if n == "" {
						return errors.New("unsupported YAML brutal option")
					}
					brutal[n] = v
				}
				value = brutal
			}
			mux[name] = value
		}
		result["multiplex"] = mux
	}
	return nil
}
