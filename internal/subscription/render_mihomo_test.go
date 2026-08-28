// SPDX-License-Identifier: GPL-3.0-or-later

package subscription

import (
	"reflect"
	"testing"
)

func TestRenderMihomoConvertsProvenSubsetAndDiagnosesRemainder(t *testing.T) {
	t.Parallel()
	startup := []byte(`{"outbounds":[{"type":"shadowsocks","tag":"ss","server":"ss.example","server_port":443,"method":"2022-blake3-aes-128-gcm","password":"ss-secret"},{"type":"trojan","tag":"trojan","server":"tr.example","server_port":8443,"password":"tr-secret","network":"tcp","tls":{"enabled":true,"server_name":"sni.example","insecure":false,"alpn":["h2","http/1.1"]}},{"type":"vmess","tag":"transport","server":"vm.example","server_port":443,"uuid":"uuid-vm","transport":{"type":"ws","path":"/ws"}},{"type":"shadowtls","tag":"future","server":"future.example","server_port":443,"password":"future-secret"},{"type":"hysteria2","tag":"hy2","server":"hy.example","server_port":9443,"password":"hy-secret","up_mbps":30,"down_mbps":200,"obfs":{"type":"salamander","password":"obfs-secret"},"tls":{"enabled":true,"server_name":"hy-sni.example"}}]}`)
	result, err := Render(startup, RenderChannel{Format: RenderFormatMihomo})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	want := `proxies:
  - name: "hy2"
    type: "hysteria2"
    server: "hy.example"
    port: 9443
    udp: true
    password: "hy-secret"
    up: "30 Mbps"
    down: "200 Mbps"
    obfs: "salamander"
    obfs-password: "obfs-secret"
    sni: "hy-sni.example"
    skip-cert-verify: false
  - name: "ss"
    type: "ss"
    server: "ss.example"
    port: 443
    udp: true
    cipher: "2022-blake3-aes-128-gcm"
    password: "ss-secret"
  - name: "trojan"
    type: "trojan"
    server: "tr.example"
    port: 8443
    udp: false
    password: "tr-secret"
    sni: "sni.example"
    alpn: ["h2","http/1.1"]
    skip-cert-verify: false
`
	if string(result.Content) != want || result.NodeCount != 3 {
		t.Fatalf("result = %#v, content %s", result, result.Content)
	}
	wantDiagnostics := []RenderDiagnostic{
		diagnostic(RenderFormatMihomo, CollectionOutbounds, 2, DiagnosticUnsupportedTransport),
		diagnostic(RenderFormatMihomo, CollectionOutbounds, 3, DiagnosticUnsupportedType),
	}
	if !reflect.DeepEqual(result.Diagnostics, wantDiagnostics) {
		t.Fatalf("Diagnostics = %#v, want %#v", result.Diagnostics, wantDiagnostics)
	}
}
