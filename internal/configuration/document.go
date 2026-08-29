// SPDX-License-Identifier: GPL-3.0-or-later

// Package configuration owns canonical configuration documents and the
// compiled adapter contracts that project them for exact sing-box versions.
package configuration

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/rehuony/sing-box-panel/internal/jsonstrict"
)

const (
	SchemaVersion     = 2
	MaximumBytes      = 2 << 20
	maximumDepth      = 32
	maximumValues     = 100_000
	maximumEntities   = 10_000
	maximumIdentifier = 128
)

var ErrInvalidDocument = errors.New("invalid canonical document")

var (
	rootKeys = map[string]struct{}{
		"schema_version": {},
		"configuration":  {},
	}
	configurationKeys = map[string]configurationValueKind{
		"log":          configurationObject,
		"dns":          configurationObject,
		"ntp":          configurationObject,
		"certificate":  configurationObject,
		"endpoints":    configurationObjectArray,
		"inbounds":     configurationObjectArray,
		"outbounds":    configurationObjectArray,
		"route":        configurationObject,
		"services":     configurationObjectArray,
		"experimental": configurationObject,
	}
	panelIdentifierPattern = regexp.MustCompile(`^[a-z][a-z0-9._-]*$`)
)

type configurationValueKind uint8

const (
	configurationObject configurationValueKind = iota + 1
	configurationObjectArray
)

// Document is one immutable, version-independent configuration revision.
// It stores the superset intent; exact-version adapters produce executable
// sing-box JSON without mutating these bytes.
type Document struct {
	canonical []byte
}

// Empty returns the global zero configuration. Selecting another core does
// not create or modify this document.
func Empty() *Document {
	document, err := Parse([]byte(`{"schema_version":2,"configuration":{}}`))
	if err != nil {
		panic(err)
	}
	return document
}

// Parse strictly validates the global envelope and every panel-owned
// management marker. Version adapters and the exact sing-box checker validate
// fields owned by a particular sing-box release.
func Parse(data []byte) (*Document, error) {
	var root map[string]any
	if err := jsonstrict.Decode(data, MaximumBytes, &root); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidDocument, err)
	}
	if root == nil {
		return nil, fmt.Errorf("%w: root must be an object", ErrInvalidDocument)
	}
	for key := range root {
		if _, allowed := rootKeys[key]; !allowed {
			return nil, fmt.Errorf("%w: unknown root field %q", ErrInvalidDocument, key)
		}
	}
	version, ok := root["schema_version"].(json.Number)
	if !ok || version.String() != "2" {
		return nil, fmt.Errorf("%w: schema_version must be exactly %d", ErrInvalidDocument, SchemaVersion)
	}
	configuration, ok := root["configuration"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%w: configuration must be an object", ErrInvalidDocument)
	}
	if err := validateConfiguration(configuration); err != nil {
		return nil, err
	}
	values := 0
	if err := validateShape(root, 0, &values); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidDocument, err)
	}
	if err := validatePanelMarkers(root, false); err != nil {
		return nil, err
	}
	canonicalJSON, err := json.Marshal(root)
	if err != nil {
		return nil, fmt.Errorf("%w: encode deterministic JSON: %v", ErrInvalidDocument, err)
	}
	return &Document{canonical: canonicalJSON}, nil
}

func validateConfiguration(configuration map[string]any) error {
	for key, value := range configuration {
		kind, allowed := configurationKeys[key]
		if !allowed {
			return fmt.Errorf("%w: unknown configuration field %q", ErrInvalidDocument, key)
		}
		switch kind {
		case configurationObject:
			if _, ok := value.(map[string]any); !ok {
				return fmt.Errorf("%w: configuration.%s must be an object", ErrInvalidDocument, key)
			}
		case configurationObjectArray:
			items, ok := value.([]any)
			if !ok {
				return fmt.Errorf("%w: configuration.%s must be an array", ErrInvalidDocument, key)
			}
			if len(items) > maximumEntities {
				return fmt.Errorf("%w: configuration.%s exceeds %d entities", ErrInvalidDocument, key, maximumEntities)
			}
			identifiers := make(map[string]struct{}, len(items))
			for index, item := range items {
				object, ok := item.(map[string]any)
				if !ok {
					return fmt.Errorf("%w: configuration.%s[%d] must be an object", ErrInvalidDocument, key, index)
				}
				identifier, err := requiredPanelIdentifier(object)
				if err != nil {
					return fmt.Errorf("%w: configuration.%s[%d]: %v", ErrInvalidDocument, key, index, err)
				}
				if _, duplicate := identifiers[identifier]; duplicate {
					return fmt.Errorf("%w: configuration.%s contains duplicate panel id %q", ErrInvalidDocument, key, identifier)
				}
				identifiers[identifier] = struct{}{}
			}
		}
	}
	return nil
}

