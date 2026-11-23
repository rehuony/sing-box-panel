// SPDX-License-Identifier: GPL-3.0-or-later

package subscription

import (
	"encoding/base64"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
)

// readingSummary is each item as "location|name|protocol|kept|origin key".
func readingSummary(reading SourceReading) []string {
	summary := make([]string, len(reading.Items))
	for index, item := range reading.Items {
		summary[index] = fmt.Sprintf("%s|%s|%s|%t|%s", item.Location, item.Name, item.Protocol, item.Kept, item.OriginKey)
	}
	return summary
}

func TestReadSourceSingBoxConfiguration(t *testing.T) {
	content := `{
  "log": {"level": "warn"},
  "outbounds": [
    {"type": "direct", "tag": "direct"},
    {"type": "selector", "tag": "proxy", "outbounds": ["Tokyo"]},
    {"type": "shadowsocks", "tag": "Tokyo", "server": "tokyo.example", "server_port": 8388, "method": "aes-128-gcm", "password": "secret"},
    {"type": "vless", "tag": "Seoul", "server": "seoul.example", "server_port": 443, "uuid": "uuid", "detour": "Tokyo",
     "tls": {"enabled": true, "server_name": "seoul.example"}},
    {"type": "block", "tag": "block"},
    {"type": "urltest", "tag": "auto", "outbounds": ["Tokyo"]}
  ],
  "endpoints": [
    {"type": "wireguard", "tag": "Home", "address": ["10.0.0.2/32"], "private_key": "key",
     "peers": [{"address": "home.example", "port": 51820, "public_key": "peer", "allowed_ips": ["0.0.0.0/0"]}]}
  ],
  "route": {"final": "proxy"}
}`
	for _, format := range []SourceFormat{SourceFormatAuto, SourceFormatSingBox} {
		reading, err := ReadSource([]byte(content), format, SourceFilter{})
		if err != nil {
			t.Fatal(err)
		}
		want := []string{
			"outbounds[2]|Tokyo|shadowsocks|true|Tokyo",
			"outbounds[3]|Seoul|vless|true|Seoul",
			"endpoints[0]|Home|wireguard|true|Home",
		}
		if reading.Format != SourceFormatSingBox || !slices.Equal(readingSummary(reading), want) {
			t.Fatalf("%s reading = %s %v", format, reading.Format, readingSummary(reading))
		}
		seoul := reading.Items[1].Outbound
		if _, tagged := seoul["tag"]; tagged || seoul["detour"] != "Tokyo" || seoul["server"] != "seoul.example" {
			t.Fatalf("converted outbound = %v", seoul)
		}
	}
	// An outbound array and a single outbound read too.
	for content, want := range map[string]string{
		`[{"type":"socks","tag":"A","server":"a.example","server_port":1080}]`: "[0]|A|socks|true|A",
		`{"type":"socks","server":"a.example","server_port":1080}`:             "outbound|socks a.example:1080|socks|true|socks a.example:1080",
	} {
		reading, err := ReadSource([]byte(content), SourceFormatAuto, SourceFilter{})
		if err != nil || !slices.Equal(readingSummary(reading), []string{want}) {
			t.Fatalf("%s: %v %v", content, readingSummary(reading), err)
		}
	}
}

func TestReadSourceMihomoConfiguration(t *testing.T) {
	content := `mixed-port: 7890
mode: rule
proxy-providers:
  remote:
    type: http
    url: https://provider.example/list.yaml
proxies:
  - name: "🇭🇰 Hong Kong"
    type: ss
    server: hk.example
    port: 8388
    cipher: aes-256-gcm
    password: secret
  - {name: Tokyo, type: trojan, server: tokyo.example, port: 443, password: pass, sni: tokyo.example}
  - {name: Tokyo, type: vmess, server: tokyo2.example, port: 443, uuid: uuid, cipher: auto, alterId: 0}
  - {type: socks5, server: socks.example, port: 1080}
  - {name: Snell, type: snell, server: snell.example, port: 443, psk: key}
proxy-groups:
  - {name: Proxy, type: select, proxies: [Tokyo]}
rules:
  - MATCH,Proxy
`
	reading, err := ReadSource([]byte(content), SourceFormatAuto, SourceFilter{Exclude: []string{"snell"}})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"proxies[0]|🇭🇰 Hong Kong|shadowsocks|true|🇭🇰 Hong Kong",
		"proxies[1]|Tokyo|trojan|true|Tokyo",
		"proxies[2]|Tokyo|vmess|true|Tokyo#2",
		"proxies[3]|socks socks.example:1080|socks|true|socks socks.example:1080",
		"proxies[4]|Snell|snell|false|Snell",
	}
	if reading.Format != SourceFormatMihomo || !slices.Equal(readingSummary(reading), want) {
		t.Fatalf("reading = %s %v", reading.Format, readingSummary(reading))
	}
	if outbound := reading.Items[1].Outbound; outbound["type"] != "trojan" || outbound["password"] != "pass" {
		t.Fatalf("converted outbound = %v", outbound)
	}
	// The unsupported proxy fails the content unless the filter drops it.
	_, err = ReadSource([]byte(content), SourceFormatAuto, SourceFilter{})
	var parseErr *SourceParseError
	if !errors.As(err, &parseErr) || parseErr.Code != SourceErrorNodeNotConvertible || parseErr.Location != "proxies[4]" || parseErr.Name != "Snell" || parseErr.Reason == "" {
		t.Fatalf("unconvertible kept node error = %v", err)
	}
	// A proxy list reads as well.
	reading, err = ReadSource([]byte("- {name: A, type: http, server: a.example, port: 8080}\n"), SourceFormatAuto, SourceFilter{})
	if err != nil || !slices.Equal(readingSummary(reading), []string{"[0]|A|http|true|A"}) {
		t.Fatalf("proxy list = %v %v", readingSummary(reading), err)
	}
}

