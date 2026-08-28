// SPDX-License-Identifier: GPL-3.0-or-later

package subscription

import (
	"bytes"
	"reflect"
	"strings"
	"testing"
)

func TestRenderHandlesWireGuardEndpointAcrossFormats(t *testing.T) {
	t.Parallel()

	startup := []byte(`{
      "endpoints":[
        {"type":"tailscale","tag":"private-tailnet","auth_key":"must-not-publish"},
        {"type":"wireguard","tag":"wg","address":["10.0.0.2/32"],"private_key":"private-key","peers":[{"address":"wg.example","port":51820,"public_key":"public-key","allowed_ips":["0.0.0.0/0"]}]}
      ],
      "outbounds":[
        {"type":"shadowsocks","tag":"chain","server":"ss.example","server_port":443,"method":"aes-128-gcm","password":"pass","detour":"wg"}
      ]
    }`)

	singBox, err := Render(startup, RenderChannel{Format: RenderFormatSingBox})
	if err != nil {
		t.Fatalf("Render sing-box: %v", err)
	}
	want := `{"endpoints":[{"address":["10.0.0.2/32"],"peers":[{"address":"wg.example","allowed_ips":["0.0.0.0/0"],"port":51820,"public_key":"public-key"}],"private_key":"private-key","tag":"wg","type":"wireguard"}],"outbounds":[{"detour":"wg","method":"aes-128-gcm","password":"pass","server":"ss.example","server_port":443,"tag":"chain","type":"shadowsocks"}]}` + "\n"
	if string(singBox.Content) != want || singBox.NodeCount != 2 || len(singBox.Diagnostics) != 0 {
		t.Fatalf("sing-box result = %#v, content %s", singBox, singBox.Content)
	}
	if bytes.Contains(singBox.Content, []byte("must-not-publish")) {
		t.Fatalf("non-publishable endpoint leaked into output: %s", singBox.Content)
	}

	for _, format := range []RenderFormat{RenderFormatMihomo, RenderFormatLoon} {
		result, err := Render(startup, RenderChannel{Format: format})
		if err != nil {
			t.Fatalf("Render %s: %v", format, err)
		}
		if result.NodeCount != 0 || len(result.Diagnostics) != 2 {
			t.Fatalf("%s result = %#v, content %s", format, result, result.Content)
		}
		if !containsDiagnostic(result.Diagnostics, CollectionEndpoints, 1, DiagnosticUnsupportedType) ||
			!containsDiagnostic(result.Diagnostics, CollectionOutbounds, 0, DiagnosticUnresolvedDependency) {
			t.Fatalf("%s diagnostics = %#v", format, result.Diagnostics)
		}
	}

	filtered, err := Render(startup, RenderChannel{Format: RenderFormatSingBox, ExcludeTypes: []string{"wireguard"}})
	if err != nil {
		t.Fatalf("Render filtered: %v", err)
	}
	if filtered.NodeCount != 0 || string(filtered.Content) != `{"endpoints":[],"outbounds":[]}`+"\n" {
		t.Fatalf("filtered result = %#v, content %s", filtered, filtered.Content)
	}
}

func TestDuplicateTagAcrossEndpointAndOutboundOmitsBoth(t *testing.T) {
	t.Parallel()

	startup := []byte(`{
      "endpoints":[{"type":"wireguard","tag":"same","address":["10.0.0.2/32"],"private_key":"private","peers":[{"address":"wg.example","port":51820,"public_key":"public","allowed_ips":["0.0.0.0/0"]}]}],
      "outbounds":[{"type":"shadowsocks","tag":"same","server":"ss.example","server_port":443,"method":"aes-128-gcm","password":"pass"}]
    }`)
	result, err := Render(startup, RenderChannel{Format: RenderFormatSingBox})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if result.NodeCount != 0 || string(result.Content) != `{"endpoints":[],"outbounds":[]}`+"\n" {
		t.Fatalf("result = %#v, content %s", result, result.Content)
	}
	want := []RenderDiagnostic{
		diagnostic(RenderFormatSingBox, CollectionEndpoints, 0, DiagnosticDuplicateTag),
		diagnostic(RenderFormatSingBox, CollectionOutbounds, 0, DiagnosticDuplicateTag),
	}
	if !reflect.DeepEqual(result.Diagnostics, want) {
		t.Fatalf("Diagnostics = %#v, want %#v", result.Diagnostics, want)
	}
}

