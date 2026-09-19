// SPDX-License-Identifier: GPL-3.0-or-later

package singbox

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/rehuony/sing-box-panel/internal/subscription"
)

type inboundOptions struct {
	anyTLS bool
	naive  bool
	snell  bool
}

type converter struct {
	exactVersion string
	convertible  map[string]struct{}
}

func newInboundConverter(exactVersion string, options inboundOptions) subscription.InboundConverter {
	convertible := map[string]struct{}{
		"mixed": {}, "socks": {}, "http": {}, "shadowsocks": {}, "vmess": {},
		"trojan": {}, "hysteria": {}, "shadowtls": {}, "vless": {}, "tuic": {},
		"hysteria2": {},
	}
	if options.anyTLS {
		convertible["anytls"] = struct{}{}
	}
	if options.naive {
		convertible["naive"] = struct{}{}
	}
	if options.snell {
		convertible["snell"] = struct{}{}
	}
	return converter{exactVersion: exactVersion, convertible: convertible}
}

func (value converter) ExactVersion() string { return value.exactVersion }

var unpublishableInboundTypes = map[string]struct{}{
	"direct": {}, "tun": {}, "redirect": {}, "tproxy": {}, "cloudflared": {},
}

func (value converter) Convert(request subscription.InboundRequest) (subscription.InboundResult, error) {
	if _, err := normalizedPublicHost(request.PublicHost); err != nil {
		return subscription.InboundResult{}, err
	}
	decoded, err := subscription.DecodeDocument(request.FinalStartupJSON)
	if err != nil {
		return subscription.InboundResult{}, err
	}
	root, ok := decoded.(map[string]any)
	if !ok {
		return subscription.InboundResult{}, subscription.InvalidDocument("root_not_object")
	}
	rawInbounds, exists := root[string(subscription.CollectionInbounds)]
	if !exists {
		return subscription.InboundResult{Nodes: []subscription.Node{}, Diagnostics: []subscription.ConversionDiagnostic{}}, nil
	}
	inbounds, ok := rawInbounds.([]any)
	if !ok {
		return subscription.InboundResult{}, subscription.InvalidDocument("inbounds_not_array")
	}
	if len(inbounds) > subscription.MaximumNodes {
		return subscription.InboundResult{}, subscription.InvalidDocument("too_many_inbounds")
	}

	nodes := make([]subscription.Node, 0, len(inbounds))
	diagnostics := make([]subscription.ConversionDiagnostic, 0)
	seenKeys := make(map[string]struct{})
	seenTags := make(map[string]struct{})
	for index, rawInbound := range inbounds {
		inboundValue, ok := rawInbound.(map[string]any)
		if !ok {
			diagnostics = append(diagnostics, inboundDiagnostic(index, subscription.DiagnosticInvalidOutbound))
			continue
		}
		typeID, typeOK := inboundValue["type"].(string)
		tag, tagOK := inboundValue["tag"].(string)
		if !typeOK || !tagOK || !subscription.ValidType(typeID) || !subscription.ValidTag(tag) {
			diagnostics = append(diagnostics, inboundDiagnostic(index, subscription.DiagnosticInvalidMetadata))
			continue
		}
		if _, known := unpublishableInboundTypes[typeID]; known {
			diagnostics = append(diagnostics, inboundDiagnostic(index, subscription.DiagnosticUnpublishableInbound))
			continue
		}
		if _, known := value.convertible[typeID]; !known {
			diagnostics = append(diagnostics, inboundDiagnostic(index, subscription.DiagnosticUnsupportedType))
			continue
		}
		port, ok := subscription.DocumentInteger(inboundValue["listen_port"], 1, 65535)
		if !ok {
			diagnostics = append(diagnostics, inboundDiagnostic(index, subscription.DiagnosticInvalidRequiredField))
			continue
		}
		credentials, credentialErr := inboundCredentials(typeID, inboundValue)
		if credentialErr != nil {
			return subscription.InboundResult{Diagnostics: append(diagnostics, inboundDiagnostic(index, subscription.DiagnosticAmbiguousCredential))}, credentialErr
		}
		for credentialIndex, credential := range credentials {
			converted, convertErr := convertInboundCredential(
				typeID, tag, request.PublicHost, port, inboundValue, credential, credentialIndex, len(credentials),
			)
			if convertErr != nil {
				diagnostics = append(diagnostics, inboundDiagnostic(index, subscription.DiagnosticInvalidRequiredField))
				continue
			}
			if _, duplicate := seenKeys[converted.Key]; duplicate {
				return subscription.InboundResult{Diagnostics: append(diagnostics, inboundDiagnostic(index, subscription.DiagnosticAmbiguousCredential))}, subscription.ErrAmbiguousInboundCredential
			}
			if _, duplicate := seenTags[converted.Tag]; duplicate {
				return subscription.InboundResult{Diagnostics: append(diagnostics, inboundDiagnostic(index, subscription.DiagnosticAmbiguousCredential))}, subscription.ErrAmbiguousInboundCredential
			}
			seenKeys[converted.Key] = struct{}{}
			seenTags[converted.Tag] = struct{}{}
			nodes = append(nodes, converted)
		}
	}
	return subscription.InboundResult{Nodes: nodes, Diagnostics: diagnostics}, nil
}

