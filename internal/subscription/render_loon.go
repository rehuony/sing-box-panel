// SPDX-License-Identifier: GPL-3.0-or-later

package subscription

import (
	"bytes"
	"encoding/json"
	"strconv"
	"strings"
)

type loonNode struct {
	name  string
	parts []string
}

func renderLoon(values []outbound, diagnostics []RenderDiagnostic) (RenderResult, []RenderDiagnostic) {
	nodes := make([]loonNode, 0, len(values))
	for _, value := range values {
		node, code := convertLoon(value)
		if code != "" {
			diagnostics = append(diagnostics, diagnostic(RenderFormatLoon, value.collection, value.index, code))
			continue
		}
		nodes = append(nodes, node)
	}
	return RenderResult{
		Format:    RenderFormatLoon,
		MediaType: "text/plain; charset=utf-8",
		Content:   marshalLoon(nodes),
		NodeCount: len(nodes),
	}, diagnostics
}

func convertLoon(value outbound) (loonNode, DiagnosticCode) {
	if value.collection == CollectionEndpoints {
		return loonNode{}, DiagnosticUnsupportedType
	}
	if !validLoonName(value.tag) {
		return loonNode{}, DiagnosticInvalidMetadata
	}
	switch value.typeID {
	case "shadowsocks":
		return loonShadowsocks(value)
	case "socks":
		return loonSOCKS(value)
	case "http":
		return loonHTTP(value)
	case "vmess":
		return loonVMess(value)
	case "vless":
		return loonVLESS(value)
	case "trojan":
		return loonTrojan(value)
	case "hysteria2":
		return loonHysteria2(value)
	case "anytls":
		return loonAnyTLS(value)
	default:
		return loonNode{}, DiagnosticUnsupportedType
	}
}

func loonShadowsocks(value outbound) (loonNode, DiagnosticCode) {
	server, port, udp, code := commonServer(value, "method", "network", "password")
	if code != "" {
		return loonNode{}, code
	}
	method, methodOK := requiredString(value.value, "method")
	password, passwordOK := requiredString(value.value, "password")
	if !methodOK || !passwordOK || !validLoonAtom(server) || !validLoonAtom(method) {
		return loonNode{}, DiagnosticInvalidRequiredField
	}
	if _, supported := loonShadowsocksMethods[method]; !supported {
		return loonNode{}, DiagnosticUnsupportedOption
	}
	return loonNode{
		name: value.tag,
		parts: []string{
			"Shadowsocks", server, strconv.FormatInt(port, 10), method,
			loonQuoted(password), loonOptionBool("udp", udp),
		},
	}, ""
}

func loonSOCKS(value outbound) (loonNode, DiagnosticCode) {
	server, port, udp, code := commonServer(value, "network", "password", "username", "version")
	if code != "" {
		return loonNode{}, code
	}
	if !validLoonAtom(server) {
		return loonNode{}, DiagnosticInvalidRequiredField
	}
	version, ok := optionalString(value.value, "version")
	if !ok || (version != "" && version != "5") {
		return loonNode{}, DiagnosticUnsupportedOption
	}
	username, usernameOK := optionalString(value.value, "username")
	password, passwordOK := optionalString(value.value, "password")
	if !usernameOK || !passwordOK || (username == "") != (password == "") {
		return loonNode{}, DiagnosticInvalidRequiredField
	}
	parts := []string{"socks5", server, strconv.FormatInt(port, 10)}
	if username != "" {
		parts = append(parts, loonQuoted(username), loonQuoted(password))
	}
	parts = append(parts, loonOptionBool("udp", udp))
	return loonNode{name: value.tag, parts: parts}, ""
}

func loonHTTP(value outbound) (loonNode, DiagnosticCode) {
	server, port, _, code := commonServer(value, "network", "password", "tls", "username")
	if code != "" {
		return loonNode{}, code
	}
	if !validLoonAtom(server) {
		return loonNode{}, DiagnosticInvalidRequiredField
	}
	username, usernameOK := optionalString(value.value, "username")
	password, passwordOK := optionalString(value.value, "password")
	if !usernameOK || !passwordOK || (username == "") != (password == "") {
		return loonNode{}, DiagnosticInvalidRequiredField
	}
	tlsValue, code := parseTLS(value.value, false)
	if code != "" {
		return loonNode{}, code
	}
	protocol := "http"
	if tlsValue.present && tlsValue.enabled {
		protocol = "https"
	}
	parts := []string{protocol, server, strconv.FormatInt(port, 10)}
	if username != "" {
		parts = append(parts, loonQuoted(username), loonQuoted(password))
	}
	tlsParts, code := loonTLSOptions(tlsValue, true)
	if code != "" {
		return loonNode{}, code
	}
	parts = append(parts, tlsParts...)
	return loonNode{name: value.tag, parts: parts}, ""
}

