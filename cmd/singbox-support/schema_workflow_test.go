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

func TestCatalogDistinguishesNativeAndReviewedSchemaSources(t *testing.T) {
	root, err := repositoryRoot()
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := loadCatalog(filepath.Join(root, catalogPath))
	if err != nil {
		t.Fatal(err)
	}
	var schemaVersions, reviewedVersions []string
	for _, version := range catalog.Versions {
		if version.SchemaSource == singbox.SchemaSourceReviewed113 {
			reviewedVersions = append(reviewedVersions, version.ExactVersion)
		}
		if singbox.SupportsNativeConfigurationSchema(version.ExactVersion) {
			schemaVersions = append(schemaVersions, version.ExactVersion)
		}
	}
	if !reflect.DeepEqual(reviewedVersions, []string{"1.13.19", "1.13.20", "1.13.21"}) {
		t.Fatalf("reviewed versions = %v", reviewedVersions)
	}
	want := []string{"1.14.0", "1.14.1", "1.14.2"}
	if !reflect.DeepEqual(schemaVersions, want) {
		t.Fatalf("native schema versions = %v, want %v", schemaVersions, want)
	}
}
