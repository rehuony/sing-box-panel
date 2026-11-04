// SPDX-License-Identifier: GPL-3.0-or-later

// Package subscription owns bounded document decoding, normalized nodes,
// source parsing and fetching, rendering, and inbound conversion contracts.
package subscription

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"unicode/utf8"
)

const (
	MaximumDocumentBytes  = 4 << 20
	MaximumDocumentDepth  = 64
	MaximumDocumentValues = 200_000
)

var ErrInvalidDocument = errors.New("invalid subscription document")

type DocumentError struct {
	code string
}

func (err *DocumentError) Error() string { return fmt.Sprintf("%v: %s", ErrInvalidDocument, err.code) }

func (err *DocumentError) Unwrap() error { return ErrInvalidDocument }

func (err *DocumentError) Code() string {
	if err == nil {
		return ""
	}
	return err.code
}

func InvalidDocument(code string) error { return &DocumentError{code: code} }

func DocumentErrorCode(err error) string {
	var target *DocumentError
	if errors.As(err, &target) {
		return target.Code()
	}
	return ""
}

func DecodeDocument(data []byte) (any, error) {
	if len(data) == 0 {
		return nil, InvalidDocument("empty_document")
	}
	if len(data) > MaximumDocumentBytes {
		return nil, InvalidDocument("document_too_large")
	}
	if !utf8.Valid(data) {
		return nil, InvalidDocument("invalid_utf8")
	}
	if err := inspect(data); err != nil {
		return nil, err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, InvalidDocument("malformed_json")
	}
	return value, nil
}

func DecodeDocumentObject(data []byte) (map[string]any, error) {
	value, err := DecodeDocument(data)
	if err != nil {
		return nil, err
	}
	object, ok := value.(map[string]any)
	if !ok {
		return nil, InvalidDocument("root_not_object")
	}
	return object, nil
}

func DocumentInteger(value any, minimum, maximum int64) (int64, bool) {
	switch number := value.(type) {
	case json.Number:
		parsed, err := strconv.ParseInt(number.String(), 10, 64)
		return parsed, err == nil && parsed >= minimum && parsed <= maximum
	case int:
		return int64(number), int64(number) >= minimum && int64(number) <= maximum
	case int64:
		return number, number >= minimum && number <= maximum
	case float64:
		parsed := int64(number)
		return parsed, float64(parsed) == number && parsed >= minimum && parsed <= maximum
	case string:
		parsed, err := strconv.ParseInt(number, 10, 64)
		return parsed, err == nil && parsed >= minimum && parsed <= maximum
	default:
		return 0, false
	}
}

func inspect(data []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	values := 0
	if err := inspectValue(decoder, 0, &values); err != nil {
		return err
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		return InvalidDocument("trailing_json")
	}
	return nil
}

func inspectValue(decoder *json.Decoder, depth int, values *int) error {
	if depth > MaximumDocumentDepth {
		return InvalidDocument("nesting_too_deep")
	}
	*values++
	if *values > MaximumDocumentValues {
		return InvalidDocument("too_many_values")
	}
	token, err := decoder.Token()
	if err != nil {
		return InvalidDocument("malformed_json")
	}
	delimiter, composite := token.(json.Delim)
	if !composite {
		return nil
	}
	switch delimiter {
	case '{':
		seen := make(map[string]struct{})
		for decoder.More() {
			nameToken, tokenErr := decoder.Token()
			if tokenErr != nil {
				return InvalidDocument("malformed_json")
			}
			name, ok := nameToken.(string)
			if !ok || strings.ContainsRune(name, '\x00') {
				return InvalidDocument("invalid_object_key")
			}
			if _, duplicate := seen[name]; duplicate {
				return InvalidDocument("duplicate_object_key")
			}
			seen[name] = struct{}{}
			if err := inspectValue(decoder, depth+1, values); err != nil {
				return err
			}
		}
		closing, tokenErr := decoder.Token()
		if tokenErr != nil || closing != json.Delim('}') {
			return InvalidDocument("malformed_json")
		}
	case '[':
		for decoder.More() {
			if err := inspectValue(decoder, depth+1, values); err != nil {
				return err
			}
		}
		closing, tokenErr := decoder.Token()
		if tokenErr != nil || closing != json.Delim(']') {
			return InvalidDocument("malformed_json")
		}
	default:
		return InvalidDocument("malformed_json")
	}
	return nil
}