func loonVMess(value outbound) (loonNode, DiagnosticCode) {
	server, port, udp, code := commonServer(value, "alter_id", "network", "security", "tls", "uuid")
	if code != "" {
		return loonNode{}, code
	}
	uuid, uuidOK := requiredString(value.value, "uuid")
	security, securityOK := optionalString(value.value, "security")
	if !uuidOK || !securityOK || !validLoonAtom(server) {
		return loonNode{}, DiagnosticInvalidRequiredField
	}
	if security == "" {
		security = "auto"
	}
	if !validLoonAtom(security) {
		return loonNode{}, DiagnosticInvalidRequiredField
	}
	if _, supported := vmessSecurityMethods[security]; !supported {
		return loonNode{}, DiagnosticUnsupportedOption
	}
	alterID, ok := optionalInteger(value.value, "alter_id", 0, 65535)
	if !ok {
		return loonNode{}, DiagnosticInvalidRequiredField
	}
	tlsValue, code := parseTLS(value.value, false)
	if code != "" {
		return loonNode{}, code
	}
	parts := []string{
		"vmess", server, strconv.FormatInt(port, 10), security, loonQuoted(uuid),
		"transport=tcp", "alterId=" + strconv.FormatInt(alterID, 10),
		loonOptionBool("over-tls", tlsValue.present && tlsValue.enabled),
	}
	tlsParts, code := loonTLSOptions(tlsValue, true)
	if code != "" {
		return loonNode{}, code
	}
	parts = append(parts, tlsParts...)
	parts = append(parts, loonOptionBool("udp", udp))
	return loonNode{name: value.tag, parts: parts}, ""
}

func loonVLESS(value outbound) (loonNode, DiagnosticCode) {
	server, port, udp, code := commonServer(value, "flow", "network", "tls", "uuid")
	if code != "" {
		return loonNode{}, code
	}
	uuid, ok := requiredString(value.value, "uuid")
	if !ok || !validLoonAtom(server) {
		return loonNode{}, DiagnosticInvalidRequiredField
	}
	flow, ok := optionalString(value.value, "flow")
	if !ok || (flow != "" && flow != "xtls-rprx-vision") {
		return loonNode{}, DiagnosticUnsupportedOption
	}
	tlsValue, code := parseTLS(value.value, false)
	if code != "" {
		return loonNode{}, code
	}
	parts := []string{
		"VLESS", server, strconv.FormatInt(port, 10), loonQuoted(uuid), "transport=tcp",
	}
	if flow != "" {
		parts = append(parts, "flow="+flow)
	}
	parts = append(parts, loonOptionBool("over-tls", tlsValue.present && tlsValue.enabled))
	tlsParts, code := loonTLSOptions(tlsValue, true)
	if code != "" {
		return loonNode{}, code
	}
	parts = append(parts, tlsParts...)
	parts = append(parts, loonOptionBool("udp", udp))
	return loonNode{name: value.tag, parts: parts}, ""
}

func loonTrojan(value outbound) (loonNode, DiagnosticCode) {
	server, port, udp, code := commonServer(value, "network", "password", "tls")
	if code != "" {
		return loonNode{}, code
	}
	password, ok := requiredString(value.value, "password")
	if !ok || !validLoonAtom(server) {
		return loonNode{}, DiagnosticInvalidRequiredField
	}
	tlsValue, code := parseTLS(value.value, true)
	if code != "" {
		return loonNode{}, code
	}
	parts := []string{"trojan", server, strconv.FormatInt(port, 10), loonQuoted(password)}
	tlsParts, code := loonTLSOptions(tlsValue, true)
	if code != "" {
		return loonNode{}, code
	}
	parts = append(parts, tlsParts...)
	parts = append(parts, loonOptionBool("udp", udp))
	return loonNode{name: value.tag, parts: parts}, ""
}