func inboundDiagnostic(index int, code subscription.DiagnosticCode) subscription.ConversionDiagnostic {
	return subscription.ConversionDiagnostic{Collection: subscription.CollectionInbounds, ItemIndex: index, Code: code}
}

type inboundCredential struct {
	identity string
	label    string
	value    map[string]any
}

func inboundCredentials(typeID string, inboundValue map[string]any) ([]inboundCredential, error) {
	rawUsers, hasUsers := inboundValue["users"]
	if !hasUsers {
		return []inboundCredential{{identity: "default", label: "default", value: inboundValue}}, nil
	}
	users, ok := rawUsers.([]any)
	if !ok || len(users) > subscription.MaximumNodes {
		return nil, subscription.ErrAmbiguousInboundCredential
	}
	if len(users) == 0 {
		return []inboundCredential{{identity: "default", label: "default", value: inboundValue}}, nil
	}
	result := make([]inboundCredential, 0, len(users))
	seen := make(map[string]struct{}, len(users))
	for index, rawUser := range users {
		user, ok := rawUser.(map[string]any)
		if !ok {
			return nil, subscription.ErrAmbiguousInboundCredential
		}
		identity := firstNonEmptyString(user, "name", "username", "uuid")
		label := identity
		if identity == "" && len(users) == 1 {
			identity, label = "default", "default"
		}
		if identity == "" {
			return nil, subscription.ErrAmbiguousInboundCredential
		}
		identityKey := strings.ToLower(typeID + "\x00" + identity)
		if _, duplicate := seen[identityKey]; duplicate {
			return nil, subscription.ErrAmbiguousInboundCredential
		}
		seen[identityKey] = struct{}{}
		if label == "" {
			label = fmt.Sprintf("user-%d", index+1)
		}
		result = append(result, inboundCredential{identity: identity, label: label, value: user})
	}
	return result, nil
}

func convertInboundCredential(
	typeID string,
	tag string,
	publicHost string,
	port int64,
	inboundValue map[string]any,
	credential inboundCredential,
	_ int,
	credentialCount int,
) (subscription.Node, error) {
	outboundType := typeID
	variant := ""
	if typeID == "mixed" {
		outboundType, variant = "socks", "socks"
	}
	identityDigest := sha256.Sum256([]byte(typeID + "\x00" + tag + "\x00" + credential.identity + "\x00" + variant))
	identityHex := hex.EncodeToString(identityDigest[:])
	nodeTag := tag
	if credentialCount > 1 || variant != "" {
		nodeTag += "-" + identityHex[:10]
	}
	outbound := map[string]any{
		"type": outboundType, "tag": nodeTag, "server": publicHost, "server_port": json.Number(fmt.Sprint(port)),
	}
	copyInboundFields(outbound, inboundValue, inboundFieldNames(typeID)...)
	copyInboundFields(outbound, credential.value, credentialFieldNames(typeID)...)
	tlsValue, err := sanitizedClientTLS(inboundValue["tls"], publicHost)
	if err != nil {
		return subscription.Node{}, err
	}
	if tlsValue != nil {
		outbound["tls"] = tlsValue
	}
	if typeID == "snell" {
		version, ok := subscription.DocumentInteger(inboundValue["version"], 5, 6)
		if !ok {
			return subscription.Node{}, errors.New("invalid Snell server version")
		}
		if version == 5 {
			version = 4
		}
		outbound["version"] = json.Number(fmt.Sprint(version))
	}
	if typeID == "shadowsocks" && strings.HasPrefix(firstNonEmptyString(inboundValue, "method"), "2022-") && lenValueArray(inboundValue["users"]) > 0 {
		serverKey := firstNonEmptyString(inboundValue, "password")
		userKey := firstNonEmptyString(credential.value, "password")
		if serverKey == "" || userKey == "" {
			return subscription.Node{}, errors.New("missing Shadowsocks server or user key")
		}
		outbound["password"] = serverKey + ":" + userKey
	}
	if transport, ok := sanitizedClientTransport(inboundValue["transport"]); ok {
		outbound["transport"] = transport
	}
	if err := validateConvertedCredential(typeID, outbound); err != nil {
		return subscription.Node{}, err
	}
	encoded, err := json.Marshal(outbound)
	if err != nil {
		return subscription.Node{}, err
	}
	return subscription.Node{
		Key: "local:" + identityHex[:24], SourceID: "local", Type: outboundType,
		OriginTag: tag, Tag: nodeTag, Credential: credential.label, Outbound: encoded,
	}, nil
}

