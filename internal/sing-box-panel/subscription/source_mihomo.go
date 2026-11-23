// SPDX-License-Identifier: GPL-3.0-or-later

package subscription

import (
	"bytes"
	"errors"
	"fmt"
	"maps"
	"regexp"
	"strings"

	"go.yaml.in/yaml/v3"
)

// mihomoSourceParser reads a Mihomo configuration's proxies, or a proxy
// list. Proxy groups, rules and proxy providers are not nodes, and providers
// are never fetched.
type mihomoSourceParser struct{}

func (mihomoSourceParser) Format() SourceFormat { return SourceFormatMihomo }

var (
	mihomoProxiesKey  = regexp.MustCompile(`(?m)^proxies[ \t]*:`)
	mihomoProxyListed = regexp.MustCompile(`(?m)^-[ \t]+\{?[ \t]*name[ \t]*:`)
)

func (mihomoSourceParser) Sniff(body []byte) int {
	switch {
	case mihomoProxiesKey.Match(body):
		return 100
	case bytes.Contains(body, []byte(`"proxies"`)):
		return 90
	case mihomoProxyListed.Match(body):
		return 50
	}
	return 0
}

func (mihomoSourceParser) Items(body []byte) ([]SourceItem, error) {
	decoder := yaml.NewDecoder(bytes.NewReader(body))
	decoder.KnownFields(false)
	var document yaml.Node
	if err := decoder.Decode(&document); err != nil {
		return nil, contentError("", "the YAML document is invalid")
	}
	var extra yaml.Node
	if err := decoder.Decode(&extra); err == nil && len(extra.Content) != 0 {
		return nil, contentError("", "the content holds several YAML documents")
	}
	count := 0
	if err := inspectSourceYAML(&document, 0, &count); err != nil {
		return nil, contentError("", err.Error())
	}
	var root any
	if err := document.Decode(&root); err != nil {
		return nil, contentError("", "the YAML document is invalid")
	}
	var proxies []any
	prefix := ""
	switch value := root.(type) {
	case []any:
		proxies = value
	case map[string]any:
		raw, exists := value["proxies"]
		if !exists {
			return nil, contentError("", "the configuration has no proxies")
		}
		if raw != nil {
			list, ok := raw.([]any)
			if !ok {
				return nil, contentError("proxies", "not a list")
			}
			proxies = list
		}
		prefix = "proxies"
	default:
		return nil, contentError("", "the document is not a configuration or proxy list")
	}
	items := make([]SourceItem, 0, len(proxies))
	for index, raw := range proxies {
		location := fmt.Sprintf("%s[%d]", prefix, index)
		proxy, ok := raw.(map[string]any)
		if !ok {
			return nil, contentError(location, "not a mapping")
		}
		kind, _ := proxy["type"].(string)
		protocol := mihomoProxyTypes[strings.ToLower(kind)]
		if protocol == "" {
			protocol = strings.ToLower(kind)
		}
		name, _ := proxy["name"].(string)
		if name == "" {
			server, _ := proxy["server"].(string)
			port, _ := DocumentInteger(proxy["port"], 1, 65535)
			name = unnamedNode(protocol, server, port)
		}
		items = append(items, SourceItem{Location: location, Name: name, Protocol: protocol, convert: func() (map[string]any, error) {
			named := maps.Clone(proxy)
			named["name"] = name
			return convertMihomoProxy(named)
		}})
	}
	return items, nil
}

// mihomoProxyTypes maps the Mihomo proxy types sources convert to sing-box
// outbound types.
var mihomoProxyTypes = map[string]string{
	"ss": "shadowsocks", "socks5": "socks", "http": "http", "vmess": "vmess",
	"vless": "vless", "trojan": "trojan", "hysteria": "hysteria",
	"hysteria2": "hysteria2", "tuic": "tuic", "anytls": "anytls",
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
	port, ok := DocumentInteger(proxy["port"], 1, 65535)
	outboundType, supported := mihomoProxyTypes[strings.ToLower(typeID)]
	if !supported {
		return nil, errors.New("unsupported proxy type")
	}
	if !ValidTag(tag) || server == "" || !ok {
		return nil, errors.New("the proxy's name, server or port is invalid")
	}
	result := map[string]any{"type": outboundType, "tag": tag, "server": server, "server_port": port}
	copyMihomoFields(result, proxy, outboundType)
	if err := convertMihomoSourceOptions(result, proxy); err != nil {
		return nil, err
	}
	if tlsEnabled, _ := proxy["tls"].(bool); tlsEnabled || outboundType == "trojan" || outboundType == "hysteria" || outboundType == "hysteria2" || outboundType == "tuic" || outboundType == "anytls" {
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
		if fingerprint := firstString(proxy, "client-fingerprint"); fingerprint != "" {
			tls["utls"] = map[string]any{"enabled": true, "fingerprint": fingerprint}
		}
		if raw, exists := proxy["reality-opts"]; exists {
			options, ok := raw.(map[string]any)
			if !ok {
				return nil, errors.New("invalid YAML TLS options")
			}
			reality := map[string]any{"enabled": true}
			for key, value := range options {
				name := map[string]string{"public-key": "public_key", "short-id": "short_id"}[key]
				if name == "" {
					return nil, errors.New("unsupported YAML TLS option")
				}
				reality[name] = value
			}
			tls["reality"] = reality
		}
	} else if proxy["client-fingerprint"] != nil || proxy["reality-opts"] != nil || proxy["sni"] != nil || proxy["servername"] != nil || proxy["peer"] != nil || proxy["alpn"] != nil || proxy["skip-cert-verify"] != nil {
		return nil, errors.New("YAML TLS option requires TLS")
	}
	if _, _, code := mihomoMappedOptions(outbound{typeID: outboundType, value: result}); code != "" {
		return nil, errors.New("invalid or unsupported YAML connection options")
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
