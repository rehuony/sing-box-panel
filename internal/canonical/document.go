// SPDX-License-Identifier: GPL-3.0-or-later

// Package canonical owns the version-independent semantic configuration
// document. Exact sing-box JSON is produced only by a capability projector.
package canonical

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
	SchemaVersion     = 1
	MaximumBytes      = 2 << 20
	maximumDepth      = 32
	maximumValues     = 100_000
	maximumEntities   = 10_000
	maximumIdentifier = 128
)

var (
	ErrInvalidDocument = errors.New("invalid canonical document")
	identifierPattern  = regexp.MustCompile(`^[a-z][a-z0-9._-]*$`)
	rootKeys           = map[string]struct{}{
		"schema_version": {},
		"global":         {},
		"nodes":          {},
		"rules":          {},
		"subscription":   {},
	}
)

// Document is immutable after construction. Map returns a defensive copy so
// concurrent projections cannot mutate the canonical value or each other.
type Document struct {
	canonical []byte
}

// Empty returns the complete schema-v1 zero document. Empty arrays are
// intentional: array order is semantic and nil is never used as an implicit
// default on the wire.
func Empty() *Document {
	document, err := Parse([]byte(`{"schema_version":1,"global":{},"nodes":[],"rules":[],"subscription":{}}`))
	if err != nil {
		panic(err)
	}
	return document
}

// Parse validates one strict canonical JSON document and stores deterministic
// bytes. It rejects duplicate keys before decoding and bounds both input size
// and decoded shape.
func Parse(data []byte) (*Document, error) {
	var root map[string]any
	if err := jsonstrict.Decode(data, MaximumBytes, &root); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidDocument, err)
	}
	if root == nil {
		return nil, fmt.Errorf("%w: root must be an object", ErrInvalidDocument)
	}
	if err := validateRoot(root); err != nil {
		return nil, err
	}
	values := 0
	if err := validateShape(root, 0, &values); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidDocument, err)
	}
	canonical, err := json.Marshal(root)
	if err != nil {
		return nil, fmt.Errorf("%w: encode deterministic JSON: %v", ErrInvalidDocument, err)
	}
	return &Document{canonical: canonical}, nil
}

func validateRoot(root map[string]any) error {
	for key := range root {
		if _, allowed := rootKeys[key]; !allowed {
			return fmt.Errorf("%w: unknown root field %q", ErrInvalidDocument, key)
		}
	}
	version, ok := root["schema_version"].(json.Number)
	if !ok || version.String() != "1" {
		return fmt.Errorf("%w: schema_version must be exactly %d", ErrInvalidDocument, SchemaVersion)
	}
	if _, ok := root["global"].(map[string]any); !ok {
		return fmt.Errorf("%w: global must be an object", ErrInvalidDocument)
	}
	if _, ok := root["subscription"].(map[string]any); !ok {
		return fmt.Errorf("%w: subscription must be an object", ErrInvalidDocument)
	}
	nodes, ok := root["nodes"].([]any)
	if !ok {
		return fmt.Errorf("%w: nodes must be an array", ErrInvalidDocument)
	}
	if err := validateEntities("nodes", nodes, true); err != nil {
		return err
	}
	rules, ok := root["rules"].([]any)
	if !ok {
		return fmt.Errorf("%w: rules must be an array", ErrInvalidDocument)
	}
	return validateEntities("rules", rules, false)
}

func validateEntities(name string, values []any, requireKind bool) error {
	if len(values) > maximumEntities {
		return fmt.Errorf("%w: %s exceeds %d entities", ErrInvalidDocument, name, maximumEntities)
	}
	identifiers := make(map[string]struct{}, len(values))
	for index, raw := range values {
		value, ok := raw.(map[string]any)
		if !ok {
			return fmt.Errorf("%w: %s[%d] must be an object", ErrInvalidDocument, name, index)
		}
		identifier, ok := value["id"].(string)
		if !ok || !validIdentifier(identifier) {
			return fmt.Errorf("%w: %s[%d].id is invalid", ErrInvalidDocument, name, index)
		}
		if _, duplicate := identifiers[identifier]; duplicate {
			return fmt.Errorf("%w: %s contains duplicate id %q", ErrInvalidDocument, name, identifier)
		}
		identifiers[identifier] = struct{}{}
		if _, ok := value["enabled"].(bool); !ok {
			return fmt.Errorf("%w: %s[%d].enabled must be explicitly true or false", ErrInvalidDocument, name, index)
		}
		if requireKind {
			kind, ok := value["kind"].(string)
			if !ok || !validIdentifier(kind) {
				return fmt.Errorf("%w: %s[%d].kind is invalid", ErrInvalidDocument, name, index)
			}
		}
	}
	return nil
}

func validIdentifier(value string) bool {
	return len(value) <= maximumIdentifier && identifierPattern.MatchString(value)
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

// CanonicalJSON returns a caller-owned copy of the deterministic full
// snapshot bytes.
func (document *Document) CanonicalJSON() []byte {
	if document == nil {
		return nil
	}
	return bytes.Clone(document.canonical)
}

// Map returns a deep copy suitable for a capability projector.
func (document *Document) Map() map[string]any {
	if document == nil {
		return nil
	}
	var clone map[string]any
	decoder := json.NewDecoder(bytes.NewReader(document.canonical))
	decoder.UseNumber()
	if err := decoder.Decode(&clone); err != nil {
		panic(err)
	}
	return clone
}
