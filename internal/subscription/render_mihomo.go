// SPDX-License-Identifier: GPL-3.0-or-later

package subscription

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
)

type yamlField struct {
	name  string
	value any
}

type mihomoNode []yamlField

func renderMihomo(values []outbound, diagnostics []RenderDiagnostic) (RenderResult, []RenderDiagnostic) {
	nodes := make([]mihomoNode, 0, len(values))
	for _, value := range values {
		node, code := convertMihomo(value)
		if code != "" {
			diagnostics = append(diagnostics, diagnostic(RenderFormatMihomo, value.collection, value.index, code))
			continue
		}
		nodes = append(nodes, node)
	}
	return RenderResult{
		Format:    RenderFormatMihomo,
		MediaType: "application/yaml; charset=utf-8",
		Content:   marshalMihomo(nodes),
		NodeCount: len(nodes),
	}, diagnostics
}

func convertMihomo(value outbound) (mihomoNode, DiagnosticCode) {
	if value.collection == CollectionEndpoints {
		return nil, DiagnosticUnsupportedType
	}
	switch value.typeID {
	case "shadowsocks":
		return mihomoShadowsocks(value)
	case "socks":
		return mihomoSOCKS(value)
	case "http":
		return mihomoHTTP(value)
	case "vmess":
		return mihomoVMess(value)
	case "vless":
		return mihomoVLESS(value)
	case "trojan":
		return mihomoTrojan(value)
	case "hysteria2":
		return mihomoHysteria2(value)
	case "tuic":
		return mihomoTUIC(value)
	case "anytls":
		return mihomoAnyTLS(value)
	default:
		return nil, DiagnosticUnsupportedType
	}
}

func mihomoShadowsocks(value outbound) (mihomoNode, DiagnosticCode) {
	server, port, udp, code := commonServer(value, "method", "network", "password")
	if code != "" {
		return nil, code
	}
	method, methodOK := requiredString(value.value, "method")
	password, passwordOK := requiredString(value.value, "password")
	if !methodOK || !passwordOK {
		return nil, DiagnosticInvalidRequiredField
	}
	if _, supported := mihomoShadowsocksMethods[method]; !supported {
		return nil, DiagnosticUnsupportedOption
	}
	return mihomoCommon(value, "ss", server, port, udp,
		yamlField{name: "cipher", value: method},
		yamlField{name: "password", value: password},
	), ""
}

func mihomoSOCKS(value outbound) (mihomoNode, DiagnosticCode) {
	server, port, udp, code := commonServer(value, "network", "password", "username", "version")
	if code != "" {
		return nil, code
	}
	version, ok := optionalString(value.value, "version")
	if !ok || (version != "" && version != "5") {
		return nil, DiagnosticUnsupportedOption
	}
	username, usernameOK := optionalString(value.value, "username")
	password, passwordOK := optionalString(value.value, "password")
	if !usernameOK || !passwordOK || (username == "") != (password == "") {
		return nil, DiagnosticInvalidRequiredField
	}
	extra := make([]yamlField, 0, 2)
	if username != "" {
		extra = append(extra,
			yamlField{name: "username", value: username},
			yamlField{name: "password", value: password},
		)
	}
	return mihomoCommon(value, "socks5", server, port, udp, extra...), ""
}

func mihomoHTTP(value outbound) (mihomoNode, DiagnosticCode) {
	server, port, udp, code := commonServer(value, "network", "password", "tls", "username")
	if code != "" {
		return nil, code
	}
	username, usernameOK := optionalString(value.value, "username")
	password, passwordOK := optionalString(value.value, "password")
	if !usernameOK || !passwordOK || (username == "") != (password == "") {
		return nil, DiagnosticInvalidRequiredField
	}
	tlsValue, code := parseTLS(value.value, false)
	if code != "" {
		return nil, code
	}
	extra := make([]yamlField, 0, 8)
	if username != "" {
		extra = append(extra,
			yamlField{name: "username", value: username},
			yamlField{name: "password", value: password},
		)
	}
	extra = append(extra, mihomoTLSFields(tlsValue, true, "sni")...)
	return mihomoCommon(value, "http", server, port, udp, extra...), ""
}