func TestRenderStableOrderDoesNotDependOnStartupOrder(t *testing.T) {
	t.Parallel()

	alpha := `{"type":"shadowsocks","tag":"alpha","server":"a.example","server_port":1,"method":"aes-128-gcm","password":"a"}`
	zulu := `{"type":"shadowsocks","tag":"zulu","server":"z.example","server_port":2,"method":"aes-128-gcm","password":"z"}`
	first := []byte(`{"outbounds":[` + zulu + `,` + alpha + `]}`)
	second := []byte(`{"outbounds":[` + alpha + `,` + zulu + `]}`)

	for _, format := range []RenderFormat{RenderFormatSingBox, RenderFormatMihomo, RenderFormatLoon} {
		firstResult, err := Render(first, RenderChannel{Format: format})
		if err != nil {
			t.Fatalf("Render first %s: %v", format, err)
		}
		secondResult, err := Render(second, RenderChannel{Format: format})
		if err != nil {
			t.Fatalf("Render second %s: %v", format, err)
		}
		if !bytes.Equal(firstResult.Content, secondResult.Content) {
			t.Fatalf("%s output depends on startup order:\n%s\n%s", format, firstResult.Content, secondResult.Content)
		}
	}
}

func TestSupportedRendererProtocolMatrix(t *testing.T) {
	t.Parallel()

	tls := `"tls":{"enabled":true,"server_name":"tls.example","insecure":false,"alpn":["h2"]}`
	tests := []struct {
		name     string
		format   RenderFormat
		outbound string
		contains []string
	}{
		{
			name: "mihomo socks5", format: RenderFormatMihomo,
			outbound: `{"type":"socks","tag":"node","server":"socks.example","server_port":1080,"version":"5","username":"user","password":"pass","network":["tcp","udp"]}`,
			contains: []string{`type: "socks5"`, `username: "user"`, `password: "pass"`, `udp: true`},
		},
		{
			name: "mihomo https", format: RenderFormatMihomo,
			outbound: `{"type":"http","tag":"node","server":"http.example","server_port":443,"username":"user","password":"pass",` + tls + `}`,
			contains: []string{`type: "http"`, `tls: true`, `sni: "tls.example"`, `alpn: ["h2"]`},
		},
		{
			name: "mihomo vmess", format: RenderFormatMihomo,
			outbound: `{"type":"vmess","tag":"node","server":"vm.example","server_port":443,"uuid":"uuid","security":"aes-128-gcm","alter_id":0,"packet_encoding":"packetaddr",` + tls + `}`,
			contains: []string{`type: "vmess"`, `alterId: 0`, `cipher: "aes-128-gcm"`, `packet-encoding: "packetaddr"`, `servername: "tls.example"`},
		},
		{
			name: "mihomo vless", format: RenderFormatMihomo,
			outbound: `{"type":"vless","tag":"node","server":"vless.example","server_port":443,"uuid":"uuid","flow":"xtls-rprx-vision","packet_encoding":"xudp",` + tls + `}`,
			contains: []string{`type: "vless"`, `flow: "xtls-rprx-vision"`, `packet-encoding: "xudp"`, `tls: true`},
		},
		{
			name: "mihomo tuic", format: RenderFormatMihomo,
			outbound: `{"type":"tuic","tag":"node","server":"tuic.example","server_port":443,"uuid":"uuid","password":"pass","congestion_control":"bbr","udp_relay_mode":"native","zero_rtt_handshake":true,` + tls + `}`,
			contains: []string{`type: "tuic"`, `congestion-controller: "bbr"`, `udp-relay-mode: "native"`, `reduce-rtt: true`},
		},
		{
			name: "mihomo anytls", format: RenderFormatMihomo,
			outbound: `{"type":"anytls","tag":"node","server":"any.example","server_port":443,"password":"pass",` + tls + `}`,
			contains: []string{`type: "anytls"`, `password: "pass"`, `sni: "tls.example"`},
		},
		{
			name: "loon socks5", format: RenderFormatLoon,
			outbound: `{"type":"socks","tag":"node","server":"socks.example","server_port":1080,"version":"5","username":"user","password":"pass","network":"tcp"}`,
			contains: []string{`node = socks5,socks.example,1080,"user","pass",udp=false`},
		},
		{
			name: "loon https", format: RenderFormatLoon,
			outbound: `{"type":"http","tag":"node","server":"http.example","server_port":443,"username":"user","password":"pass",` + tls + `}`,
			contains: []string{`node = https,http.example,443,"user","pass",sni=tls.example,alpn=h2,skip-cert-verify=false`},
		},
		{
			name: "loon vmess", format: RenderFormatLoon,
			outbound: `{"type":"vmess","tag":"node","server":"vm.example","server_port":443,"uuid":"uuid","security":"chacha20-poly1305","alter_id":0,` + tls + `}`,
			contains: []string{`node = vmess,vm.example,443,chacha20-poly1305,"uuid",transport=tcp,alterId=0,over-tls=true,sni=tls.example,alpn=h2,skip-cert-verify=false,udp=true`},
		},
		{
			name: "loon trojan", format: RenderFormatLoon,
			outbound: `{"type":"trojan","tag":"node","server":"tr.example","server_port":443,"password":"pass",` + tls + `}`,
			contains: []string{`node = trojan,tr.example,443,"pass",sni=tls.example,alpn=h2,skip-cert-verify=false,udp=true`},
		},
		{
			name: "loon anytls", format: RenderFormatLoon,
			outbound: `{"type":"anytls","tag":"node","server":"any.example","server_port":443,"password":"pass",` + tls + `}`,
			contains: []string{`node = AnyTLS,any.example,443,"pass",sni=tls.example,alpn=h2,skip-cert-verify=false,udp=true`},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			result, err := Render([]byte(`{"outbounds":[`+test.outbound+`]}`), RenderChannel{Format: test.format})
			if err != nil {
				t.Fatalf("Render: %v", err)
			}
			if result.NodeCount != 1 || len(result.Diagnostics) != 0 {
				t.Fatalf("result = %#v, content %s", result, result.Content)
			}
			for _, want := range test.contains {
				if !strings.Contains(string(result.Content), want) {
					t.Fatalf("content missing %q:\n%s", want, result.Content)
				}
			}
		})
	}
}

