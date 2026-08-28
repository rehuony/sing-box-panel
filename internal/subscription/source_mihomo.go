// SPDX-License-Identifier: GPL-3.0-or-later

package subscription

import (
	"bytes"
	"errors"
	"strings"

	"go.yaml.in/yaml/v3"
)

func parseMihomoSource(raw []byte) ([]map[string]any, error) {
	decoder := yaml.NewDecoder(bytes.NewReader(raw))
	decoder.KnownFields(false)
	var yamlDocument yaml.Node
	if err := decoder.Decode(&yamlDocument); err != nil {
		return nil, errors.New("malformed YAML")
	}
	var extra yaml.Node
	if err := decoder.Decode(&extra); err == nil && len(extra.Content) != 0 {
		return nil, errors.New("multiple YAML documents")
	}
	count := 0
	if err := inspectSourceYAML(&yamlDocument, 0, &count); err != nil {
		return nil, err
	}
	var root map[string]any
	if err := yamlDocument.Decode(&root); err != nil || root == nil {
		return nil, errors.New("YAML root is not an object")
	}
	rawProxies, ok := root["proxies"].([]any)
	if !ok {
		return nil, errors.New("YAML proxies is not an array")
	}
	values := make([]map[string]any, 0, len(rawProxies))
	for _, rawProxy := range rawProxies {
		proxy, ok := rawProxy.(map[string]any)
		if !ok {
			return nil, errors.New("YAML proxy is not an object")
		}
		converted, err := convertMihomoProxy(proxy)
		if err != nil {
			return nil, err
		}
		values = append(values, converted)
	}
	return values, nil
}

func inspectSourceYAML(value *yaml.Node, depth int, count *int) error {
	if depth > MaximumDocumentDepth {
		return errors.New("YAML nesting too deep")
	}
	*count++
	if *count > MaximumDocumentValues {
		return errors.New("YAML has too many values")
	}
	if value.Kind == yaml.AliasNode || value.Anchor != "" {
		return errors.New("YAML aliases and anchors are not allowed")
	}
	if value.Kind == yaml.MappingNode {
		if len(value.Content)%2 != 0 {
			return errors.New("malformed YAML mapping")
		}
		seen := make(map[string]struct{}, len(value.Content)/2)
		for index := 0; index < len(value.Content); index += 2 {
			key := value.Content[index]
			if key.Kind != yaml.ScalarNode || key.Value == "" {
				return errors.New("invalid YAML mapping key")
			}
			if _, duplicate := seen[key.Value]; duplicate {
				return errors.New("duplicate YAML mapping key")
			}
			seen[key.Value] = struct{}{}
		}
	}
	for _, child := range value.Content {
		if err := inspectSourceYAML(child, depth+1, count); err != nil {
			return err
		}
	}
	return nil
}

func convertMihomoProxy(proxy map[string]any) (map[string]any, error) {
	typeID, _ := proxy["type"].(string)
	tag, _ := proxy["name"].(string)
	server, _ := proxy["server"].(string)
	port, ok := sourceInteger(proxy["port"], 1, 65535)
	typeMap := map[string]string{
		"ss": "shadowsocks", "socks5": "socks", "http": "http", "vmess": "vmess",
		"vless": "vless", "trojan": "trojan", "hysteria": "hysteria",
		"hysteria2": "hysteria2", "tuic": "tuic", "anytls": "anytls",
	}
	outboundType, supported := typeMap[strings.ToLower(typeID)]
	if !supported || !ValidTag(tag) || server == "" || !ok {
		return nil, errors.New("invalid or unsupported YAML proxy")
	}
	result := map[string]any{"type": outboundType, "tag": tag, "server": server, "server_port": port}
	copyMihomoFields(result, proxy, outboundType)
	if tlsEnabled, _ := proxy["tls"].(bool); tlsEnabled || outboundType == "trojan" || outboundType == "hysteria2" || outboundType == "tuic" || outboundType == "anytls" {
		tls := map[string]any{"enabled": true}
		if sni := firstString(proxy, "servername", "sni", "peer"); sni != "" {
			tls["server_name"] = sni
		}
		if insecure, ok := proxy["skip-cert-verify"].(bool); ok {
			tls["insecure"] = insecure
		}
		if alpn, ok := proxy["alpn"].([]any); ok {
			tls["alpn"] = alpn
		}
		result["tls"] = tls
	}
	return result, nil
}

func copyMihomoFields(result, proxy map[string]any, typeID string) {
	switch typeID {
	case "shadowsocks":
		copyRenamed(result, proxy, "method", "cipher")
		copyRenamed(result, proxy, "password", "password")
	case "socks", "http":
		copyRenamed(result, proxy, "username", "username")
		copyRenamed(result, proxy, "password", "password")
	case "vmess":
		copyRenamed(result, proxy, "uuid", "uuid")
		copyRenamed(result, proxy, "security", "cipher")
		copyRenamed(result, proxy, "alter_id", "alterId")
	case "vless":
		copyRenamed(result, proxy, "uuid", "uuid")
		copyRenamed(result, proxy, "flow", "flow")
	case "trojan", "hysteria2", "anytls":
		copyRenamed(result, proxy, "password", "password")
	case "hysteria":
		copyRenamed(result, proxy, "auth_str", "auth-str")
		copyRenamed(result, proxy, "auth", "auth")
	case "tuic":
		copyRenamed(result, proxy, "uuid", "uuid")
		copyRenamed(result, proxy, "password", "password")
		copyRenamed(result, proxy, "congestion_control", "congestion-controller")
		copyRenamed(result, proxy, "udp_relay_mode", "udp-relay-mode")
		copyRenamed(result, proxy, "zero_rtt_handshake", "reduce-rtt")
	}
}
