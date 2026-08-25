// SPDX-License-Identifier: GPL-3.0-or-later

// Package openapiverify validates the repository's offline OpenAPI contract.
package openapiverify

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"go.yaml.in/yaml/v3"
)

var arrayIndexPattern = regexp.MustCompile(`^(?:0|[1-9][0-9]*)$`)

var httpMethods = []string{"get", "put", "post", "delete", "options", "head", "patch", "trace"}

// ValidateFile reads and validates one OpenAPI YAML document.
func ValidateFile(path string) []error {
	source, err := os.ReadFile(path)
	if err != nil {
		return []error{fmt.Errorf("read OpenAPI document: %w", err)}
	}
	return Validate(source)
}

// Validate checks the repository's required OpenAPI structural invariants.
func Validate(source []byte) []error {
	root, err := decodeSingleDocument(source)
	if err != nil {
		return []error{err}
	}
	if findings := inspectYAMLSyntax(root, nil, nil); len(findings) != 0 {
		return findings
	}

	var document any
	if err := root.Decode(&document); err != nil {
		return []error{fmt.Errorf("decode OpenAPI YAML: %w", err)}
	}
	return validateDocument(document)
}

func decodeSingleDocument(source []byte) (*yaml.Node, error) {
	decoder := yaml.NewDecoder(bytes.NewReader(source))
	var root yaml.Node
	if err := decoder.Decode(&root); err != nil {
		if errors.Is(err, io.EOF) {
			return nil, errors.New("OpenAPI document is empty")
		}
		return nil, fmt.Errorf("parse OpenAPI YAML: %w", err)
	}

	var extra yaml.Node
	if err := decoder.Decode(&extra); err == nil {
		return nil, errors.New("OpenAPI input must contain exactly one YAML document")
	} else if !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("parse trailing OpenAPI YAML: %w", err)
	}
	return &root, nil
}

func inspectYAMLSyntax(node *yaml.Node, path []any, findings []error) []error {
	switch node.Kind {
	case yaml.DocumentNode:
		for _, child := range node.Content {
			findings = inspectYAMLSyntax(child, path, findings)
		}
	case yaml.SequenceNode:
		for index, child := range node.Content {
			findings = inspectYAMLSyntax(child, appendPath(path, index), findings)
		}
	case yaml.MappingNode:
		seen := make(map[string]int)
		for index := 0; index+1 < len(node.Content); index += 2 {
			keyNode := node.Content[index]
			valueNode := node.Content[index+1]
			if keyNode.Kind != yaml.ScalarNode {
				findings = append(findings, fmt.Errorf(
					"%s: mapping keys must be scalars (line %d)", location(path), keyNode.Line,
				))
				continue
			}
			key := keyNode.Value
			if firstLine, ok := seen[key]; ok {
				findings = append(findings, fmt.Errorf(
					"%s: duplicate key on lines %d and %d", location(appendPath(path, key)), firstLine, keyNode.Line,
				))
			} else {
				seen[key] = keyNode.Line
			}
			findings = inspectYAMLSyntax(valueNode, appendPath(path, key), findings)
		}
	case yaml.AliasNode:
		findings = append(findings, fmt.Errorf("%s: YAML aliases are not allowed (line %d)", location(path), node.Line))
	}
	return findings
}

func validateDocument(value any) []error {
	document, ok := value.(map[string]any)
	if !ok {
		return []error{errors.New("$: OpenAPI document must be a mapping")}
	}

	var findings []error
	if _, ok := document["openapi"].(string); !ok {
		findings = append(findings, errors.New(`$["openapi"]: missing OpenAPI version`))
	}
	paths, pathsOK := document["paths"].(map[string]any)
	if !pathsOK {
		findings = append(findings, errors.New(`$["paths"]: must be a mapping`))
	} else {
		findings = append(findings, validateOperations(paths)...)
	}
	return walkReferences(document, document, nil, findings)
}

