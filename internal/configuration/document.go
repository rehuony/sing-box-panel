// SPDX-License-Identifier: GPL-3.0-or-later

// Package configuration owns immutable raw sing-box configuration documents.
package configuration

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/rehuony/sing-box-panel/internal/jsonstrict"
)

const (
	SchemaVersion = 1
	MaximumBytes  = 2 << 20
	maximumDepth  = 32
	maximumValues = 100_000
)

var ErrInvalidDocument = errors.New("invalid configuration document")

// Document is one immutable sing-box JSON configuration revision. Its root is
// the configuration object consumed by the selected sing-box binary; panel
// metadata and version envelopes are not part of this document.
type Document struct {
	canonical []byte
}

// Empty returns the global zero configuration. Selecting another core does
// not create or modify this document.
func Empty() *Document {
	document, err := Parse([]byte(`{}`))
	if err != nil {
		panic(err)
	}
	return document
}

// Parse validates the bounded, strictly decoded JSON object and canonicalizes
// it deterministically. Field semantics remain owned by the exact sing-box
// binary's check command, and optional browser schemas only improve editing.
func Parse(data []byte) (*Document, error) {
	var root map[string]any
	if err := jsonstrict.Decode(data, MaximumBytes, &root); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidDocument, err)
	}
	if root == nil {
		return nil, fmt.Errorf("%w: root must be an object", ErrInvalidDocument)
	}
	values := 0
	if err := validateShape(root, 0, &values); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidDocument, err)
	}
	canonicalJSON, err := json.Marshal(root)
	if err != nil {
		return nil, fmt.Errorf("%w: encode deterministic JSON: %v", ErrInvalidDocument, err)
	}
	return &Document{canonical: canonicalJSON}, nil
}

func (document *Document) CanonicalJSON() []byte {
	if document == nil {
		return nil
	}
	return bytes.Clone(document.canonical)
}

// Configuration returns a defensive copy of the complete sing-box
// configuration object.
func (document *Document) Configuration() map[string]any {
	if document == nil {
		return nil
	}
	var root map[string]any
	decoder := json.NewDecoder(bytes.NewReader(document.canonical))
	decoder.UseNumber()
	if err := decoder.Decode(&root); err != nil {
		panic(err)
	}
	return root
}

func decodeCanonicalMap(raw []byte, target *map[string]any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	return decoder.Decode(target)
}

func encodeCanonicalMap(value map[string]any) ([]byte, error) {
	return json.Marshal(value)
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
