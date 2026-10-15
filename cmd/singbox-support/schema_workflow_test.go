// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/rehuony/sing-box-panel/internal/singbox"
)

func TestConfigurationSchemaAssetsShareOnlyIdenticalFinalContent(t *testing.T) {
	output := t.TempDir()
	raw := []byte(`{"type":"object","properties":{"log":{"type":"object"}}}`)
	first, err := writeConfigurationSchema(singbox.Version{
		ExactVersion: "1.13.19", SchemaSource: singbox.SchemaSourceReviewed113,
	}, raw, output)
	if err != nil {
		t.Fatal(err)
	}
	second, err := writeConfigurationSchema(singbox.Version{
		ExactVersion: "1.14.0", SchemaSource: singbox.SchemaSourceNative,
	}, []byte(`{ "properties": { "log": { "type": "object" } }, "type": "object" }`), output)
	if err != nil {
		t.Fatal(err)
	}
	if first.SchemaFile != second.SchemaFile || first.SchemaSHA256 != second.SchemaSHA256 {
		t.Fatal("identical final schemas did not share one asset")
	}
	if first.ExactVersion != "1.13.19" || first.Source != singbox.SchemaSourceReviewed113 ||
		second.ExactVersion != "1.14.0" || second.Source != singbox.SchemaSourceNative {
		t.Fatal("shared asset lost its exact-version provenance")
	}
	files, err := os.ReadDir(output)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 {
		t.Fatalf("identical schemas produced %d files, want 1", len(files))
	}
	content, err := os.ReadFile(filepath.Join(output, first.SchemaFile))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), `"x-panel"`) || first.SchemaSHA256 == digestHex(raw) ||
		first.SchemaSHA256 != digestHex(content) || first.SchemaFile != "schema-"+digestHex(content)+".json" {
		t.Fatal("asset identity does not bind the final overlaid schema")
	}
	changed, err := writeConfigurationSchema(singbox.Version{
		ExactVersion: "1.14.1", SchemaSource: singbox.SchemaSourceNative,
	}, []byte(`{"type":"object","description":"Updated help","properties":{"log":{"type":"object"}}}`), output)
	if err != nil {
		t.Fatal(err)
	}
	if changed.SchemaSHA256 == first.SchemaSHA256 || changed.SchemaFile == first.SchemaFile {
		t.Fatal("changed schema metadata reused an existing asset")
	}
	files, err = os.ReadDir(output)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 2 {
		t.Fatalf("distinct final schemas produced %d files, want 2", len(files))
	}
}

func TestCanonicalJSONFilePreservesLargeIntegers(t *testing.T) {
	filename := filepath.Join(t.TempDir(), "schema.json")
	if err := os.WriteFile(filename, []byte("{\n  \"maximum\": 9007199254740993\n}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	canonical, err := canonicalJSONFile(filename)
	if err != nil {
		t.Fatal(err)
	}
	if string(canonical) != `{"maximum":9007199254740993}` {
		t.Fatalf("canonical JSON = %s", canonical)
	}
}

func TestPresentationOverlayAddsOnlyXPanelAnnotations(t *testing.T) {
	upstreamDocument := map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties": map[string]any{
			"log": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"token": map[string]any{"type": "string", "minLength": float64(8)},
				},
			},
			"inbounds": map[string]any{
				"type":  "array",
				"items": map[string]any{"type": "object"},
			},
		},
	}
	upstream, err := json.Marshal(upstreamDocument)
	if err != nil {
		t.Fatal(err)
	}
	schema, err := applyConfigurationSchemaPresentationOverlay(upstream)
	if err != nil {
		t.Fatal(err)
	}
	var overlaid any
	if err := json.Unmarshal(schema, &overlaid); err != nil {
		t.Fatal(err)
	}
	if stripped := stripXPanel(overlaid); !reflect.DeepEqual(stripped, upstreamDocument) {
		t.Fatalf("overlay changed upstream constraints\ngot:  %#v\nwant: %#v", stripped, upstreamDocument)
	}
	encoded := string(schema)
	if !strings.Contains(encoded, `"identity-field":"tag"`) {
		t.Fatalf("collection annotation does not use sing-box tag identity: %s", encoded)
	}
}

func stripXPanel(value any) any {
	switch typed := value.(type) {
	case []any:
		result := make([]any, len(typed))
		for index, child := range typed {
			result[index] = stripXPanel(child)
		}
		return result
	case map[string]any:
		result := make(map[string]any, len(typed))
		for key, child := range typed {
			if key != "x-panel" {
				result[key] = stripXPanel(child)
			}
		}
		return result
	default:
		return value
	}
}