func mihomoVMess(value outbound) (mihomoNode, DiagnosticCode) {
	server, port, udp, code := commonServer(
		value, "alter_id", "network", "packet_encoding", "security", "tls", "uuid",
	)
	if code != "" {
		return nil, code
	}
	uuid, ok := requiredString(value.value, "uuid")
	if !ok {
		return nil, DiagnosticInvalidRequiredField
	}
	security, ok := optionalString(value.value, "security")
	if !ok {
		return nil, DiagnosticInvalidRequiredField
	}
	if security == "" {
		security = "auto"
	}
	if _, supported := vmessSecurityMethods[security]; !supported {
		return nil, DiagnosticUnsupportedOption
	}
	alterID, ok := optionalInteger(value.value, "alter_id", 0, 65535)
	if !ok {
		return nil, DiagnosticInvalidRequiredField
	}
	packetEncoding, ok := optionalString(value.value, "packet_encoding")
	if !ok || !oneOf(packetEncoding, "", "packetaddr", "xudp") {
		return nil, DiagnosticInvalidRequiredField
	}
	tlsValue, code := parseTLS(value.value, false)
	if code != "" {
		return nil, code
	}
	extra := []yamlField{
		{name: "uuid", value: uuid},
		{name: "alterId", value: alterID},
		{name: "cipher", value: security},
	}
	if packetEncoding != "" {
		extra = append(extra, yamlField{name: "packet-encoding", value: packetEncoding})
	}
	extra = append(extra, mihomoTLSFields(tlsValue, true, "servername")...)
	return mihomoCommon(value, "vmess", server, port, udp, extra...), ""
}

func mihomoVLESS(value outbound) (mihomoNode, DiagnosticCode) {
	server, port, udp, code := commonServer(
		value, "flow", "network", "packet_encoding", "tls", "uuid",
	)
	if code != "" {
		return nil, code
	}
	uuid, ok := requiredString(value.value, "uuid")
	if !ok {
		return nil, DiagnosticInvalidRequiredField
	}
	flow, flowOK := optionalString(value.value, "flow")
	packetEncoding, packetOK := optionalString(value.value, "packet_encoding")
	if !flowOK || !packetOK || !oneOf(flow, "", "xtls-rprx-vision") ||
		!oneOf(packetEncoding, "", "packetaddr", "xudp") {
		return nil, DiagnosticInvalidRequiredField
	}
	tlsValue, code := parseTLS(value.value, false)
	if code != "" {
		return nil, code
	}
	extra := []yamlField{{name: "uuid", value: uuid}}
	if flow != "" {
		extra = append(extra, yamlField{name: "flow", value: flow})
	}
	if packetEncoding != "" {
		extra = append(extra, yamlField{name: "packet-encoding", value: packetEncoding})
	}
	extra = append(extra, mihomoTLSFields(tlsValue, true, "servername")...)
	return mihomoCommon(value, "vless", server, port, udp, extra...), ""
}

func mihomoTrojan(value outbound) (mihomoNode, DiagnosticCode) {
	server, port, udp, code := commonServer(value, "network", "password", "tls")
	if code != "" {
		return nil, code
	}
	password, ok := requiredString(value.value, "password")
	if !ok {
		return nil, DiagnosticInvalidRequiredField
	}
	tlsValue, code := parseTLS(value.value, true)
	if code != "" {
		return nil, code
	}
	extra := []yamlField{{name: "password", value: password}}
	extra = append(extra, mihomoTLSFields(tlsValue, false, "sni")...)
	return mihomoCommon(value, "trojan", server, port, udp, extra...), ""
}

