// SPDX-License-Identifier: GPL-3.0-or-later

package subscription

import (
	"bytes"
	"reflect"
	"testing"
)

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
		for _, format := range []Format{FormatSingBox, FormatMihomo, FormatLoon} {
			first, firstErr := Render(input, Channel{Format: format})
			second, secondErr := Render(input, Channel{Format: format})
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
