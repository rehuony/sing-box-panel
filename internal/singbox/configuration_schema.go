// SPDX-License-Identifier: GPL-3.0-or-later

package singbox

import (
	"bytes"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path"
	"strings"
	"sync"

	"github.com/rehuony/sing-box-panel/internal/coreartifact"
	jsonschema "github.com/santhosh-tekuri/jsonschema/v6"
)

var ErrConfigurationSchemaUnavailable = errors.New("configuration schema is unavailable")

//go:embed all:schemas
var schemaAssets embed.FS

// SchemaContract is the canonical schema committed for one exact sing-box version.
// SchemaSHA256 is the only content identity exposed to API and Web consumers.
type SchemaContract struct {
	SchemaSHA256 string          `json:"schema_sha256"`
	Schema       json.RawMessage `json:"schema"`
}

type schemaManifest struct {
	SchemaVersion int                   `json:"schema_version"`
	Entries       []schemaManifestEntry `json:"entries"`
}

type schemaManifestEntry struct {
	ExactVersion string `json:"exact_version"`
	SchemaSHA256 string `json:"schema_sha256"`
	SchemaFile   string `json:"schema_file"`
	Source       string `json:"source"`
}

type compiledSchema struct {
	entry     schemaManifestEntry
	schema    json.RawMessage
	validator *jsonschema.Schema
}

var (
	configurationSchemasOnce sync.Once
	configurationSchemas     map[string]compiledSchema
	configurationSchemasErr  error
)

// SupportsNativeConfigurationSchema reports whether an exact version belongs
// to a release line with the upstream `sing-box schema` command.
func SupportsNativeConfigurationSchema(exactVersion string) bool {
	parsed, err := coreartifact.ParseExactVersion(strings.TrimSpace(exactVersion))
	if err != nil || parsed.IsZero() || parsed.String() != exactVersion {
		return false
	}
	return parsed.Major() > 1 || parsed.Major() == 1 && parsed.Minor() >= 14
}

// SupportsConfigurationSchema reports explicitly cataloged exact-version support.
func SupportsConfigurationSchema(exactVersion string) bool {
	version, found := Lookup(exactVersion)
	return found && version.SchemaSource != ""
}

// ConfigurationSchema resolves a committed canonical schema by exact version only.
func ConfigurationSchema(exactVersion string) (SchemaContract, error) {
	compiled, err := configurationSchema(exactVersion)
	if err != nil {
		return SchemaContract{}, err
	}
	return SchemaContract{
		SchemaSHA256: compiled.entry.SchemaSHA256,
		Schema:       append(json.RawMessage(nil), compiled.schema...),
	}, nil
}

// ValidateConfiguration applies the committed schema for reviewed exact versions. The exact binary's `sing-box check` remains authoritative.
func ValidateConfiguration(exactVersion string, raw []byte) error {
	compiled, err := configurationSchema(exactVersion)
	if err != nil {
		return err
	}
	var document any
	if err := decodeJSONPreservingNumbers(raw, &document); err != nil {
		return fmt.Errorf("decode sing-box configuration: %w", err)
	}
	if err := compiled.validator.Validate(document); err != nil {
		return fmt.Errorf("configuration violates sing-box %s schema: %w", exactVersion, err)
	}
	return nil
}

func configurationSchema(exactVersion string) (compiledSchema, error) {
	if _, found := Lookup(exactVersion); !found || !SupportsConfigurationSchema(exactVersion) {
		return compiledSchema{}, fmt.Errorf("%w: sing-box %s", ErrConfigurationSchemaUnavailable, exactVersion)
	}
	schemas, err := loadConfigurationSchemas()
	if err != nil {
		return compiledSchema{}, err
	}
	compiled, found := schemas[exactVersion]
	if !found {
		return compiledSchema{}, fmt.Errorf("configuration schema is missing for sing-box %s", exactVersion)
	}
	return compiled, nil
}

func loadConfigurationSchemas() (map[string]compiledSchema, error) {
	configurationSchemasOnce.Do(func() {
		configurationSchemas, configurationSchemasErr = readConfigurationSchemasFromFS(schemaAssets)
	})
	return configurationSchemas, configurationSchemasErr
}

// ValidateConfigurationSchemaAssets verifies the exact-version manifest,
// content digests, local references, and JSON Schema compilation.
func ValidateConfigurationSchemaAssets() error {
	_, err := loadConfigurationSchemas()
	return err
}

func ConfigurationSchemaWebAssets() (map[string][]byte, error) {
	schemas, err := loadConfigurationSchemas()
	if err != nil {
		return nil, err
	}
	manifest, err := schemaAssets.ReadFile("schemas/manifest.json")
	if err != nil {
		return nil, err
	}
	result := map[string][]byte{"manifest.json": append([]byte(nil), manifest...)}
	for _, compiled := range schemas {
		result[compiled.entry.SchemaFile] = append([]byte(nil), compiled.schema...)
	}
	return result, nil
}

