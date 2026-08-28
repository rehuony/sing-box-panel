// SPDX-License-Identifier: GPL-3.0-or-later

package singbox11319

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/rehuony/sing-box-panel/internal/subscription/node"
)

func convertInboundCredential(
	typeID string,
	tag string,
	publicHost string,
	port int64,
	inboundValue map[string]any,
	credential inboundCredential,
	credentialIndex int,
	credentialCount int,
) (node.Node, error) {
	outboundType := typeID
	variant := ""
	if typeID == "mixed" {
		outboundType = "socks"
		variant = "socks"
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
	if tlsValue, ok := sanitizedClientTLS(inboundValue["tls"], publicHost); ok {
		outbound["tls"] = tlsValue
	}
	if transport, ok := sanitizedClientTransport(inboundValue["transport"]); ok {
		outbound["transport"] = transport
	}
	if err := validateConvertedCredential(typeID, outbound); err != nil {
		return node.Node{}, err
	}
	encoded, err := json.Marshal(outbound)
	if err != nil {
		return node.Node{}, err
	}
	return node.Node{
		Key: "local:" + identityHex[:24], SourceID: "local", Type: outboundType,
		Tag: nodeTag, Credential: credential.label, Outbound: encoded,
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