func mihomoHysteria2(value outbound) (mihomoNode, DiagnosticCode) {
	server, port, udp, code := commonServer(
		value, "down_mbps", "network", "obfs", "password", "tls", "up_mbps",
	)
	if code != "" {
		return nil, code
	}
	password, ok := requiredString(value.value, "password")
	if !ok {
		return nil, DiagnosticInvalidRequiredField
	}
	tlsValue, code := parseTLS(value.value, true)
	if code != "" {
		return nil, code
	}
	extra := []yamlField{{name: "password", value: password}}
	if up, exists, ok := optionalPresentInteger(value.value, "up_mbps", 0, 1_000_000); !ok {
		return nil, DiagnosticInvalidRequiredField
	} else if exists {
		extra = append(extra, yamlField{name: "up", value: fmt.Sprintf("%d Mbps", up)})
	}
	if down, exists, ok := optionalPresentInteger(value.value, "down_mbps", 0, 1_000_000); !ok {
		return nil, DiagnosticInvalidRequiredField
	} else if exists {
		extra = append(extra, yamlField{name: "down", value: fmt.Sprintf("%d Mbps", down)})
	}
	obfsFields, code := mihomoHysteria2Obfs(value.value)
	if code != "" {
		return nil, code
	}
	extra = append(extra, obfsFields...)
	extra = append(extra, mihomoTLSFields(tlsValue, false, "sni")...)
	return mihomoCommon(value, "hysteria2", server, port, udp, extra...), ""
}

func mihomoHysteria2Obfs(value map[string]any) ([]yamlField, DiagnosticCode) {
	raw, exists := value["obfs"]
	if !exists {
		return nil, ""
	}
	object, ok := raw.(map[string]any)
	if !ok {
		return nil, DiagnosticUnsupportedOption
	}
	if code := unsupportedFields(object, map[string]struct{}{"password": {}, "type": {}}); code != "" {
		return nil, DiagnosticUnsupportedOption
	}
	typeID, typeOK := requiredString(object, "type")
	password, passwordOK := requiredString(object, "password")
	if !typeOK || !passwordOK || (typeID != "salamander" && typeID != "gecko") {
		return nil, DiagnosticUnsupportedOption
	}
	return []yamlField{
		{name: "obfs", value: typeID},
		{name: "obfs-password", value: password},
	}, ""
}

func mihomoTUIC(value outbound) (mihomoNode, DiagnosticCode) {
	server, port, udp, code := commonServer(
		value, "congestion_control", "network", "password", "tls", "udp_relay_mode", "uuid", "zero_rtt_handshake",
	)
	if code != "" {
		return nil, code
	}
	uuid, uuidOK := requiredString(value.value, "uuid")
	password, passwordOK := requiredString(value.value, "password")
	if !uuidOK || !passwordOK {
		return nil, DiagnosticInvalidRequiredField
	}
	tlsValue, code := parseTLS(value.value, true)
	if code != "" {
		return nil, code
	}
	extra := []yamlField{{name: "uuid", value: uuid}, {name: "password", value: password}}
	congestion, ok := optionalString(value.value, "congestion_control")
	if !ok || !oneOf(congestion, "", "cubic", "new_reno", "bbr") {
		return nil, DiagnosticInvalidRequiredField
	}
	if congestion != "" {
		extra = append(extra, yamlField{name: "congestion-controller", value: congestion})
	}
	relayMode, ok := optionalString(value.value, "udp_relay_mode")
	if !ok || !oneOf(relayMode, "", "native", "quic") {
		return nil, DiagnosticInvalidRequiredField
	}
	if relayMode != "" {
		extra = append(extra, yamlField{name: "udp-relay-mode", value: relayMode})
	}
	zeroRTT, ok := optionalBool(value.value, "zero_rtt_handshake")
	if !ok {
		return nil, DiagnosticInvalidRequiredField
	}
	if _, exists := value.value["zero_rtt_handshake"]; exists {
		extra = append(extra, yamlField{name: "reduce-rtt", value: zeroRTT})
	}
	extra = append(extra, mihomoTLSFields(tlsValue, false, "sni")...)
	return mihomoCommon(value, "tuic", server, port, udp, extra...), ""
}

func mihomoAnyTLS(value outbound) (mihomoNode, DiagnosticCode) {
	server, port, udp, code := commonServer(value, "network", "password", "tls")
	if code != "" {
		return nil, code
	}
	password, ok := requiredString(value.value, "password")
	if !ok {
		return nil, DiagnosticInvalidRequiredField
	}
	tlsValue, code := parseTLS(value.value, true)
	if code != "" {
		return nil, code
	}
	extra := []yamlField{{name: "password", value: password}}
	extra = append(extra, mihomoTLSFields(tlsValue, false, "sni")...)
	return mihomoCommon(value, "anytls", server, port, udp, extra...), ""
}

