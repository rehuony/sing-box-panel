// SPDX-License-Identifier: GPL-3.0-or-later

// Package jsonstrict decodes JSON while rejecting ambiguous and trailing input.
package jsonstrict

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

type containerFrame struct {
	object    bool
	expecting bool
	keys      map[string]struct{}
}

// Decode decodes one JSON value into target, rejecting duplicate object keys,
// unknown struct fields, trailing values, and inputs larger than maxBytes.
func Decode(data []byte, maxBytes int64, target any) error {
	if maxBytes > 0 && int64(len(data)) > maxBytes {
		return fmt.Errorf("JSON document exceeds %d bytes", maxBytes)
	}
	if err := rejectDuplicateKeys(data); err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	decoder.UseNumber()
	if err := decoder.Decode(target); err != nil {
		var typeErr *json.UnmarshalTypeError
		if errors.As(err, &typeErr) {
			// The decoder's own text names Go types, which callers show.
			if typeErr.Field == "" {
				return fmt.Errorf("decode JSON: unexpected %s value", typeErr.Value)
			}
			return fmt.Errorf("decode JSON: field %q has an unexpected %s value", typeErr.Field, typeErr.Value)
		}
		return fmt.Errorf("decode JSON: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("decode JSON: trailing value")
		}
		return fmt.Errorf("decode JSON trailing input: %w", err)
	}
	return nil
}

// CanonicalObject decodes data, or fallback when data is empty, as one strict
// JSON object and re-encodes it with sorted keys.
func CanonicalObject(data []byte, fallback string, maxBytes int64) (json.RawMessage, error) {
	if len(data) == 0 {
		data = []byte(fallback)
	}
	var object map[string]any
	if err := Decode(data, maxBytes, &object); err != nil {
		return nil, err
	}
	if object == nil {
		return nil, errors.New("value must be a non-null JSON object")
	}
	encoded, err := json.Marshal(object)
	if err != nil {
		return nil, fmt.Errorf("encode JSON object: %w", err)
	}
	return encoded, nil
}

func rejectDuplicateKeys(data []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	stack := make([]containerFrame, 0, 16)
	for {
		token, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			if len(stack) != 0 {
				return errors.New("decode JSON: incomplete container")
			}
			return nil
		}
		if err != nil {
			return fmt.Errorf("decode JSON: %w", err)
		}
		switch value := token.(type) {
		case json.Delim:
			switch value {
			case '{':
				stack = append(stack, containerFrame{object: true, expecting: true, keys: make(map[string]struct{})})
			case '[':
				stack = append(stack, containerFrame{})
			case '}', ']':
				if len(stack) == 0 {
					return errors.New("decode JSON: unmatched closing delimiter")
				}
				stack = stack[:len(stack)-1]
				markObjectValueConsumed(stack)
			}
		case string:
			if len(stack) > 0 && stack[len(stack)-1].object && stack[len(stack)-1].expecting {
				top := &stack[len(stack)-1]
				if _, exists := top.keys[value]; exists {
					return fmt.Errorf("decode JSON: duplicate object key %q", value)
				}
				top.keys[value] = struct{}{}
				top.expecting = false
				continue
			}
			markObjectValueConsumed(stack)
		default:
			markObjectValueConsumed(stack)
		}
	}
}

func markObjectValueConsumed(stack []containerFrame) {
	if len(stack) == 0 {
		return
	}
	top := &stack[len(stack)-1]
	if top.object && !top.expecting {
		top.expecting = true
	}
}
