// SPDX-License-Identifier: GPL-3.0-or-later

package singbox

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"testing/fstest"
)

func TestConfigurationSchemaIsSelectedOnlyByExactVersion(t *testing.T) {
	for _, version := range Versions() {
		if version.SchemaSource == "" {
			continue
		}
		exactVersion := version.ExactVersion
		t.Run(exactVersion, func(t *testing.T) {
			contract, err := ConfigurationSchema(exactVersion)
			if err != nil {
				t.Fatalf("ConfigurationSchema(%s) error = %v", exactVersion, err)
			}
			digest := sha256.Sum256(contract.Schema)
			if hex.EncodeToString(digest[:]) != contract.SchemaSHA256 {
				t.Fatal("contract digest differs from content")
			}
			var schema any
			if err := json.Unmarshal(contract.Schema, &schema); err != nil {
				t.Fatal(err)
			}
			assertLocalSchemaReferences(t, schema)
		})
	}
	for _, exactVersion := range []string{"1.11.15", "1.12.25", "1.13.18", "1.13.22", "99.0.0", "1.14", "v1.14.1", "1.14.1-beta.1", "invalid"} {
		if _, err := ConfigurationSchema(exactVersion); !errors.Is(err, ErrConfigurationSchemaUnavailable) {
			t.Fatalf("ConfigurationSchema(%s) error = %v, want ErrSchemaUnavailable", exactVersion, err)
		}
	}
}

func TestConfigurationSchemaWebAssetsContainOnlyManifestAndFormSchema(t *testing.T) {
	assets, err := ConfigurationSchemaWebAssets()
	if err != nil {
		t.Fatal(err)
	}
	var manifest schemaManifest
	if err := json.Unmarshal(assets["manifest.json"], &manifest); err != nil {
		t.Fatal(err)
	}
	if len(manifest.Entries) == 0 || len(assets) != len(manifest.Entries)+1 {
		t.Fatalf("Web schema assets = %d, manifest entries = %d", len(assets), len(manifest.Entries))
	}
	for _, entry := range manifest.Entries {
		if len(assets[entry.SchemaFile]) == 0 {
			t.Fatalf("missing Web schema asset %q", entry.SchemaFile)
		}
	}
}

func TestConfigurationSchemaManifestFailsClosed(t *testing.T) {
	validHeader := `{"schema_version":1,"entries":[]}`
	tests := []struct {
		name   string
		assets fstest.MapFS
		match  string
	}{
		{name: "missing manifest", assets: fstest.MapFS{}, match: "read schema manifest"},
		{name: "missing native version", assets: fstest.MapFS{
			"schemas/manifest.json": &fstest.MapFile{Data: []byte(validHeader)},
		}, match: "schema catalog requires"},
		{name: "unknown field", assets: fstest.MapFS{
			"schemas/manifest.json": &fstest.MapFile{Data: []byte(`{"schema_version":1,"entries":[],"unknown":true}`)},
		}, match: "unknown field"},
		{name: "multiple values", assets: fstest.MapFS{
			"schemas/manifest.json": &fstest.MapFile{Data: []byte(validHeader + `{}`)},
		}, match: "multiple values"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := readConfigurationSchemasFromFS(test.assets); err == nil || !strings.Contains(err.Error(), test.match) {
				t.Fatalf("readConfigurationSchemasFromFS() error = %v, want %q", err, test.match)
			}
		})
	}
}

func TestValidateConfigurationIsOptionalBeforeNativeSchema(t *testing.T) {
	if err := ValidateConfiguration("1.13.18", []byte(`{}`)); !errors.Is(err, ErrConfigurationSchemaUnavailable) {
		t.Fatalf("ValidateConfiguration(1.13.18) error = %v", err)
	}
	for _, version := range Versions() {
		if version.SchemaSource == "" {
			continue
		}
		exactVersion := version.ExactVersion
		t.Run(exactVersion, func(t *testing.T) {
			if err := ValidateConfiguration(exactVersion, []byte(`{}`)); err != nil {
				t.Fatalf("ValidateConfiguration(%s) error = %v", exactVersion, err)
			}
			if err := ValidateConfiguration(exactVersion, []byte(`[]`)); err == nil || errors.Is(err, ErrConfigurationSchemaUnavailable) {
				t.Fatalf("ValidateConfiguration(%s, array) error = %v", exactVersion, err)
			}
		})
	}
}

func assertLocalSchemaReferences(t *testing.T, value any) {
	t.Helper()
	switch typed := value.(type) {
	case []any:
		for _, child := range typed {
			assertLocalSchemaReferences(t, child)
		}
	case map[string]any:
		for key, child := range typed {
			if key == "$ref" {
				reference, ok := child.(string)
				if !ok || !strings.HasPrefix(reference, "#/") {
					t.Fatalf("schema contains external reference %#v", child)
				}
			}
			assertLocalSchemaReferences(t, child)
		}
	}
}
