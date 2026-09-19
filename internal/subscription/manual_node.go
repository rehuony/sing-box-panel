// SPDX-License-Identifier: GPL-3.0-or-later

package subscription

import (
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
)

// NodeFieldError contains only an allowlisted field path and a fixed code;
// neither credentials nor values from the input appear in API errors.
type NodeFieldError struct{ Path, Code string }

func (e *NodeFieldError) Error() string { return fmt.Sprintf("outbound/%s: %s", e.Path, e.Code) }

func ValidateManualNode(raw []byte) error {
	decoded, err := DecodeDocument(raw)
	if err != nil {
		return &NodeFieldError{Code: "invalid_json"}
	}
	value, ok := decoded.(map[string]any)
	if !ok {
		return &NodeFieldError{Code: "object_required"}
	}
	typeID, _ := value["type"].(string)
	switch typeID {
	case "socks", "http", "shadowsocks", "vmess", "vless", "trojan", "hysteria", "hysteria2", "tuic", "anytls", "naive", "shadowtls", "snell", "ssh":
	default:
		return &NodeFieldError{Path: "type", Code: "unsupported_node_protocol"}
	}
	str := func(key string) string { s, _ := value[key].(string); return s }
	if !ValidTag(str("tag")) {
		return &NodeFieldError{Path: "tag", Code: "required"}
	}
	if !validNodeCoordinate(value) {
		return &NodeFieldError{Path: "server", Code: "invalid_endpoint"}
	}
	if err := validateSourceRequiredFields(typeID, value); err != nil {
		return &NodeFieldError{Code: "missing_protocol_credential"}
	}
	if str("detour") == str("tag") {
		return &NodeFieldError{Path: "detour", Code: "dependency_cycle"}
	}
	tls, _ := value["tls"].(map[string]any)
	reality, _ := tls["reality"].(map[string]any)
	quic := typeID == "hysteria" || typeID == "hysteria2" || typeID == "tuic"
	if (quic || typeID == "anytls" || typeID == "naive") && tls["enabled"] != true {
		return &NodeFieldError{Path: "tls/enabled", Code: "required"}
	}
	if quic {
		utls, _ := tls["utls"].(map[string]any)
		if reality["enabled"] == true || utls["enabled"] == true || tls["fragment"] == true || tls["record_fragment"] == true {
			return &NodeFieldError{Path: "tls", Code: "tcp_tls_option_on_quic"}
		}
	}
	if reality["enabled"] == true {
		key, _ := reality["public_key"].(string)
		decoded, err := base64.RawURLEncoding.DecodeString(key)
		if err != nil || len(decoded) != 32 {
			return &NodeFieldError{Path: "tls/reality/public_key", Code: "invalid_key"}
		}
		short, _ := reality["short_id"].(string)
		if _, err := hex.DecodeString(short); err != nil || len(short) > 16 {
			return &NodeFieldError{Path: "tls/reality/short_id", Code: "invalid_hex"}
		}
		if tls["enabled"] != true {
			return &NodeFieldError{Path: "tls/enabled", Code: "required"}
		}
	}
	if typeID == "shadowsocks" && optionEnabled(value["udp_over_tcp"]) && optionEnabled(value["multiplex"]) {
		return &NodeFieldError{Path: "udp_over_tcp", Code: "conflicts_with_multiplex"}
	}
	if typeID == "hysteria2" && value["server_ports"] != nil && value["server_port"] != nil {
		return &NodeFieldError{Path: "server_ports", Code: "conflicts_with_server_port"}
	}
	if mux, ok := value["multiplex"].(map[string]any); ok && !validMuxOptions(mux) {
		return &NodeFieldError{Path: "multiplex", Code: "invalid_or_conflicting_limits"}
	}
	return nil
}

func optionEnabled(value any) bool {
	if enabled, ok := value.(bool); ok {
		return enabled
	}
	object, _ := value.(map[string]any)
	return object["enabled"] == true
}

func validNodeCoordinate(value map[string]any) bool {
	typeID, _ := value["type"].(string)
	if realm, ok := value["realm"].(map[string]any); ok && typeID == "hysteria2" {
		address, _ := realm["server_url"].(string)
		parsed, err := url.Parse(address)
		id, _ := realm["realm_id"].(string)
		stun, hasStun := realm["stun_servers"]
		stunOK := false
		switch v := stun.(type) {
		case string:
			stunOK = v != ""
		case []any:
			stunOK = len(v) > 0
			for _, item := range v {
				if s, ok := item.(string); !ok || s == "" {
					stunOK = false
				}
			}
		}
		return err == nil && parsed.Host != "" && (parsed.Scheme == "http" || parsed.Scheme == "https") && id != "" && hasStun && stunOK && value["server"] == nil && value["server_port"] == nil && value["server_ports"] == nil
	}
	server, _ := value["server"].(string)
	if server == "" || strings.TrimSpace(server) != server || strings.ContainsAny(server, "/\\\r\n\t ") {
		return false
	}
	if strings.Contains(server, ":") && net.ParseIP(strings.Trim(server, "[]")) == nil {
		return false
	}
	if typeID == "hysteria2" && value["server_ports"] != nil {
		return validPortRanges(value["server_ports"])
	}
	if typeID == "ssh" && value["server_port"] == nil {
		return true
	}
	_, ok := sourceInteger(value["server_port"], 1, 65535)
	return ok
}

func validPortRanges(value any) bool {
	values, ok := value.([]any)
	if !ok {
		if s, isString := value.(string); isString {
			values = []any{s}
		} else {
			return false
		}
	}
	if len(values) == 0 {
		return false
	}
	for _, value := range values {
		s, ok := value.(string)
		if !ok {
			return false
		}
		parts := strings.Split(s, ":")
		if len(parts) > 2 {
			return false
		}
		last := 0
		for _, part := range parts {
			port, err := strconv.Atoi(part)
			if err != nil || port < 1 || port > 65535 || port < last {
				return false
			}
			last = port
		}
	}
	return true
}
