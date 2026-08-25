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
