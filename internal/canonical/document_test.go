// SPDX-License-Identifier: GPL-3.0-or-later

package canonical

import (
	"bytes"
	"errors"
	"testing"
)

func TestEmptyIsCompleteAndStable(t *testing.T) {
	document := Empty()
	want := []byte(`{"global":{},"nodes":[],"rules":[],"schema_version":1,"subscription":{}}`)
	if got := document.CanonicalJSON(); !bytes.Equal(got, want) {
		t.Fatalf("CanonicalJSON() = %s, want %s", got, want)
	}

	clone := document.Map()
	clone["global"].(map[string]any)["mutated"] = true
	if bytes.Contains(document.CanonicalJSON(), []byte("mutated")) {
		t.Fatal("Map() exposed document-owned state")
	}
}

func TestParsePreservesArrayOrderAndCanonicalizesObjects(t *testing.T) {
	input := []byte(`{
      "subscription": {},
      "rules": [{"enabled":true,"id":"rule-a"},{"id":"rule-b","enabled":false}],
      "nodes": [
        {"kind":"outbound","id":"node-b","enabled":false},
        {"enabled":true,"id":"node-a","kind":"inbound"}
      ],
      "global": {"log":{"level":"warn"}},
      "schema_version": 1
    }`)
	document, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	got := document.CanonicalJSON()
	if bytes.Index(got, []byte("node-b")) > bytes.Index(got, []byte("node-a")) {
		t.Fatalf("entity array order changed: %s", got)
	}
}

func TestParseRejectsAmbiguousOrUnstableDocuments(t *testing.T) {
	tests := map[string]string{
		"duplicate key":    `{"schema_version":1,"global":{},"global":{},"nodes":[],"rules":[],"subscription":{}}`,
		"unknown root":     `{"schema_version":1,"global":{},"nodes":[],"rules":[],"subscription":{},"mystery":1}`,
		"implicit enabled": `{"schema_version":1,"global":{},"nodes":[{"id":"node-a","kind":"outbound"}],"rules":[],"subscription":{}}`,
		"duplicate id":     `{"schema_version":1,"global":{},"nodes":[{"id":"node-a","kind":"outbound","enabled":true},{"id":"node-a","kind":"outbound","enabled":false}],"rules":[],"subscription":{}}`,
		"wrong schema":     `{"schema_version":2,"global":{},"nodes":[],"rules":[],"subscription":{}}`,
	}
	for name, input := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := Parse([]byte(input))
			if !errors.Is(err, ErrInvalidDocument) {
				t.Fatalf("Parse() error = %v, want ErrInvalidDocument", err)
			}
		})
	}
}
