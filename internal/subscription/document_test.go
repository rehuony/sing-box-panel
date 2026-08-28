// SPDX-License-Identifier: GPL-3.0-or-later

package subscription

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
		{name: "depth", raw: strings.Repeat("[", MaximumDocumentDepth+2) + "0" + strings.Repeat("]", MaximumDocumentDepth+2), code: "nesting_too_deep"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := DecodeDocument([]byte(test.raw))
			if !errors.Is(err, ErrInvalidDocument) || DocumentErrorCode(err) != test.code {
				t.Fatalf("DecodeDocument() error = %v, code = %q, want %q", err, DocumentErrorCode(err), test.code)
			}
		})
	}
}

func TestIntegerAcceptsJSONAndSourceRepresentations(t *testing.T) {
	for _, value := range []any{1, int64(1), float64(1), "1"} {
		if got, ok := DocumentInteger(value, 1, 65535); !ok || got != 1 {
			t.Fatalf("DocumentInteger(%#v) = %d, %v, want 1, true", value, got, ok)
		}
	}
}
