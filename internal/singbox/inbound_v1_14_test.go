// SPDX-License-Identifier: GPL-3.0-or-later

package singbox

import (
	"bytes"
	"crypto/ecdh"
	"encoding/base64"
	"encoding/json"
	"testing"

	"github.com/rehuony/sing-box-panel/internal/subscription"
)

func TestInbound114NativeClientConversion(t *testing.T) {
	for _, exactVersion := range []string{"1.14.0", "1.14.1"} {
		t.Run(exactVersion, func(t *testing.T) {
			for _, test := range []struct {
				name, inbound string
				check         func(*testing.T, map[string]any)
			}{
				{"mixed empty users", `{"type":"mixed","users":[]}`, func(t *testing.T, out map[string]any) {
					if out["type"] != "socks" {
						t.Fatal(out)
					}
				}},
				{"snell 5 client 4", `{"type":"snell","version":5,"psk":"test-server-key","obfs_mode":"http","users":[{"name":"alice","userkey":"test-user-key"}]}`, func(t *testing.T, out map[string]any) {
					if out["version"] != json.Number("4") || out["psk"] != "test-server-key" || out["userkey"] != "test-user-key" || out["obfs_mode"] != "http" {
						t.Fatal(out)
					}
				}},
				{"snell 6", `{"type":"snell","version":6,"psk":"test-server-key","mode":"unshaped"}`, func(t *testing.T, out map[string]any) {
					if out["version"] != json.Number("6") || out["mode"] != "unshaped" {
						t.Fatal(out)
					}
				}},
				{"shadowsocks 2022", `{"type":"shadowsocks","method":"2022-blake3-aes-128-gcm","password":"server-key","users":[{"name":"alice","password":"user-key"}]}`, func(t *testing.T, out map[string]any) {
					if out["password"] != "server-key:user-key" {
						t.Fatal(out)
					}
				}},
				{"explicit SNI", `{"type":"anytls","users":[{"password":"test-password"}],"tls":{"enabled":true,"server_name":"tls.example.com","alpn":"h2","key_path":"/never/publish/private","certificate_path":"/never/publish/certificate"}}`, func(t *testing.T, out map[string]any) {
					tls := out["tls"].(map[string]any)
					if tls["server_name"] != "tls.example.com" || tls["alpn"] != "h2" || tls["insecure"] != false {
						t.Fatal(tls)
					}
				}},
			} {
				t.Run(test.name, func(t *testing.T) {
					inbound, err := subscription.DecodeDocumentObject([]byte(test.inbound))
					if err != nil {
						t.Fatal(err)
					}
					inbound["tag"], inbound["listen"], inbound["listen_port"] = "real-name", "127.0.0.1", 443
					input, _ := json.Marshal(map[string]any{"inbounds": []any{inbound}})
					result, err := NewInboundRegistry().Convert(exactVersion, subscription.InboundRequest{FinalStartupJSON: input, PublicHost: "203.0.113.7"})
					if err != nil || len(result.Nodes) != 1 || len(result.Diagnostics) != 0 {
						t.Fatalf("conversion: %+v %v", result, err)
					}
					node := result.Nodes[0]
					if node.OriginTag != "real-name" || bytes.Contains(node.Outbound, []byte("/never/publish")) || bytes.Contains(node.Outbound, []byte("127.0.0.1")) {
						t.Fatal("incorrect provenance or leaked listener/server fields")
					}
					out, err := subscription.DecodeDocumentObject(node.Outbound)
					if err != nil {
						t.Fatal(err)
					}
					test.check(t, out)
					document, _ := json.Marshal(map[string]any{"outbounds": []json.RawMessage{node.Outbound}})
					if err := ValidateConfiguration(exactVersion, document); err != nil {
						t.Fatal(err)
					}
				})
			}
		})
	}
}

func TestRealityPublishesDerivedPublicKeyAndKeepsSNI(t *testing.T) {
	privateKey, err := ecdh.X25519().NewPrivateKey(bytes.Repeat([]byte{7}, 32))
	if err != nil {
		t.Fatal(err)
	}
	encoded := base64.RawURLEncoding.EncodeToString(privateKey.Bytes())
	tls := map[string]any{"enabled": true, "reality": map[string]any{
		"enabled": true, "private_key": encoded, "short_id": []any{"0123456789abcdef"},
		"handshake": map[string]any{"server": "handshake.example.com", "server_port": 443, "detour": "server-only"},
	}}
	client, err := sanitizedClientTLS(tls, "node.example.com")
	if err != nil {
		t.Fatal(err)
	}
	reality := client["reality"].(map[string]any)
	if reality["public_key"] != base64.RawURLEncoding.EncodeToString(privateKey.PublicKey().Bytes()) || reality["short_id"] != "0123456789abcdef" || client["server_name"] != "handshake.example.com" {
		t.Fatal(client)
	}
	encodedClient, _ := json.Marshal(client)
	for _, secret := range []string{encoded, "private_key", "server-only", "handshake\""} {
		if bytes.Contains(encodedClient, []byte(secret)) {
			t.Fatal("Reality server settings leaked")
		}
	}
	tls["server_name"] = "explicit.example.com"
	client, err = sanitizedClientTLS(tls, "node.example.com")
	if err != nil || client["server_name"] != "explicit.example.com" {
		t.Fatal(client, err)
	}
	tls["reality"].(map[string]any)["private_key"] = "invalid"
	if _, err := sanitizedClientTLS(tls, "node.example.com"); err == nil {
		t.Fatal("invalid server key accepted")
	}
}
