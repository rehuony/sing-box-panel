// SPDX-License-Identifier: GPL-3.0-or-later

package render

import (
	"reflect"
	"testing"
)

func TestRenderLoonConvertsProvenSubsetAndEscapesSecrets(t *testing.T) {
	t.Parallel()
	startup := []byte(`{"outbounds":[{"type":"vless","tag":"vless","server":"v.example","server_port":443,"uuid":"uuid-v","flow":"xtls-rprx-vision","tls":{"enabled":true,"server_name":"sni.example"}},{"type":"shadowsocks","tag":"ss","server":"ss.example","server_port":8388,"method":"aes-128-gcm","password":"p\"ass"},{"type":"hysteria2","tag":"hy2","server":"hy.example","server_port":8443,"password":"hy-secret","obfs":{"type":"salamander","password":"obfs-secret"},"tls":{"enabled":true,"server_name":"hy.example","insecure":true}},{"type":"tuic","tag":"tuic","server":"tuic.example","server_port":443,"uuid":"uuid-t","password":"tuic-secret","tls":{"enabled":true}},{"type":"vmess","tag":"bad=tag","server":"bad.example","server_port":443,"uuid":"uuid-bad"}]}`)
	result, err := Render(startup, Channel{Format: FormatLoon})
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
	wantDiagnostics := []Diagnostic{
		diagnostic(FormatLoon, CollectionOutbounds, 3, DiagnosticUnsupportedType),
		diagnostic(FormatLoon, CollectionOutbounds, 4, DiagnosticInvalidMetadata),
	}
	if !reflect.DeepEqual(result.Diagnostics, wantDiagnostics) {
		t.Fatalf("Diagnostics = %#v, want %#v", result.Diagnostics, wantDiagnostics)
	}
}