func TestConversionFailuresUseStableCodes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		format   RenderFormat
		outbound string
		code     DiagnosticCode
	}{
		{name: "missing port", format: RenderFormatMihomo, outbound: `{"type":"shadowsocks","tag":"node","server":"a","method":"aes-128-gcm","password":"pass"}`, code: DiagnosticInvalidRequiredField},
		{name: "fractional port", format: RenderFormatLoon, outbound: `{"type":"shadowsocks","tag":"node","server":"a","server_port":1.5,"method":"aes-128-gcm","password":"pass"}`, code: DiagnosticInvalidRequiredField},
		{name: "unknown option", format: RenderFormatMihomo, outbound: `{"type":"shadowsocks","tag":"node","server":"a","server_port":1,"method":"aes-128-gcm","password":"pass","future":true}`, code: DiagnosticUnsupportedOption},
		{name: "udp only network", format: RenderFormatLoon, outbound: `{"type":"shadowsocks","tag":"node","server":"a","server_port":1,"method":"aes-128-gcm","password":"pass","network":"udp"}`, code: DiagnosticUnsupportedNetwork},
		{name: "detour", format: RenderFormatMihomo, outbound: `{"type":"shadowsocks","tag":"node","server":"a","server_port":1,"method":"aes-128-gcm","password":"pass","detour":"other"}`, code: DiagnosticUnresolvedDependency},
		{name: "complex tls", format: RenderFormatMihomo, outbound: `{"type":"trojan","tag":"node","server":"a","server_port":1,"password":"pass","tls":{"enabled":true,"utls":{"enabled":true}}}`, code: DiagnosticUnsupportedTLS},
		{name: "loon multiple alpn", format: RenderFormatLoon, outbound: `{"type":"trojan","tag":"node","server":"a","server_port":1,"password":"pass","tls":{"enabled":true,"alpn":["h2","http/1.1"]}}`, code: DiagnosticUnsupportedTLS},
		{name: "mihomo unknown cipher", format: RenderFormatMihomo, outbound: `{"type":"shadowsocks","tag":"node","server":"a","server_port":1,"method":"future-cipher","password":"pass"}`, code: DiagnosticUnsupportedOption},
		{name: "loon unsupported cipher", format: RenderFormatLoon, outbound: `{"type":"shadowsocks","tag":"node","server":"a","server_port":1,"method":"none","password":"pass"}`, code: DiagnosticUnsupportedOption},
		{name: "invalid tuic relay", format: RenderFormatMihomo, outbound: `{"type":"tuic","tag":"node","server":"a","server_port":1,"uuid":"uuid","password":"pass","udp_relay_mode":"future","tls":{"enabled":true}}`, code: DiagnosticInvalidRequiredField},
		{name: "invalid hysteria obfs", format: RenderFormatLoon, outbound: `{"type":"hysteria2","tag":"node","server":"a","server_port":1,"password":"pass","obfs":{"type":"gecko","password":"obfs"},"tls":{"enabled":true}}`, code: DiagnosticUnsupportedOption},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			result, err := Render([]byte(`{"outbounds":[`+test.outbound+`]}`), RenderChannel{Format: test.format})
			if err != nil {
				t.Fatalf("Render: %v", err)
			}
			if result.NodeCount != 0 || len(result.Diagnostics) != 1 || result.Diagnostics[0].Code != test.code {
				t.Fatalf("result = %#v, want diagnostic %q", result, test.code)
			}
		})
	}
}

