// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

const reviewedSchemaFieldsPath = "cmd/singbox-support/schema-sources/1.13-fields.csv"

// Each shared definition is expanded once. Subsequent uses name the definition
// in the shape column, so recursive rules do not produce an infinite table.
func reviewedSchemaFields(root string) ([]byte, error) {
	raw, err := os.ReadFile(filepath.Join(root, "cmd/singbox-support/schema-sources/1.13.json"))
	if err != nil {
		return nil, err
	}
	var schema map[string]any
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&schema); err != nil {
		return nil, err
	}
	var output bytes.Buffer
	writer := csv.NewWriter(&output)
	if err := writer.Write([]string{"configuration_path", "shape_and_values", "optional", "constraints", "source"}); err != nil {
		return nil, err
	}
	definitions, _ := schema["$defs"].(map[string]any)
	seen := make(map[string]bool)
	var walk func(map[string]any, string, string) error
	walk = func(node map[string]any, path, source string) error {
		if reference, ok := node["$ref"].(string); ok {
			if seen[reference] {
				return nil
			}
			seen[reference] = true
			definition, ok := definitions[strings.TrimPrefix(reference, "#/$defs/")].(map[string]any)
			if !ok {
				return fmt.Errorf("unresolved reviewed field reference %s", reference)
			}
			return walk(definition, path+" ("+strings.TrimPrefix(reference, "#/$defs/")+")", source)
		}
		if comment, ok := node["$comment"].(string); ok && strings.HasPrefix(comment, "option/") {
			source = comment
		}
		properties, _ := node["properties"].(map[string]any)
		required, _ := node["required"].([]any)
		for _, name := range slices.Sorted(maps.Keys(properties)) {
			property, ok := properties[name].(map[string]any)
			if !ok {
				return fmt.Errorf("non-object reviewed field %s.%s", path, name)
			}
			fieldSource := source
			if comment, ok := property["$comment"].(string); ok {
				fieldSource = comment
			}
			shape := maps.Clone(property)
			delete(shape, "$comment")
			delete(shape, "description")
			encoded, err := json.Marshal(shape)
			if err != nil {
				return err
			}
			optional := "yes; conditional core requirements still apply"
			if slices.Contains(required, any(name)) {
				optional = "no in this branch"
			}
			constraints, _ := property["description"].(string)
			if restriction, exists := node["not"]; exists {
				encodedRestriction, _ := json.Marshal(restriction)
				constraints += " Parent forbids: " + string(encodedRestriction)
			}
			if err := writer.Write([]string{path + "." + name, string(encoded), optional, constraints, "https://github.com/SagerNet/sing-box/blob/v1.13.21/" + fieldSource}); err != nil {
				return err
			}
			if err := walk(property, path+"."+name, fieldSource); err != nil {
				return err
			}
		}
		if item, ok := node["items"].(map[string]any); ok {
			if err := walk(item, path+"[]", source); err != nil {
				return err
			}
		}
		if item, ok := node["additionalProperties"].(map[string]any); ok {
			if err := walk(item, path+".*", source); err != nil {
				return err
			}
		}
		for _, keyword := range []string{"allOf", "oneOf", "anyOf"} {
			branches, _ := node[keyword].([]any)
			for index, branch := range branches {
				if child, ok := branch.(map[string]any); ok {
					if err := walk(child, fmt.Sprintf("%s{%s:%d}", path, keyword, index), source); err != nil {
						return err
					}
				}
			}
		}
		return nil
	}
	if err := walk(schema, "$", "include/registry.go"); err != nil {
		return nil, err
	}
	writer.Flush()
	return output.Bytes(), writer.Error()
}