func TestReadSourceShareLinks(t *testing.T) {
	vmess := base64.StdEncoding.EncodeToString([]byte(`{"v":"2","ps":"VMess WS","add":"vmess.example","port":"443","id":"uuid","aid":"0","scy":"auto","net":"ws","path":"/ws","host":"cdn.example","tls":"tls","sni":"cdn.example"}`))
	userInfo := base64.RawURLEncoding.EncodeToString([]byte("aes-256-gcm:secret"))
	links := strings.Join([]string{
		"# exported links",
		"ss://" + userInfo + "@ss.example:8388#Shadowsocks",
		"vmess://" + vmess,
		"vless://uuid@vless.example:443?security=reality&sni=www.example&pbk=" + strings.Repeat("A", 43) + "&sid=ab&fp=chrome&type=grpc&serviceName=svc#Reality",
		"trojan://pass@trojan.example:443?sni=trojan.example#Trojan",
		"hy2://pass@hy2.example:443?sni=hy2.example&obfs=salamander&obfs-password=obfs",
		"tuic://uuid:pass@tuic.example:443?congestion_control=bbr#TUIC",
		"",
		"socks5://user:pass@socks.example:1080#Socks",
		"https://user:pass@proxy.example:443#Proxy",
		"anytls://pass@anytls.example:443#AnyTLS",
		"hysteria://hysteria.example:443?auth=token#Hysteria",
		"trojan://pass@trojan.example:443?sni=trojan.example#Trojan",
	}, "\n")
	for name, content := range map[string]string{
		"plain":  links,
		"base64": base64.StdEncoding.EncodeToString([]byte(links)),
		"lines":  strings.Join(splitEvery(base64.StdEncoding.EncodeToString([]byte(links)), 76), "\r\n"),
	} {
		t.Run(name, func(t *testing.T) {
			reading, err := ReadSource([]byte(content), SourceFormatAuto, SourceFilter{})
			if err != nil {
				t.Fatal(err)
			}
			want := []string{
				"line 2|Shadowsocks|shadowsocks|true|Shadowsocks",
				"line 3|VMess WS|vmess|true|VMess WS",
				"line 4|Reality|vless|true|Reality",
				"line 5|Trojan|trojan|true|Trojan",
				"line 6|hysteria2 hy2.example:443|hysteria2|true|hysteria2 hy2.example:443",
				"line 7|TUIC|tuic|true|TUIC",
				"line 9|Socks|socks|true|Socks",
				"line 10|Proxy|http|true|Proxy",
				"line 11|AnyTLS|anytls|true|AnyTLS",
				"line 12|Hysteria|hysteria|true|Hysteria",
				"line 13|Trojan|trojan|true|Trojan#2",
			}
			if reading.Format != SourceFormatShareLinks || !slices.Equal(readingSummary(reading), want) {
				t.Fatalf("reading = %s %v", reading.Format, readingSummary(reading))
			}
			vmessOutbound := reading.Items[1].Outbound
			transport, _ := vmessOutbound["transport"].(map[string]any)
			tls, _ := vmessOutbound["tls"].(map[string]any)
			if transport["type"] != "ws" || transport["path"] != "/ws" || tls["server_name"] != "cdn.example" {
				t.Fatalf("vmess outbound = %v", vmessOutbound)
			}
			reality := reading.Items[2].Outbound["tls"].(map[string]any)["reality"].(map[string]any)
			if reality["short_id"] != "ab" || reading.Items[2].Outbound["transport"].(map[string]any)["service_name"] != "svc" {
				t.Fatalf("vless outbound = %v", reading.Items[2].Outbound)
			}
			if obfs := reading.Items[4].Outbound["obfs"].(map[string]any); obfs["type"] != "salamander" {
				t.Fatalf("hysteria2 outbound = %v", reading.Items[4].Outbound)
			}
		})
	}
}

