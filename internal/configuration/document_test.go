// SPDX-License-Identifier: GPL-3.0-or-later

package configuration

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestDocumentPreservesRawSingBoxConfiguration(t *testing.T) {
	t.Parallel()

	document, err := Parse([]byte(`{
			"inbounds": [{"type":"mixed","tag":"public","listen_port":1080}],
			"route": {"rules":[{"action":"reject"}]},
			"future_option": {"enabled": true}
		}`))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	copy := document.Configuration()
	copy["log"] = map[string]any{"level": "debug"}
	want := `{"future_option":{"enabled":true},"inbounds":[{"listen_port":1080,"tag":"public","type":"mixed"}],"route":{"rules":[{"action":"reject"}]}}`
	if got := string(document.CanonicalJSON()); got != want {
		t.Fatalf("CanonicalJSON = %s, want %s", got, want)
	}
}

func TestDocumentRejectsNonObjectAndAmbiguousJSON(t *testing.T) {
	t.Parallel()

	tests := []string{
		`[]`,
		`null`,
		`{"duplicate":1,"duplicate":2}`,
	}
	for _, input := range tests {
		input := input
		t.Run(input, func(t *testing.T) {
			t.Parallel()
			if _, err := Parse([]byte(input)); !errors.Is(err, ErrInvalidDocument) {
				t.Fatalf("Parse error = %v, want ErrInvalidDocument", err)
			}
		})
	}
}

func TestEmptyIsCompleteAndStable(t *testing.T) {
	t.Parallel()

	if got, want := string(Empty().CanonicalJSON()), `{}`; got != want {
		t.Fatalf("Empty = %s, want %s", got, want)
	}
}

func TestReleaseConfigurationFixture(t *testing.T) {
	t.Parallel()

	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate configuration test source")
	}
	fixturePath := filepath.Join(filepath.Dir(filename), "..", "..", "scripts", "testdata", "release-configuration.json")
	fixture, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatal(err)
	}
	document, err := Parse(fixture)
	if err != nil {
		t.Fatalf("Parse(release fixture): %v", err)
	}
	log, ok := document.Configuration()["log"].(map[string]any)
	if !ok || log["level"] != "debug" {
		t.Fatalf("release fixture log configuration = %#v", log)
	}
}
