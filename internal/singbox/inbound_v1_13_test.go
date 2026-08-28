// SPDX-License-Identifier: GPL-3.0-or-later

package singbox

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/rehuony/sing-box-panel/internal/subscription"
)

func convert(t *testing.T, document []byte, publicHost string) (subscription.InboundResult, error) {
	t.Helper()
	return NewInboundRegistry().Convert(
		"1.13.19",
		subscription.InboundRequest{FinalStartupJSON: document, PublicHost: publicHost},
	)
}

func TestInbound11319ContractRegistry(t *testing.T) {
	t.Parallel()
	tests := []struct {
		typeID string
		extra  string
	}{
		{typeID: "mixed"},
		{typeID: "socks", extra: `,"users":[{"username":"user","password":"pass"}]`},
		{typeID: "http", extra: `,"users":[{"username":"user","password":"pass"}]`},
		{typeID: "shadowsocks", extra: `,"method":"aes-128-gcm","users":[{"name":"one","password":"pass"}]`},
		{typeID: "vmess", extra: `,"users":[{"name":"one","uuid":"uuid"}]`},
		{typeID: "trojan", extra: `,"users":[{"name":"one","password":"pass"}],"tls":{"enabled":true,"certificate_path":"/server/cert","key_path":"/server/private-key","acme":{"domain":["secret.example"]}}`},
		{typeID: "naive", extra: `,"users":[{"username":"user","password":"pass"}]`},
		{typeID: "hysteria", extra: `,"users":[{"name":"one","auth_str":"pass"}],"tls":{"enabled":true}`},
		{typeID: "shadowtls", extra: `,"users":[{"name":"one","password":"pass"}]`},
		{typeID: "vless", extra: `,"users":[{"name":"one","uuid":"uuid"}]`},
		{typeID: "tuic", extra: `,"users":[{"name":"one","uuid":"uuid","password":"pass"}],"tls":{"enabled":true}`},
		{typeID: "hysteria2", extra: `,"users":[{"name":"one","password":"pass"}],"tls":{"enabled":true}`},
		{typeID: "anytls", extra: `,"users":[{"name":"one","password":"pass"}],"tls":{"enabled":true}`},
	}
	for _, test := range tests {
		t.Run(test.typeID, func(t *testing.T) {
			document := []byte(fmt.Sprintf(`{"inbounds":[{"type":%q,"tag":%q,"listen":"::","listen_port":443%s}]}`,
				test.typeID, test.typeID, test.extra))
			result, err := convert(t, document, "public.example")
			if err != nil || len(result.Nodes) != 1 || len(result.Diagnostics) != 0 {
				t.Fatalf("nodes=%+v diagnostics=%+v err=%v", result.Nodes, result.Diagnostics, err)
			}
			if result.Nodes[0].Key == "" || result.Nodes[0].SourceID != "local" ||
				!bytes.Contains(result.Nodes[0].Outbound, []byte(`"server":"public.example"`)) {
				t.Fatalf("node=%+v", result.Nodes[0])
			}
			if bytes.Contains(result.Nodes[0].Outbound, []byte("private-key")) ||
				bytes.Contains(result.Nodes[0].Outbound, []byte("certificate_path")) ||
				bytes.Contains(result.Nodes[0].Outbound, []byte("acme")) {
				t.Fatalf("server TLS material leaked: %s", result.Nodes[0].Outbound)
			}
			if _, err := subscription.RenderNodes(result.Nodes, subscription.RenderChannel{Format: subscription.RenderFormatSingBox}); err != nil {
				t.Fatalf("RenderNodes: %v", err)
			}
		})
	}
}

func TestInbound11319ServerOnlyDiagnosticsAndVersionFailClosed(t *testing.T) {
	t.Parallel()
	types := []string{"direct", "tun", "redirect", "tproxy", "cloudflared"}
	inbounds := make([]map[string]any, 0, len(types))
	for index, typeID := range types {
		inbounds = append(inbounds, map[string]any{
			"type": typeID, "tag": typeID, "listen_port": 10000 + index,
		})
	}
	document, err := json.Marshal(map[string]any{"inbounds": inbounds})
	if err != nil {
		t.Fatal(err)
	}
	result, err := convert(t, document, "public.example")
	if err != nil || len(result.Nodes) != 0 || len(result.Diagnostics) != len(types) {
		t.Fatalf("nodes=%+v diagnostics=%+v err=%v", result.Nodes, result.Diagnostics, err)
	}
	for index, diagnostic := range result.Diagnostics {
		if diagnostic.Collection != subscription.CollectionInbounds || diagnostic.ItemIndex != index ||
			diagnostic.Code != subscription.DiagnosticUnpublishableInbound {
			t.Fatalf("diagnostic[%d]=%+v", index, diagnostic)
		}
	}
}

func TestInbound11319RejectsTypesAbsentFromTheExactUpstreamRegistry(t *testing.T) {
	t.Parallel()

	result, err := convert(t, []byte(`{"inbounds":[{"type":"snell","tag":"snell","listen_port":443,"psk":"pass"}]}`), "public.example")
	if err != nil || len(result.Nodes) != 0 || len(result.Diagnostics) != 1 ||
		result.Diagnostics[0].Code != subscription.DiagnosticUnsupportedType {
		t.Fatalf("nodes=%+v diagnostics=%+v err=%v", result.Nodes, result.Diagnostics, err)
	}
}

func TestInbound11319MultiUserCredentialsHaveSeparateStableGrantKeys(t *testing.T) {
	t.Parallel()
	document := []byte(`{"inbounds":[{"type":"vmess","tag":"multi","listen_port":443,"users":[{"name":"alpha","uuid":"uuid-a"},{"name":"beta","uuid":"uuid-b"}]}]}`)
	first, err := convert(t, document, "public.example")
	if err != nil || len(first.Nodes) != 2 || len(first.Diagnostics) != 0 {
		t.Fatalf("nodes=%+v diagnostics=%+v err=%v", first.Nodes, first.Diagnostics, err)
	}
	second, err := convert(t, document, "public.example")
	if err != nil || first.Nodes[0].Key != second.Nodes[0].Key || first.Nodes[1].Key != second.Nodes[1].Key || first.Nodes[0].Key == first.Nodes[1].Key {
		t.Fatalf("unstable node identities: first=%+v second=%+v err=%v", first.Nodes, second.Nodes, err)
	}
	selected, err := subscription.RenderNodes(first.Nodes[:1], subscription.RenderChannel{Format: subscription.RenderFormatSingBox})
	if err != nil || selected.NodeCount != 1 || bytes.Contains(selected.Content, []byte("uuid-b")) {
		t.Fatalf("selected result=%+v content=%s err=%v", selected, selected.Content, err)
	}

	ambiguous := []byte(`{"inbounds":[{"type":"trojan","tag":"ambiguous","listen_port":443,"users":[{"password":"one"},{"password":"two"}]}]}`)
	result, err := convert(t, ambiguous, "public.example")
	if !errors.Is(err, subscription.ErrAmbiguousInboundCredential) || len(result.Diagnostics) != 1 || result.Diagnostics[0].Code != subscription.DiagnosticAmbiguousCredential {
		t.Fatalf("ambiguous diagnostics=%+v err=%v", result.Diagnostics, err)
	}
}
