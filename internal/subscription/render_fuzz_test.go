// SPDX-License-Identifier: GPL-3.0-or-later

package subscription

import (
	"bytes"
	"reflect"
	"testing"
	"unicode/utf8"

	"go.yaml.in/yaml/v3"
)

func FuzzChannelYAMLUnicodeRoundTrip(f *testing.F) {
	for _, value := range []string{"", "🏠 🔑 🚀 🐟 ✨", "𠮷 👩🏽‍💻 🇯🇵", `"\U0001F3E0" \\U0001F511`, "line\nnext\t\"quoted\"\x00\r\n", "\U00010000\U0010FFFF"} {
		f.Add(value)
	}
	f.Fuzz(func(t *testing.T, value string) {
		if len(value) > 1024 || !utf8.ValidString(value) {
			t.Skip()
		}
		original := map[string]map[string]string{"strings": {"说明 " + value: "🏠 " + value, "literal": `\U0001F3E0`}}
		input, err := yaml.Marshal(original)
		if err != nil {
			t.Fatal(err)
		}
		template, err := parseChannelTemplate(&NativeTemplate{Format: RenderFormatMihomo, Content: string(input)}, RenderFormatMihomo)
		if err != nil {
			t.Fatal(err)
		}
		content, err := template.render(RenderFormatMihomo, nil)
		if err != nil {
			t.Fatal(err)
		}
		var restored map[string]map[string]string
		if err := yaml.Unmarshal(content, &restored); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(original, restored) {
			t.Fatalf("YAML changed strings: got %q, want %q", restored, original)
		}
		if !bytes.Contains(content, []byte("🏠")) {
			t.Fatal("emoji was escaped")
		}
	})
}

func FuzzRenderIsPureAndDeterministic(f *testing.F) {
	f.Add([]byte(`{"outbounds":[]}`))
	f.Add([]byte(`{"outbounds":[{"type":"shadowsocks","tag":"node","server":"example.com","server_port":443,"method":"aes-128-gcm","password":"secret"}]}`))
	f.Add([]byte(`{"outbounds":[{"type":"vmess","tag":"node","server":"example.com","server_port":443,"uuid":"uuid","transport":{"type":"ws"}}]}`))
	f.Add([]byte(`{"outbounds":[{"type":"direct","tag":"direct"}]}`))
	f.Add([]byte(`{"endpoints":[{"type":"wireguard","tag":"wg","address":["10.0.0.2/32"],"private_key":"private","peers":[{"address":"example.com","port":51820,"public_key":"public","allowed_ips":["0.0.0.0/0"]}]}],"outbounds":[]}`))
	f.Add([]byte(`{"a":{"duplicate":1,"duplicate":2}}`))

	f.Fuzz(func(t *testing.T, input []byte) {
		if len(input) > MaximumStartupBytes+1 {
			t.Skip()
		}
		original := bytes.Clone(input)
		for _, format := range []RenderFormat{RenderFormatSingBox, RenderFormatMihomo, RenderFormatLoon} {
			first, firstErr := Render(input, RenderChannel{Format: format})
			second, secondErr := Render(input, RenderChannel{Format: format})
			if (firstErr == nil) != (secondErr == nil) {
				t.Fatalf("%s error determinism: %v then %v", format, firstErr, secondErr)
			}
			if firstErr != nil {
				if firstErr.Error() != secondErr.Error() {
					t.Fatalf("%s error text changed: %q then %q", format, firstErr, secondErr)
				}
				continue
			}
			if !bytes.Equal(first.Content, second.Content) ||
				first.NodeCount != second.NodeCount ||
				!reflect.DeepEqual(first.Diagnostics, second.Diagnostics) {
				t.Fatalf("%s result is not deterministic", format)
			}
		}
		if !bytes.Equal(input, original) {
			t.Fatal("Render mutated caller input")
		}
	})
}
