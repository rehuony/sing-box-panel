// SPDX-License-Identifier: GPL-3.0-or-later

package jsonstrict

import (
	"strings"
	"testing"
)

func TestDecode(t *testing.T) {
	type nested struct {
		Enabled bool `json:"enabled"`
	}
	type document struct {
		Name   string `json:"name"`
		Nested nested `json:"nested"`
	}
	cases := []struct {
		name    string
		input   string
		wantErr string
	}{
		{name: "valid", input: `{"name":"panel","nested":{"enabled":true}}`},
		{name: "duplicate root", input: `{"name":"a","name":"b","nested":{"enabled":true}}`, wantErr: "duplicate object key"},
		{name: "duplicate nested", input: `{"name":"a","nested":{"enabled":true,"enabled":false}}`, wantErr: "duplicate object key"},
		{name: "unknown", input: `{"name":"a","nested":{"enabled":true},"extra":1}`, wantErr: "unknown field"},
		{name: "trailing", input: `{"name":"a","nested":{"enabled":true}} {}`, wantErr: "trailing"},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			var got document
			err := Decode([]byte(test.input), 1024, &got)
			if test.wantErr == "" {
				if err != nil {
					t.Fatalf("Decode() error = %v", err)
				}
				if got.Name != "panel" || !got.Nested.Enabled {
					t.Fatalf("Decode() = %#v", got)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("Decode() error = %v, want substring %q", err, test.wantErr)
			}
		})
	}
}

func TestDecodeLimit(t *testing.T) {
	var value map[string]any
	if err := Decode([]byte(`{"value":1}`), 4, &value); err == nil {
		t.Fatal("Decode() unexpectedly accepted oversized input")
	}
}