func TestRenderAppliesExactTagAndTypeExclusions(t *testing.T) {
	t.Parallel()

	startup := []byte(`{"outbounds":[
      {"type":"shadowsocks","tag":"keep","server":"a","server_port":1,"method":"none","password":"a"},
      {"type":"shadowsocks","tag":"drop-tag","server":"b","server_port":2,"method":"none","password":"b"},
      {"type":"vmess","tag":"drop-type","server":"c","server_port":3,"uuid":"c"}
    ]}`)
	result, err := Render(startup, RenderChannel{
		Format:       RenderFormatMihomo,
		ExcludeTags:  []string{"drop-tag"},
		ExcludeTypes: []string{"vmess"},
	})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if result.NodeCount != 1 || !bytes.Contains(result.Content, []byte(`name: "keep"`)) {
		t.Fatalf("result = count %d, content %s", result.NodeCount, result.Content)
	}
	if len(result.Diagnostics) != 0 {
		t.Fatalf("configured exclusions produced diagnostics: %#v", result.Diagnostics)
	}
}

func TestRenderEmptyOutbounds(t *testing.T) {
	t.Parallel()

	tests := []struct {
		format RenderFormat
		want   string
	}{
		{format: RenderFormatSingBox, want: "{\"outbounds\":[]}\n"},
		{format: RenderFormatMihomo, want: "proxies: []\n"},
		{format: RenderFormatLoon, want: "[Proxy]\n"},
	}
	for _, test := range tests {
		t.Run(string(test.format), func(t *testing.T) {
			t.Parallel()
			result, err := Render([]byte(`{"log":{"level":"info"}}`), RenderChannel{Format: test.format})
			if err != nil {
				t.Fatalf("Render: %v", err)
			}
			if string(result.Content) != test.want || result.NodeCount != 0 || result.Diagnostics == nil {
				t.Fatalf("result = %#v, content %q", result, result.Content)
			}
		})
	}
}

func containsDiagnostic(values []RenderDiagnostic, collection Collection, index int, code DiagnosticCode) bool {
	for _, value := range values {
		if value.Collection == collection && value.ItemIndex == index && value.Code == code {
			return true
		}
	}
	return false
}