func inboundFieldNames(typeID string) []string {
	switch typeID {
	case "socks", "mixed":
		return []string{"version", "network"}
	case "shadowsocks":
		return []string{"method", "password", "network"}
	case "vmess":
		return []string{"uuid", "security", "alter_id", "global_padding", "authenticated_length", "packet_encoding"}
	case "trojan":
		return []string{"password", "network"}
	case "naive":
		return []string{"username", "password"}
	case "hysteria":
		return []string{"auth", "auth_str", "up_mbps", "down_mbps", "obfs", "network"}
	case "shadowtls":
		return []string{"version", "password"}
	case "vless":
		return []string{"uuid", "flow", "packet_encoding", "network"}
	case "tuic":
		return []string{"uuid", "password", "congestion_control", "udp_relay_mode", "zero_rtt_handshake", "heartbeat"}
	case "hysteria2":
		return []string{"password", "up_mbps", "down_mbps", "obfs", "network"}
	case "snell":
		return []string{"psk", "obfs_mode", "mode"}
	case "anytls":
		return []string{"password"}
	case "http":
		return []string{"username", "password"}
	default:
		return nil
	}
}

func credentialFieldNames(typeID string) []string {
	switch typeID {
	case "mixed", "socks", "http", "naive":
		return []string{"username", "password"}
	case "shadowsocks":
		return []string{"password"}
	case "vmess":
		return []string{"uuid", "security", "alter_id"}
	case "trojan", "shadowtls", "hysteria2", "anytls":
		return []string{"password"}
	case "hysteria":
		return []string{"auth", "auth_str"}
	case "vless":
		return []string{"uuid", "flow"}
	case "tuic":
		return []string{"uuid", "password"}
	case "snell":
		return []string{"userkey"}
	default:
		return nil
	}
}

func copyInboundFields(target map[string]any, source map[string]any, names ...string) {
	for _, name := range names {
		if value, exists := source[name]; exists {
			target[name] = value
		}
	}
}

func validateConvertedCredential(typeID string, outbound map[string]any) error {
	required := [][]string{}
	switch typeID {
	case "shadowsocks", "trojan", "shadowtls", "hysteria2", "anytls":
		required = [][]string{{"password"}}
	case "snell":
		required = [][]string{{"psk"}}
	case "vmess", "vless":
		required = [][]string{{"uuid"}}
	case "naive":
		required = [][]string{{"username"}, {"password"}}
	case "hysteria":
		required = [][]string{{"auth", "auth_str"}}
	case "tuic":
		required = [][]string{{"uuid"}, {"password"}}
	}
	for _, alternatives := range required {
		present := false
		for _, key := range alternatives {
			if value, ok := outbound[key].(string); ok && value != "" {
				present = true
			}
		}
		if !present {
			return errors.New("missing required inbound credential field")
		}
	}
	return nil
}

func firstNonEmptyString(value map[string]any, names ...string) string {
	for _, name := range names {
		if text, ok := value[name].(string); ok && text != "" {
			return text
		}
	}
	return ""
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
		return "", subscription.ErrInvalidPublicHost
	}
	return value, nil
}

func lenValueArray(value any) int {
	items, _ := value.([]any)
	return len(items)
}
