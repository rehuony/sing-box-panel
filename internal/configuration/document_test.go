// SPDX-License-Identifier: GPL-3.0-or-later

package configuration

import (
	"encoding/json"
	"errors"
	"testing"
)

func TestV2DocumentPreservesGlobalIntentAndStripsPanelMetadata(t *testing.T) {
	t.Parallel()

	document, err := ParseV2([]byte(`{
		"schema_version": 2,
		"configuration": {
			"inbounds": [
				{"_panel":{"id":"public","enabled":true},"type":"mixed","tag":"public","listen_port":1080},
				{"_panel":{"id":"disabled","enabled":false},"type":"direct","tag":"disabled"}
			],
			"route": {"rules":[{"_panel":{"id":"private-rule","enabled":false},"action":"reject"}]}
		}
	}`))
	if err != nil {
		t.Fatalf("ParseV2: %v", err)
	}

	configuration := document.Configuration()
	configuration["log"] = map[string]any{"level": "debug"}
	if string(document.CanonicalJSON()) == "" {
		t.Fatal("CanonicalJSON returned empty bytes")
	}

	stripped, err := StripPanelMetadata(document.Configuration())
	if err != nil {
		t.Fatalf("StripPanelMetadata: %v", err)
	}
	encoded, err := json.Marshal(stripped)
	if err != nil {
		t.Fatalf("Marshal stripped configuration: %v", err)
	}
	want := `{"inbounds":[{"listen_port":1080,"tag":"public","type":"mixed"}],"route":{"rules":[]}}`
	if string(encoded) != want {
		t.Fatalf("stripped configuration = %s, want %s", encoded, want)
	}
}

func TestV2DocumentRejectsUnknownEnvelopeAndInvalidManagedEntities(t *testing.T) {
	t.Parallel()

	tests := []string{
		`{"schema_version":2,"configuration":{},"other":true}`,
		`{"schema_version":2,"configuration":{"unknown":{}}}`,
		`{"schema_version":2,"configuration":{"inbounds":[{"type":"mixed"}]}}`,
		`{"schema_version":2,"configuration":{"inbounds":[{"_panel":{"id":"same","enabled":true}},{"_panel":{"id":"same","enabled":false}}]}}`,
		`{"schema_version":2,"configuration":{"outbounds":[{"_panel":{"id":"Bad ID","enabled":true}}]}}`,
		`{"schema_version":2,"configuration":{"services":{}}}`,
	}
	for _, input := range tests {
		input := input
		t.Run(input, func(t *testing.T) {
			t.Parallel()
			if _, err := ParseV2([]byte(input)); !errors.Is(err, ErrInvalidDocument) {
				t.Fatalf("ParseV2 error = %v, want ErrInvalidDocument", err)
			}
		})
	}
}

func TestEmptyV2IsCompleteAndStable(t *testing.T) {
	t.Parallel()

	if got, want := string(EmptyV2().CanonicalJSON()), `{"configuration":{},"schema_version":2}`; got != want {
		t.Fatalf("EmptyV2 = %s, want %s", got, want)
	}
}