func validateOperations(paths map[string]any) []error {
	operationIDs := make(map[string]string)
	var findings []error
	for _, route := range sortedKeys(paths) {
		pathItem, ok := paths[route].(map[string]any)
		if !ok {
			findings = append(findings, fmt.Errorf("%s: path item must be a mapping", location([]any{"paths", route})))
			continue
		}
		for _, method := range httpMethods {
			operationValue, exists := pathItem[method]
			if !exists {
				continue
			}
			operationPath := []any{"paths", route, method}
			operation, ok := operationValue.(map[string]any)
			if !ok {
				findings = append(findings, fmt.Errorf("%s: operation must be a mapping", location(operationPath)))
				continue
			}
			operationID, ok := operation["operationId"].(string)
			if !ok || operationID == "" {
				findings = append(findings, fmt.Errorf(
					"%s: missing non-empty operationId", location(appendPath(operationPath, "operationId")),
				))
				continue
			}
			operationIDPath := location(appendPath(operationPath, "operationId"))
			if firstPath, exists := operationIDs[operationID]; exists {
				findings = append(findings, fmt.Errorf(
					"%s: duplicate operationId %q; first used at %s", operationIDPath, operationID, firstPath,
				))
			} else {
				operationIDs[operationID] = operationIDPath
			}
		}
	}
	return findings
}

func walkReferences(document, value any, path []any, findings []error) []error {
	switch typed := value.(type) {
	case map[string]any:
		if referenceValue, exists := typed["$ref"]; exists {
			reference, ok := referenceValue.(string)
			switch {
			case !ok || reference == "":
				findings = append(findings, fmt.Errorf(
					"%s: reference must be a non-empty string", location(appendPath(path, "$ref")),
				))
			case !strings.HasPrefix(reference, "#"):
				findings = append(findings, fmt.Errorf(
					"%s: external reference %q cannot be validated offline", location(appendPath(path, "$ref")), reference,
				))
			default:
				if _, found := resolveJSONPointer(document, reference); !found {
					findings = append(findings, fmt.Errorf(
						"%s: unresolved reference %q", location(appendPath(path, "$ref")), reference,
					))
				}
			}
		}
		for _, key := range sortedKeys(typed) {
			findings = walkReferences(document, typed[key], appendPath(path, key), findings)
		}
	case []any:
		for index, child := range typed {
			findings = walkReferences(document, child, appendPath(path, index), findings)
		}
	}
	return findings
}

func resolveJSONPointer(document any, reference string) (any, bool) {
	if reference == "#" {
		return document, document != nil
	}
	if !strings.HasPrefix(reference, "#/") {
		return nil, false
	}

	cursor := document
	for _, encoded := range strings.Split(strings.TrimPrefix(reference, "#/"), "/") {
		token := strings.ReplaceAll(strings.ReplaceAll(encoded, "~1", "/"), "~0", "~")
		switch typed := cursor.(type) {
		case map[string]any:
			var ok bool
			cursor, ok = typed[token]
			if !ok {
				return nil, false
			}
		case []any:
			if !arrayIndexPattern.MatchString(token) {
				return nil, false
			}
			index, err := strconv.Atoi(token)
			if err != nil || index >= len(typed) {
				return nil, false
			}
			cursor = typed[index]
		default:
			return nil, false
		}
	}
	return cursor, cursor != nil
}

func sortedKeys(value map[string]any) []string {
	keys := make([]string, 0, len(value))
	for key := range value {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	return keys
}

func appendPath(path []any, part any) []any {
	result := slices.Clone(path)
	return append(result, part)
}

func location(path []any) string {
	var result strings.Builder
	result.WriteByte('$')
	for _, part := range path {
		switch typed := part.(type) {
		case int:
			fmt.Fprintf(&result, "[%d]", typed)
		case string:
			result.WriteByte('[')
			result.WriteString(strconv.Quote(typed))
			result.WriteByte(']')
		}
	}
	return result.String()
}
