// SPDX-License-Identifier: GPL-3.0-or-later

// Package manualjson validates exact-byte JSONC startup candidates. Parsing
// never rewrites the source bytes; standard JSON is a derived inspection view.
package manualjson

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"unicode/utf8"

	"github.com/rehuony/sing-box-panel/internal/coreartifact"
	"github.com/rehuony/sing-box-panel/internal/jsonstrict"
	"github.com/tailscale/hujson"
)

const (
	MaximumBytes  = 4 << 20
	maximumDepth  = 64
	maximumValues = 200_000
)

var ErrInvalidManualJSON = errors.New("invalid manual JSON")

// Binding prevents manual residual data from silently crossing exact core,
// binary, or canonical revision boundaries.
type Binding struct {
	CoreVersion    coreartifact.ExactVersion
	ArtifactDigest coreartifact.SHA256
	BaseRevisionID string
}

// Document owns the exact source bytes and a derived strict JSON view.
type Document struct {
	binding  Binding
	raw      []byte
	standard []byte
	digest   [32]byte
}

func Parse(data []byte, binding Binding) (*Document, error) {
	if err := validateBinding(binding); err != nil {
		return nil, err
	}
	if len(data) == 0 {
		return nil, fmt.Errorf("%w: document is empty", ErrInvalidManualJSON)
	}
	if len(data) > MaximumBytes {
		return nil, fmt.Errorf("%w: document exceeds %d bytes", ErrInvalidManualJSON, MaximumBytes)
	}
	if !utf8.Valid(data) {
		return nil, fmt.Errorf("%w: document is not valid UTF-8", ErrInvalidManualJSON)
	}
	if err := preflightDepth(data); err != nil {
		return nil, err
	}
	parsed, err := hujson.Parse(data)
	if err != nil {
		return nil, fmt.Errorf("%w: parse JSONC: %v", ErrInvalidManualJSON, err)
	}
	if _, ok := parsed.Value.(*hujson.Object); !ok {
		return nil, fmt.Errorf("%w: sing-box configuration root must be an object", ErrInvalidManualJSON)
	}
	values := 0
	if err := validateValue(&parsed, 0, &values); err != nil {
		return nil, err
	}
	if packed := parsed.Pack(); !bytes.Equal(packed, data) {
		return nil, fmt.Errorf("%w: parser did not preserve exact source bytes", ErrInvalidManualJSON)
	}
	standardTree := parsed.Clone()
	standardTree.Standardize()
	standard := standardTree.Pack()
	var object map[string]any
	if err := jsonstrict.Decode(standard, MaximumBytes, &object); err != nil {
		return nil, fmt.Errorf("%w: derived JSON is invalid: %v", ErrInvalidManualJSON, err)
	}
	return &Document{
		binding:  binding,
		raw:      bytes.Clone(data),
		standard: bytes.Clone(standard),
		digest:   sha256.Sum256(data),
	}, nil
}

func validateBinding(binding Binding) error {
	if binding.CoreVersion.IsZero() {
		return fmt.Errorf("%w: exact core version is required", ErrInvalidManualJSON)
	}
	if binding.ArtifactDigest.IsZero() {
		return fmt.Errorf("%w: exact artifact digest is required", ErrInvalidManualJSON)
	}
	if binding.BaseRevisionID == "" {
		return fmt.Errorf("%w: base canonical revision is required", ErrInvalidManualJSON)
	}
	return nil
}

func validateValue(value *hujson.Value, depth int, values *int) error {
	if depth > maximumDepth {
		return fmt.Errorf("%w: document nesting exceeds depth %d", ErrInvalidManualJSON, maximumDepth)
	}
	*values++
	if *values > maximumValues {
		return fmt.Errorf("%w: document exceeds %d values", ErrInvalidManualJSON, maximumValues)
	}
	switch typed := value.Value.(type) {
	case *hujson.Object:
		seen := make(map[string]struct{}, len(typed.Members))
		for _, member := range typed.Members {
			literal, ok := member.Name.Value.(hujson.Literal)
			if !ok || literal.Kind() != '"' {
				return fmt.Errorf("%w: object member name is not a string", ErrInvalidManualJSON)
			}
			name := literal.String()
			if _, duplicate := seen[name]; duplicate {
				return fmt.Errorf("%w: duplicate object key %q", ErrInvalidManualJSON, name)
			}
			seen[name] = struct{}{}
			if err := validateValue(&member.Value, depth+1, values); err != nil {
				return err
			}
		}
	case *hujson.Array:
		for index := range typed.Elements {
			if err := validateValue(&typed.Elements[index], depth+1, values); err != nil {
				return err
			}
		}
	case hujson.Literal:
	default:
		return fmt.Errorf("%w: unsupported syntax node %T", ErrInvalidManualJSON, value.Value)
	}
	return nil
}

// preflightDepth bounds parser recursion without attempting to validate the
// JSONC grammar. The real parser remains the syntax authority.
func preflightDepth(data []byte) error {
	const (
		stateValue = iota
		stateString
		stateEscape
		stateLineComment
		stateBlockComment
		stateBlockStar
	)
	state := stateValue
	depth := 0
	for index := 0; index < len(data); index++ {
		current := data[index]
		switch state {
		case stateValue:
			switch current {
			case '"':
				state = stateString
			case '/':
				if index+1 < len(data) && data[index+1] == '/' {
					state = stateLineComment
					index++
				} else if index+1 < len(data) && data[index+1] == '*' {
					state = stateBlockComment
					index++
				}
			case '{', '[':
				depth++
				if depth > maximumDepth {
					return fmt.Errorf("%w: document nesting exceeds depth %d", ErrInvalidManualJSON, maximumDepth)
				}
			case '}', ']':
				if depth > 0 {
					depth--
				}
			}
		case stateString:
			if current == '\\' {
				state = stateEscape
			} else if current == '"' {
				state = stateValue
			}
		case stateEscape:
			state = stateString
		case stateLineComment:
			if current == '\n' || current == '\r' {
				state = stateValue
			}
		case stateBlockComment:
			if current == '*' {
				state = stateBlockStar
			}
		case stateBlockStar:
			if current == '/' {
				state = stateValue
			} else if current != '*' {
				state = stateBlockComment
			}
		}
	}
	return nil
}

func (document *Document) Binding() Binding {
	if document == nil {
		return Binding{}
	}
	return document.binding
}

func (document *Document) RawBytes() []byte {
	if document == nil {
		return nil
	}
	return bytes.Clone(document.raw)
}

func (document *Document) StandardJSON() []byte {
	if document == nil {
		return nil
	}
	return bytes.Clone(document.standard)
}

func (document *Document) SHA256() string {
	if document == nil {
		return ""
	}
	return hex.EncodeToString(document.digest[:])
}

// Object returns a caller-owned standard JSON object for reverse projection.
func (document *Document) Object() map[string]any {
	if document == nil {
		return nil
	}
	var object map[string]any
	decoder := json.NewDecoder(bytes.NewReader(document.standard))
	decoder.UseNumber()
	if err := decoder.Decode(&object); err != nil {
		panic(err)
	}
	return object
}