func loonHysteria2(value outbound) (loonNode, DiagnosticCode) {
	server, port, udp, code := commonServer(value, "network", "obfs", "password", "tls")
	if code != "" {
		return loonNode{}, code
	}
	password, ok := requiredString(value.value, "password")
	if !ok || !validLoonAtom(server) {
		return loonNode{}, DiagnosticInvalidRequiredField
	}
	tlsValue, code := parseTLS(value.value, true)
	if code != "" {
		return loonNode{}, code
	}
	parts := []string{"Hysteria2", server, strconv.FormatInt(port, 10), loonQuoted(password)}
	if raw, exists := value.value["obfs"]; exists {
		object, ok := raw.(map[string]any)
		if !ok || unsupportedFields(object, map[string]struct{}{"password": {}, "type": {}}) != "" {
			return loonNode{}, DiagnosticUnsupportedOption
		}
		typeID, typeOK := requiredString(object, "type")
		obfsPassword, passwordOK := requiredString(object, "password")
		if !typeOK || !passwordOK || typeID != "salamander" {
			return loonNode{}, DiagnosticUnsupportedOption
		}
		parts = append(parts, "salamander-password="+loonQuoted(obfsPassword))
	}
	tlsParts, code := loonTLSOptions(tlsValue, true)
	if code != "" {
		return loonNode{}, code
	}
	parts = append(parts, tlsParts...)
	parts = append(parts, loonOptionBool("udp", udp))
	return loonNode{name: value.tag, parts: parts}, ""
}

func loonAnyTLS(value outbound) (loonNode, DiagnosticCode) {
	server, port, udp, code := commonServer(value, "network", "password", "tls")
	if code != "" {
		return loonNode{}, code
	}
	password, ok := requiredString(value.value, "password")
	if !ok || !validLoonAtom(server) {
		return loonNode{}, DiagnosticInvalidRequiredField
	}
	tlsValue, code := parseTLS(value.value, true)
	if code != "" {
		return loonNode{}, code
	}
	parts := []string{"AnyTLS", server, strconv.FormatInt(port, 10), loonQuoted(password)}
	tlsParts, code := loonTLSOptions(tlsValue, true)
	if code != "" {
		return loonNode{}, code
	}
	parts = append(parts, tlsParts...)
	parts = append(parts, loonOptionBool("udp", udp))
	return loonNode{name: value.tag, parts: parts}, ""
}

func loonTLSOptions(value tlsOptions, includeServerName bool) ([]string, DiagnosticCode) {
	if !value.present {
		return nil, ""
	}
	if !value.enabled {
		return nil, ""
	}
	if len(value.alpn) > 1 {
		return nil, DiagnosticUnsupportedTLS
	}
	parts := make([]string, 0, 3)
	if includeServerName && value.serverName != "" {
		parts = append(parts, "sni="+loonOptionValue(value.serverName))
	}
	if len(value.alpn) == 1 {
		parts = append(parts, "alpn="+loonOptionValue(value.alpn[0]))
	}
	parts = append(parts, loonOptionBool("skip-cert-verify", value.insecure))
	return parts, ""
}

var loonShadowsocksMethods = map[string]struct{}{
	"2022-blake3-aes-128-gcm":       {},
	"2022-blake3-aes-256-gcm":       {},
	"2022-blake3-chacha20-poly1305": {},
	"aes-128-gcm":                   {},
	"aes-256-gcm":                   {},
	"chacha20-ietf-poly1305":        {},
}

func marshalLoon(nodes []loonNode) []byte {
	var output bytes.Buffer
	output.WriteString("[Proxy]\n")
	for _, node := range nodes {
		output.WriteString(node.name)
		output.WriteString(" = ")
		output.WriteString(strings.Join(node.parts, ","))
		output.WriteByte('\n')
	}
	return output.Bytes()
}

func validLoonName(value string) bool {
	if strings.TrimSpace(value) != value || strings.ContainsAny(value, "\r\n=") {
		return false
	}
	return !strings.HasPrefix(value, "#") && !strings.HasPrefix(value, ";") && !strings.HasPrefix(value, "[")
}

func validLoonAtom(value string) bool {
	return value != "" && strings.TrimSpace(value) == value && !strings.ContainsAny(value, ",\r\n\"")
}

func loonQuoted(value string) string {
	encoded, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return string(encoded)
}

func loonOptionBool(name string, value bool) string {
	return name + "=" + strconv.FormatBool(value)
}

func loonOptionValue(value string) string {
	if validLoonAtom(value) {
		return value
	}
	return loonQuoted(value)
}
