// SPDX-License-Identifier: GPL-3.0-or-later

package coreartifact

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
)

func validateJSONObject(data []byte, maximumBytes, maximumDepth int) error {
	if len(data) == 0 {
		return fmt.Errorf("empty JSON document")
	}
	if len(data) > maximumBytes {
		return fmt.Errorf("JSON document exceeds %d bytes", maximumBytes)
	}

	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := readJSONValue(decoder, 0, maximumDepth); err != nil {
		return err
	}
	if _, err := decoder.Token(); err != io.EOF {
		if err == nil {
			return fmt.Errorf("JSON document contains more than one value")
		}
		return fmt.Errorf("read trailing JSON: %w", err)
	}
	return nil
}

func readJSONValue(decoder *json.Decoder, depth, maximumDepth int) error {
	if depth > maximumDepth {
		return fmt.Errorf("JSON nesting exceeds depth %d", maximumDepth)
	}
	token, err := decoder.Token()
	if err != nil {
		return fmt.Errorf("decode JSON token: %w", err)
	}
	delimiter, composite := token.(json.Delim)
	if !composite {
		return nil
	}

	switch delimiter {
	case '{':
		seen := make(map[string]struct{})
		for decoder.More() {
			nameToken, err := decoder.Token()
			if err != nil {
				return fmt.Errorf("decode JSON object name: %w", err)
			}
			name, ok := nameToken.(string)
			if !ok {
				return fmt.Errorf("JSON object name is not a string")
			}
			if _, exists := seen[name]; exists {
				return fmt.Errorf("duplicate JSON object key %q", name)
			}
			seen[name] = struct{}{}
			if err := readJSONValue(decoder, depth+1, maximumDepth); err != nil {
				return err
			}
		}
		closing, err := decoder.Token()
		if err != nil {
			return fmt.Errorf("decode JSON object close: %w", err)
		}
		if closing != json.Delim('}') {
			return fmt.Errorf("invalid JSON object close")
		}
	case '[':
		for decoder.More() {
			if err := readJSONValue(decoder, depth+1, maximumDepth); err != nil {
				return err
			}
		}
		closing, err := decoder.Token()
		if err != nil {
			return fmt.Errorf("decode JSON array close: %w", err)
		}
		if closing != json.Delim(']') {
			return fmt.Errorf("invalid JSON array close")
		}
	default:
		return fmt.Errorf("unexpected JSON delimiter %q", delimiter)
	}
	return nil
}

func strictDecode(data []byte, destination any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	decoder.UseNumber()
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			return fmt.Errorf("JSON document contains more than one value")
		}
		return err
	}
	return nil
}
