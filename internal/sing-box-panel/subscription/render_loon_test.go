// SPDX-License-Identifier: GPL-3.0-or-later

package subscription

import (
	"reflect"
	"strings"
	"testing"
)

func TestLoonALPNPreservesValues(t *testing.T) {
	for _, test := range []struct {
		name, tls, want, reason string
	}{
		{"absent", `"enabled":true`, "", ""},
		{"empty", `"enabled":true,"alpn":[]`, "", ""},
		{"scalar", `"enabled":true,"alpn":"h2"`, "alpn=h2", ""},
		{"ordered", `"enabled":true,"alpn":["http/1.1","h2"]`, `alpn="http/1.1,h2"`, ""},
		{"escaped", `"enabled":true,"alpn":["quoted\"protocol","h2"]`, `alpn="quoted\"protocol,h2"`, ""},
		{"embedded comma", `"enabled":true,"alpn":["h2,http/1.1"]`, "", "unrepresentable_alpn"},
		{"newline", `"enabled":true,"alpn":["h2\n"]`, "", "unrepresentable_alpn"},
		{"empty protocol", `"enabled":true,"alpn":[""]`, "", "invalid_field"},
	} {
		t.Run(test.name, func(t *testing.T) {
			result, err := Render([]byte(`{"outbounds":[{"type":"anytls","tag":"Any","server":"example.test","server_port":443,"password":"secret","tls":{`+test.tls+`}}]}`), RenderChannel{Format: RenderFormatLoon})
			if err != nil {
				t.Fatal(err)
			}
			if test.reason != "" {
				if result.NodeCount != 0 || len(result.PreviewDiagnostics) != 1 || result.PreviewDiagnostics[0].Reason != test.reason || result.PreviewDiagnostics[0].FieldPath != "outbounds[0].tls.alpn" {
					t.Fatalf("unexpected failure: %+v", result.PreviewDiagnostics)
				}
				return
			}
			if result.NodeCount != 1 || len(result.Diagnostics) != 0 || !strings.Contains(string(result.Content), "skip-cert-verify=false") {
				t.Fatalf("node omitted or TLS changed: %s", result.Content)
			}
			if test.want == "" && strings.Contains(string(result.Content), "alpn=") || test.want != "" && !strings.Contains(string(result.Content), test.want) {
				t.Fatalf("ALPN changed: %s", result.Content)
			}
		})
	}
}

func TestRenderLoonConvertsProvenSubsetAndEscapesSecrets(t *testing.T) {
	t.Parallel()
	startup := []byte(`{"outbounds":[{"type":"vless","tag":"vless","server":"v.example","server_port":443,"uuid":"uuid-v","flow":"xtls-rprx-vision","tls":{"enabled":true,"server_name":"sni.example"}},{"type":"shadowsocks","tag":"ss","server":"ss.example","server_port":8388,"method":"aes-128-gcm","password":"p\"ass"},{"type":"hysteria2","tag":"hy2","server":"hy.example","server_port":8443,"password":"hy-secret","obfs":{"type":"salamander","password":"obfs-secret"},"tls":{"enabled":true,"server_name":"hy.example","insecure":true}},{"type":"tuic","tag":"tuic","server":"tuic.example","server_port":443,"uuid":"uuid-t","password":"tuic-secret","tls":{"enabled":true}},{"type":"vmess","tag":"bad=tag","server":"bad.example","server_port":443,"uuid":"uuid-bad"}]}`)
	result, err := Render(startup, RenderChannel{Format: RenderFormatLoon})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	want := `[Proxy]
hy2 = Hysteria2,hy.example,8443,"hy-secret",salamander-password="obfs-secret",sni=hy.example,skip-cert-verify=true,udp=true
ss = Shadowsocks,ss.example,8388,aes-128-gcm,"p\"ass",udp=true
vless = VLESS,v.example,443,"uuid-v",transport=tcp,flow=xtls-rprx-vision,over-tls=true,sni=sni.example,skip-cert-verify=false,udp=true
`
	if string(result.Content) != want || result.NodeCount != 3 {
		t.Fatalf("result = %#v, content %s", result, result.Content)
	}
	wantDiagnostics := []RenderDiagnostic{
		diagnostic(RenderFormatLoon, CollectionOutbounds, 3, DiagnosticUnsupportedType),
		diagnostic(RenderFormatLoon, CollectionOutbounds, 4, DiagnosticInvalidMetadata),
	}
	if !reflect.DeepEqual(result.Diagnostics, wantDiagnostics) {
		t.Fatalf("Diagnostics = %#v, want %#v", result.Diagnostics, wantDiagnostics)
	}
}