func readConfigurationSchemasFromFS(assets fs.FS) (map[string]compiledSchema, error) {
	rawManifest, err := fs.ReadFile(assets, "schemas/manifest.json")
	if err != nil {
		return nil, fmt.Errorf("read schema manifest: %w", err)
	}
	var manifest schemaManifest
	if err := decodeStrictJSON(rawManifest, &manifest); err != nil {
		return nil, fmt.Errorf("decode schema manifest: %w", err)
	}
	if manifest.SchemaVersion != 1 {
		return nil, fmt.Errorf("unsupported schema manifest version %d", manifest.SchemaVersion)
	}
	schemaVersions := 0
	for _, version := range generatedVersions {
		if version.SchemaSource != "" {
			schemaVersions++
		}
	}
	if len(manifest.Entries) != schemaVersions {
		return nil, fmt.Errorf("schema manifest has %d entries; schema catalog requires %d", len(manifest.Entries), schemaVersions)
	}

	result := make(map[string]compiledSchema, len(manifest.Entries))
	for index, entry := range manifest.Entries {
		if err := validateSchemaManifestEntry(entry); err != nil {
			return nil, fmt.Errorf("schema manifest entry %d: %w", index, err)
		}
		if _, duplicate := result[entry.ExactVersion]; duplicate {
			return nil, fmt.Errorf("schema manifest contains duplicate version %s", entry.ExactVersion)
		}
		rawSchema, err := fs.ReadFile(assets, path.Join("schemas", entry.SchemaFile))
		if err != nil {
			return nil, fmt.Errorf("read schema %s: %w", entry.ExactVersion, err)
		}
		if digestHex(rawSchema) != entry.SchemaSHA256 {
			return nil, fmt.Errorf("schema %s content digest differs from the manifest", entry.ExactVersion)
		}
		validator, err := compileSchema(entry.ExactVersion, rawSchema)
		if err != nil {
			return nil, err
		}
		result[entry.ExactVersion] = compiledSchema{entry: entry, schema: rawSchema, validator: validator}
	}
	return result, nil
}

func validateSchemaManifestEntry(entry schemaManifestEntry) error {
	version, found := Lookup(entry.ExactVersion)
	if !found || version.SchemaSource == "" || version.SchemaSource != entry.Source {
		return fmt.Errorf("version %q has an unreviewed schema source %q", entry.ExactVersion, entry.Source)
	}
	digest, err := coreartifact.ParseSHA256(entry.SchemaSHA256)
	if err != nil || digest.String() != entry.SchemaSHA256 {
		return errors.New("schema digest is not a lowercase SHA-256")
	}
	if path.Clean(entry.SchemaFile) != entry.SchemaFile || path.IsAbs(entry.SchemaFile) ||
		path.Ext(entry.SchemaFile) != ".json" || path.Base(entry.SchemaFile) != entry.SchemaFile {
		return errors.New("schema file is not a safe root JSON path")
	}
	return nil
}

func compileSchema(exactVersion string, raw []byte) (*jsonschema.Schema, error) {
	var document any
	if err := decodeJSONPreservingNumbers(raw, &document); err != nil {
		return nil, fmt.Errorf("decode schema %s: %w", exactVersion, err)
	}
	if err := rejectExternalSchemaReferences(document); err != nil {
		return nil, fmt.Errorf("schema %s: %w", exactVersion, err)
	}
	resource := "urn:sing-box-panel:configuration-schema:" + exactVersion
	compiler := jsonschema.NewCompiler()
	compiler.DefaultDraft(jsonschema.Draft2020)
	if err := compiler.AddResource(resource, document); err != nil {
		return nil, fmt.Errorf("add schema %s: %w", exactVersion, err)
	}
	compiled, err := compiler.Compile(resource)
	if err != nil {
		return nil, fmt.Errorf("compile schema %s: %w", exactVersion, err)
	}
	return compiled, nil
}

func rejectExternalSchemaReferences(value any) error {
	switch typed := value.(type) {
	case []any:
		for _, child := range typed {
			if err := rejectExternalSchemaReferences(child); err != nil {
				return err
			}
		}
	case map[string]any:
		for key, child := range typed {
			if key == "$ref" {
				reference, ok := child.(string)
				if !ok || !strings.HasPrefix(reference, "#") {
					return fmt.Errorf("external schema reference %#v is forbidden", child)
				}
			}
			if err := rejectExternalSchemaReferences(child); err != nil {
				return err
			}
		}
	}
	return nil
}

func decodeStrictJSON(raw []byte, destination any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	return requireJSONEOF(decoder)
}

func decodeJSONPreservingNumbers(raw []byte, destination any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	return requireJSONEOF(decoder)
}

func requireJSONEOF(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("JSON contains multiple values")
		}
		return err
	}
	return nil
}

func digestHex(content []byte) string {
	digest := sha256.Sum256(content)
	return hex.EncodeToString(digest[:])
}
