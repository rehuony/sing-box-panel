// SPDX-License-Identifier: GPL-3.0-or-later

package source

import (
	"bytes"
	"encoding/base64"
	"errors"
	"testing"

	"github.com/rehuony/sing-box-panel/internal/subscription/render"
)

func TestParseSourceFormats(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		format Format
		raw    []byte
		want   Format
		count  int
	}{
		{
			name: "sing-box JSON", format: FormatAuto, want: FormatSingBoxJSON, count: 1,
			raw: []byte(`{"outbounds":[{"type":"shadowsocks","tag":"ss","server":"ss.example","server_port":8388,"method":"aes-128-gcm","password":"pass"}]}`),
		},
		{
			name: "Mihomo YAML", format: FormatAuto, want: FormatMihomoYAML, count: 2,
			raw: []byte(`proxies:
  - name: ss
    type: ss
    server: ss.example
    port: 8388
    cipher: aes-128-gcm
    password: pass
  - name: trojan
    type: trojan
    server: trojan.example
    port: 443
    password: pass
    sni: public.example
`),
		},
		{
			name: "URI list", format: FormatURIList, want: FormatURIList, count: 4,
			raw: []byte("socks5://user:pass@socks.example:1080#socks\n" +
				"https://user:pass@http.example:443#https\n" +
				"vless://uuid@vless.example:443?security=tls&sni=vless.example#vless\n" +
				"trojan://pass@trojan.example:443?sni=trojan.example#trojan\n"),
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			nodes, detected, err := Parse(test.format, test.raw, "fixture")
			if err != nil || detected != test.want || len(nodes) != test.count {
				t.Fatalf("detected=%s nodes=%+v err=%v", detected, nodes, err)
			}
			for _, node := range nodes {
				if !bytes.HasPrefix([]byte(node.Key), []byte("source:fixture:")) {
					t.Fatalf("node key=%q", node.Key)
				}
			}
			if _, err := render.RenderNodes(nodes, render.Channel{Format: render.FormatSingBox}); err != nil {
				t.Fatalf("RenderNodes: %v", err)
			}
		})
	}
}

func TestParseSourceBase64URIListAndVMess(t *testing.T) {
	t.Parallel()
	vmessJSON := `{"v":"2","ps":"vmess","add":"vm.example","port":"443","id":"uuid","aid":"0","scy":"auto","tls":"tls","sni":"vm.example"}`
	vmess := "vmess://" + base64.RawStdEncoding.EncodeToString([]byte(vmessJSON))
	list := "ss://" + base64.RawURLEncoding.EncodeToString([]byte("aes-128-gcm:pass")) + "@ss.example:8388#ss\n" + vmess + "\n"
	raw := []byte(base64.StdEncoding.EncodeToString([]byte(list)))
	nodes, detected, err := Parse(FormatAuto, raw, "base64")
	if err != nil || detected != FormatURIList || len(nodes) != 2 {
		t.Fatalf("detected=%s nodes=%+v err=%v", detected, nodes, err)
	}
}

func TestParseSourceRejectsWholeMaliciousOrAmbiguousCandidate(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		format Format
		raw    []byte
	}{
		{name: "YAML duplicate key", format: FormatMihomoYAML, raw: []byte("proxies:\n  - name: one\n    name: two\n")},
		{name: "YAML alias", format: FormatMihomoYAML, raw: []byte("defaults: &x {server: example}\nproxies:\n  - *x\n")},
		{name: "JSON duplicate key", format: FormatSingBoxJSON, raw: []byte(`{"outbounds":[],"outbounds":[]}`)},
		{name: "duplicate identity", format: FormatSingBoxJSON, raw: []byte(`[{"type":"socks","tag":"one","server":"example","server_port":1080},{"type":"socks","tag":"one","server":"example","server_port":1080}]`)},
		{name: "missing required", format: FormatSingBoxJSON, raw: []byte(`[{"type":"trojan","tag":"one","server":"example","server_port":443}]`)},
		{name: "unsupported URI", format: FormatURIList, raw: []byte("ftp://example.test/file\n")},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, _, err := Parse(test.format, test.raw, "invalid"); !errors.Is(err, ErrInvalidSource) {
				t.Fatalf("error=%v", err)
			}
		})
	}
}