func mihomoCommon(
	value outbound,
	typeID string,
	server string,
	port int64,
	udp bool,
	extra ...yamlField,
) mihomoNode {
	fields := mihomoNode{
		{name: "name", value: value.tag},
		{name: "type", value: typeID},
		{name: "server", value: server},
		{name: "port", value: port},
		{name: "udp", value: udp},
	}
	return append(fields, extra...)
}

func mihomoTLSFields(value tlsOptions, emitTLS bool, serverNameKey string) []yamlField {
	if !value.present {
		return nil
	}
	fields := make([]yamlField, 0, 4)
	if emitTLS {
		fields = append(fields, yamlField{name: "tls", value: value.enabled})
	}
	if !value.enabled {
		return fields
	}
	if value.serverName != "" {
		fields = append(fields, yamlField{name: serverNameKey, value: value.serverName})
	}
	if len(value.alpn) > 0 {
		fields = append(fields, yamlField{name: "alpn", value: value.alpn})
	}
	fields = append(fields, yamlField{name: "skip-cert-verify", value: value.insecure})
	return fields
}

var mihomoShadowsocksMethods = map[string]struct{}{
	"2022-blake3-aes-128-gcm":       {},
	"2022-blake3-aes-256-gcm":       {},
	"2022-blake3-chacha20-poly1305": {},
	"aes-128-ccm":                   {},
	"aes-128-cfb":                   {},
	"aes-128-ctr":                   {},
	"aes-128-gcm":                   {},
	"aes-128-gcm-siv":               {},
	"aes-192-ccm":                   {},
	"aes-192-cfb":                   {},
	"aes-192-ctr":                   {},
	"aes-192-gcm":                   {},
	"aes-256-ccm":                   {},
	"aes-256-cfb":                   {},
	"aes-256-ctr":                   {},
	"aes-256-gcm":                   {},
	"aes-256-gcm-siv":               {},
	"aegis-128l":                    {},
	"aegis-256":                     {},
	"aez-384":                       {},
	"chacha20":                      {},
	"chacha20-ietf":                 {},
	"chacha20-ietf-poly1305":        {},
	"chacha8-ietf-poly1305":         {},
	"deoxys-ii-256-128":             {},
	"lea-128-gcm":                   {},
	"lea-192-gcm":                   {},
	"lea-256-gcm":                   {},
	"none":                          {},
	"rabbit128-poly1305":            {},
	"rc4-md5":                       {},
	"xchacha20":                     {},
	"xchacha20-ietf-poly1305":       {},
	"xchacha8-ietf-poly1305":        {},
}

var vmessSecurityMethods = map[string]struct{}{
	"aes-128-gcm":       {},
	"auto":              {},
	"chacha20-poly1305": {},
	"none":              {},
	"zero":              {},
}

func oneOf(value string, allowed ...string) bool {
	for _, candidate := range allowed {
		if value == candidate {
			return true
		}
	}
	return false
}

func optionalPresentInteger(value map[string]any, key string, minimum, maximum int64) (int64, bool, bool) {
	raw, exists := value[key]
	if !exists {
		return 0, false, true
	}
	parsed, ok := integer(raw, minimum, maximum)
	return parsed, true, ok
}

func marshalMihomo(nodes []mihomoNode) []byte {
	if len(nodes) == 0 {
		return []byte("proxies: []\n")
	}
	var output bytes.Buffer
	output.WriteString("proxies:\n")
	for _, node := range nodes {
		for index, field := range node {
			if index == 0 {
				output.WriteString("  - ")
			} else {
				output.WriteString("    ")
			}
			output.WriteString(field.name)
			output.WriteString(": ")
			output.WriteString(yamlScalar(field.value))
			output.WriteByte('\n')
		}
	}
	return output.Bytes()
}

func yamlScalar(value any) string {
	switch typed := value.(type) {
	case string:
		encoded, err := json.Marshal(typed)
		if err != nil {
			panic(err)
		}
		return string(encoded)
	case bool:
		return strconv.FormatBool(typed)
	case int64:
		return strconv.FormatInt(typed, 10)
	case []string:
		encoded, err := json.Marshal(typed)
		if err != nil {
			panic(err)
		}
		return string(encoded)
	default:
		panic(fmt.Sprintf("unsupported YAML scalar %T", value))
	}
}