func splitEvery(value string, size int) []string {
	parts := []string{}
	for len(value) > size {
		parts, value = append(parts, value[:size]), value[size:]
	}
	return append(parts, value)
}

func TestSourceFilterMatchesNames(t *testing.T) {
	links := strings.Join([]string{
		"socks5://a.example:1080#HK 01",
		"socks5://b.example:1080#hk-02 IPLC",
		"socks5://c.example:1080#Japan 01",
		"socks5://d.example:1080#US 01 [expire]",
	}, "\n")
	kept := func(filter SourceFilter) []string {
		t.Helper()
		reading, err := ReadSource([]byte(links), SourceFormatShareLinks, filter)
		if err != nil {
			t.Fatal(err)
		}
		names := []string{}
		for _, item := range reading.Kept() {
			names = append(names, item.Name)
		}
		return names
	}
	for _, test := range []struct {
		name   string
		filter SourceFilter
		want   []string
	}{
		{"none", SourceFilter{}, []string{"HK 01", "hk-02 IPLC", "Japan 01", "US 01 [expire]"}},
		{"substring ignores case", SourceFilter{Include: []string{"hk"}}, []string{"HK 01", "hk-02 IPLC"}},
		{"regular expression", SourceFilter{Include: []string{`/^(HK|Japan) \d+$/`}}, []string{"HK 01", "Japan 01"}},
		{"exclude only", SourceFilter{Exclude: []string{"[EXPIRE]"}}, []string{"HK 01", "hk-02 IPLC", "Japan 01"}},
		{"include then exclude", SourceFilter{Include: []string{"01", "/iplc/"}, Exclude: []string{"/(?i)^us/", "japan"}}, []string{"HK 01"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := kept(test.filter); !slices.Equal(got, test.want) {
				t.Fatalf("kept %v, want %v", got, test.want)
			}
		})
	}
	for _, filter := range []SourceFilter{{Include: []string{"/(/"}}, {Exclude: []string{""}}, {Exclude: []string{strings.Repeat("x", 513)}}} {
		var field *SourceFieldError
		if _, err := ReadSource([]byte(links), SourceFormatShareLinks, filter); !errors.As(err, &field) || !strings.HasPrefix(field.Path, "filter.") {
			t.Fatalf("invalid filter %v accepted: %v", filter, err)
		}
	}
}

func TestReadSourceIsStrict(t *testing.T) {
	for _, test := range []struct {
		name, content string
		format        SourceFormat
		code          string
		location      string
	}{
		{"empty", " \n ", SourceFormatAuto, SourceErrorEmptyContent, ""},
		{"unrecognized", "<html>gateway error</html>", SourceFormatAuto, SourceErrorUnrecognizedFormat, ""},
		{"broken JSON", `{"outbounds":[`, SourceFormatSingBox, SourceErrorInvalidContent, ""},
		{"non-object outbound", `{"outbounds":[1]}`, SourceFormatSingBox, SourceErrorInvalidContent, "outbounds[0]"},
		{"mihomo without proxies", "mode: rule\n", SourceFormatMihomo, SourceErrorInvalidContent, ""},
		{"stray line", "socks5://a.example:1080#A\nnot a link\n", SourceFormatShareLinks, SourceErrorInvalidContent, "line 2"},
		{"unknown scheme", "socks5://a.example:1080#A\nwireguard://b.example:51820#B\n", SourceFormatShareLinks, SourceErrorNodeNotConvertible, "line 2"},
		{"missing credential", "trojan://trojan.example:443#T\n", SourceFormatShareLinks, SourceErrorNodeNotConvertible, "line 1"},
		{"invalid outbound", `{"outbounds":[{"type":"hysteria2","tag":"H","server":"h.example","server_port":443,"password":"p"}]}`, SourceFormatAuto, SourceErrorNodeNotConvertible, "outbounds[0]"},
		{"control character", "socks5://a.example:1080#A%0AB\n", SourceFormatShareLinks, SourceErrorInvalidNodeName, "line 1"},
		{"another protocol", "socks5://a.example:1080#A\ntrojan://pass@t.example:443#T\n", "socks", SourceErrorInvalidContent, "line 2"},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := ReadSource([]byte(test.content), test.format, SourceFilter{})
			var parseErr *SourceParseError
			if !errors.As(err, &parseErr) || parseErr.Code != test.code || parseErr.Location != test.location {
				t.Fatalf("error = %#v (%v), want %s at %q", parseErr, err, test.code, test.location)
			}
		})
	}
	// Excluding the node that does not convert makes the content readable.
	reading, err := ReadSource([]byte("socks5://a.example:1080#A\nwireguard://b.example:51820#B\n"), SourceFormatShareLinks, SourceFilter{Exclude: []string{"B"}})
	if err != nil || len(reading.Kept()) != 1 {
		t.Fatalf("filtered reading = %v, %v", readingSummary(reading), err)
	}
}
