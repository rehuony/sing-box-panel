// SPDX-License-Identifier: GPL-3.0-or-later

package document

import (
	"errors"
	"strings"
	"testing"
)

func TestDecodeRejectsAmbiguousAndBoundedDocuments(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		code string
	}{
		{name: "duplicate", raw: `{"a":1,"a":2}`, code: "duplicate_object_key"},
		{name: "trailing", raw: `{} {}`, code: "trailing_json"},
		{name: "depth", raw: strings.Repeat("[", MaximumDepth+2) + "0" + strings.Repeat("]", MaximumDepth+2), code: "nesting_too_deep"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := Decode([]byte(test.raw))
			if !errors.Is(err, ErrInvalid) || Code(err) != test.code {
				t.Fatalf("Decode() error = %v, code = %q, want %q", err, Code(err), test.code)
			}
		})
	}
}

func TestIntegerAcceptsJSONAndSourceRepresentations(t *testing.T) {
	for _, value := range []any{1, int64(1), float64(1), "1"} {
		if got, ok := Integer(value, 1, 65535); !ok || got != 1 {
			t.Fatalf("Integer(%#v) = %d, %v, want 1, true", value, got, ok)
		}
	}
}
