// SPDX-License-Identifier: GPL-3.0-or-later

package subscription

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
)

func TestMihomoTransportTLSAndMuxRoundTrip(t *testing.T) {
	raw := []byte(`{"outbounds":[{"type":"vless","tag":"node","server":"example.test","server_port":443,"uuid":"d8432f7e-311f-4b38-82d0-d31cc043f871","transport":{"type":"grpc","service_name":"proxy"},"tls":{"enabled":true,"server_name":"example.test","utls":{"enabled":true,"fingerprint":"chrome"},"reality":{"enabled":true,"public_key":"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA","short_id":"abcd"}},"multiplex":{"enabled":true,"protocol":"h2mux","max_connections":4,"padding":true,"brutal":{"enabled":true,"up_mbps":50,"down_mbps":100}}}]}`)
	result, err := Render(raw, RenderChannel{Format: RenderFormatMihomo})
	if err != nil || result.NodeCount != 1 || len(result.Diagnostics) != 0 {
		t.Fatalf("render failed: %v, %v", err, result.Diagnostics)
	}
	var decoded map[string]any
	if err := yaml.Unmarshal(result.Content, &decoded); err != nil {
		t.Fatal(err)
	}
	proxy := decoded["proxies"].([]any)[0].(map[string]any)
	if proxy["client-fingerprint"] != "chrome" || proxy["grpc-opts"].(map[string]any)["grpc-service-name"] != "proxy" || proxy["smux"].(map[string]any)["max-connections"] != 4 {
		t.Fatalf("mapped options missing: %v", proxy)
	}
	nodes, _, err := ParseSource(SourceFormatMihomoYAML, result.Content, "roundtrip")
	if err != nil || len(nodes) != 1 {
		t.Fatalf("parse: %v", err)
	}
	var native map[string]any
	if err := json.Unmarshal(nodes[0].Outbound, &native); err != nil {
		t.Fatal(err)
	}
	if native["transport"].(map[string]any)["service_name"] != "proxy" || native["tls"].(map[string]any)["reality"].(map[string]any)["short_id"] != "abcd" || native["multiplex"].(map[string]any)["brutal"].(map[string]any)["down_mbps"] != float64(100) {
		t.Fatalf("lost native options: %v", native)
	}
	// Native rendering after the conversion cannot lose options through map mutation.
	nativeRender, err := Render(raw, RenderChannel{Format: RenderFormatSingBox})
	if err != nil || !bytes.Contains(nativeRender.Content, []byte(`"public_key"`)) {
		t.Fatal("native rendering changed")
	}
}

func TestMihomoUnsupportedOrMalformedOptionsDoNotDisappear(t *testing.T) {
	for _, options := range []string{
		`"transport":{"type":"ws","headers":{"Host":123}}`,
		`"transport":{"type":"grpc","service_name":"test","idle_timeout":"5s"}`,
		`"multiplex":{"enabled":"true"}`,
		`"multiplex":{"enabled":true,"max_connections":4,"max_streams":4}`,
		`"tls":{"enabled":true,"utls":{"enabled":true,"future":true}}`,
		`"tls":{"enabled":true,"reality":{"enabled":true,"public_key":"bad"}}`,
	} {
		raw := `{"outbounds":[{"type":"vless","tag":"node","server":"example.test","server_port":443,"uuid":"uuid",` + options + `}]}`
		result, err := Render([]byte(raw), RenderChannel{Format: RenderFormatMihomo})
		if err != nil || result.NodeCount != 0 || len(result.Diagnostics) != 1 {
			t.Fatalf("options accepted: %s, %v", options, err)
		}
	}
	for _, options := range []string{
		"network: ws\n    ws-opts: {path: /proxy, headers: {Host: example.test}}\n    plugin: obsolete",
		"network: grpc\n    grpc-opts: {grpc-service-name: proxy, grpc-user-agent: unsupported}",
		"tls: true\n    client-fingerprint: 123",
		"smux: {enabled: true, only-tcp: true}",
		"ws-opts: {path: /forgot-network}",
	} {
		raw := "proxies:\n  - {name: first, type: socks5, server: safe.test, port: 1080}\n  - name: broken\n    type: vless\n    server: example.test\n    port: 443\n    uuid: uuid\n    " + options + "\n"
		if nodes, _, err := ParseSource(SourceFormatMihomoYAML, []byte(raw), "reject"); err == nil || len(nodes) != 0 {
			t.Fatal("partially converted invalid source")
		}
	}
}

func TestMihomoWebSocketHeadersAndEarlyData(t *testing.T) {
	raw := []byte(`{"outbounds":[{"type":"vmess","tag":"ws","server":"example.test","server_port":443,"uuid":"uuid","transport":{"type":"ws","path":"/ws","headers":{"Host":"cdn.example"},"max_early_data":2048,"early_data_header_name":"Sec-WebSocket-Protocol"}}]}`)
	result, err := Render(raw, RenderChannel{Format: RenderFormatMihomo})
	if err != nil || result.NodeCount != 1 {
		t.Fatalf("render: %v", err)
	}
	nodes, _, err := ParseSource(SourceFormatMihomoYAML, result.Content, "ws")
	if err != nil || len(nodes) != 1 || !strings.Contains(string(nodes[0].Outbound), `"max_early_data":2048`) || !strings.Contains(string(nodes[0].Outbound), `"Host":"cdn.example"`) {
		t.Fatalf("options lost: %v", err)
	}
}