func requiredPanelIdentifier(object map[string]any) (string, error) {
	marker, ok := object["_panel"].(map[string]any)
	if !ok {
		return "", errors.New("_panel must be an object")
	}
	if len(marker) != 2 {
		return "", errors.New("_panel must contain exactly id and enabled")
	}
	identifier, ok := marker["id"].(string)
	if !ok || len(identifier) > maximumIdentifier || !panelIdentifierPattern.MatchString(identifier) {
		return "", errors.New("_panel.id is invalid")
	}
	if _, ok := marker["enabled"].(bool); !ok {
		return "", errors.New("_panel.enabled must be explicitly true or false")
	}
	return identifier, nil
}

func validatePanelMarkers(value any, arrayItem bool) error {
	switch typed := value.(type) {
	case map[string]any:
		if marker, exists := typed["_panel"]; exists {
			if _, ok := marker.(map[string]any); !ok {
				return fmt.Errorf("%w: _panel must be an object", ErrInvalidDocument)
			}
			if _, err := requiredPanelIdentifier(typed); err != nil {
				return fmt.Errorf("%w: %v", ErrInvalidDocument, err)
			}
		} else if arrayItem {
			// Only top-level managed collections require a marker. Nested native
			// arrays may contain protocol-owned objects without panel identity.
			arrayItem = false
		}
		for key, child := range typed {
			if strings.ContainsRune(key, '\x00') {
				return fmt.Errorf("%w: object key contains a NUL byte", ErrInvalidDocument)
			}
			if key == "_panel" {
				continue
			}
			if err := validatePanelMarkers(child, false); err != nil {
				return err
			}
		}
	case []any:
		for _, child := range typed {
			if err := validatePanelMarkers(child, true); err != nil {
				return err
			}
		}
	}
	return nil
}

func (document *Document) CanonicalJSON() []byte {
	if document == nil {
		return nil
	}
	return bytes.Clone(document.canonical)
}

// Configuration returns a defensive copy of the version-independent sing-box
// configuration object.
func (document *Document) Configuration() map[string]any {
	if document == nil {
		return nil
	}
	var root struct {
		Configuration map[string]any `json:"configuration"`
	}
	decoder := json.NewDecoder(bytes.NewReader(document.canonical))
	decoder.UseNumber()
	if err := decoder.Decode(&root); err != nil {
		panic(err)
	}
	return root.Configuration
}

func decodeCanonicalMap(raw []byte, target *map[string]any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	return decoder.Decode(target)
}

func encodeCanonicalMap(value map[string]any) ([]byte, error) {
	return json.Marshal(value)
}

// StripPanelMetadata removes all panel-owned markers and disabled array
// entries from a caller-owned configuration tree.
func StripPanelMetadata(configuration map[string]any) (map[string]any, error) {
	stripped, included, err := stripPanelValue(configuration, false)
	if err != nil {
		return nil, err
	}
	if !included {
		return nil, fmt.Errorf("%w: configuration root cannot be disabled", ErrInvalidDocument)
	}
	result, ok := stripped.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%w: configuration root must be an object", ErrInvalidDocument)
	}
	return result, nil
}

func stripPanelValue(value any, arrayItem bool) (any, bool, error) {
	switch typed := value.(type) {
	case map[string]any:
		if markerValue, exists := typed["_panel"]; exists {
			marker, ok := markerValue.(map[string]any)
			if !ok {
				return nil, false, fmt.Errorf("%w: _panel must be an object", ErrInvalidDocument)
			}
			enabled, ok := marker["enabled"].(bool)
			if !ok {
				return nil, false, fmt.Errorf("%w: _panel.enabled must be a boolean", ErrInvalidDocument)
			}
			if arrayItem && !enabled {
				return nil, false, nil
			}
		}
		result := make(map[string]any, len(typed))
		for key, child := range typed {
			if key == "_panel" {
				continue
			}
			stripped, included, err := stripPanelValue(child, false)
			if err != nil {
				return nil, false, err
			}
			if included {
				result[key] = stripped
			}
		}
		return result, true, nil
	case []any:
		result := make([]any, 0, len(typed))
		for _, child := range typed {
			stripped, included, err := stripPanelValue(child, true)
			if err != nil {
				return nil, false, err
			}
			if included {
				result = append(result, stripped)
			}
		}
		return result, true, nil
	default:
		return typed, true, nil
	}
}

func validateShape(value any, depth int, values *int) error {
	if depth > maximumDepth {
		return fmt.Errorf("document nesting exceeds depth %d", maximumDepth)
	}
	*values++
	if *values > maximumValues {
		return fmt.Errorf("document exceeds %d values", maximumValues)
	}
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			if strings.ContainsRune(key, '\x00') {
				return errors.New("object key contains a NUL byte")
			}
			if err := validateShape(child, depth+1, values); err != nil {
				return err
			}
		}
	case []any:
		for _, child := range typed {
			if err := validateShape(child, depth+1, values); err != nil {
				return err
			}
		}
	case nil, bool, string, json.Number:
	default:
		return fmt.Errorf("unsupported JSON value %T", value)
	}
	return nil
}
