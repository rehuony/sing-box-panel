// SPDX-License-Identifier: GPL-3.0-or-later

// Package document decodes bounded, unambiguous subscription documents.
package document

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
	MaximumBytes  = 4 << 20
	MaximumDepth  = 64
	MaximumValues = 200_000
)

var ErrInvalid = errors.New("invalid subscription document")

type Error struct {
	code string
}

func (err *Error) Error() string { return fmt.Sprintf("%v: %s", ErrInvalid, err.code) }

func (err *Error) Unwrap() error { return ErrInvalid }

func (err *Error) Code() string {
	if err == nil {
		return ""
	}
	return err.code
}

func Invalid(code string) error { return &Error{code: code} }

func Code(err error) string {
	var target *Error
	if errors.As(err, &target) {
		return target.Code()
	}
	return ""
}

func Decode(data []byte) (any, error) {
	if len(data) == 0 {
		return nil, Invalid("empty_document")
	}
	if len(data) > MaximumBytes {
		return nil, Invalid("document_too_large")
	}
	if !utf8.Valid(data) {
		return nil, Invalid("invalid_utf8")
	}
	if err := inspect(data); err != nil {
		return nil, err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, Invalid("malformed_json")
	}
	return value, nil
}

func DecodeObject(data []byte) (map[string]any, error) {
	value, err := Decode(data)
	if err != nil {
		return nil, err
	}
	object, ok := value.(map[string]any)
	if !ok {
		return nil, Invalid("root_not_object")
	}
	return object, nil
}

func Integer(value any, minimum, maximum int64) (int64, bool) {
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
		return Invalid("trailing_json")
	}
	return nil
}

func inspectValue(decoder *json.Decoder, depth int, values *int) error {
	if depth > MaximumDepth {
		return Invalid("nesting_too_deep")
	}
	*values++
	if *values > MaximumValues {
		return Invalid("too_many_values")
	}
	token, err := decoder.Token()
	if err != nil {
		return Invalid("malformed_json")
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
				return Invalid("malformed_json")
			}
			name, ok := nameToken.(string)
			if !ok || strings.ContainsRune(name, '\x00') {
				return Invalid("invalid_object_key")
			}
			if _, duplicate := seen[name]; duplicate {
				return Invalid("duplicate_object_key")
			}
			seen[name] = struct{}{}
			if err := inspectValue(decoder, depth+1, values); err != nil {
				return err
			}
		}
		closing, tokenErr := decoder.Token()
		if tokenErr != nil || closing != json.Delim('}') {
			return Invalid("malformed_json")
		}
	case '[':
		for decoder.More() {
			if err := inspectValue(decoder, depth+1, values); err != nil {
				return err
			}
		}
		closing, tokenErr := decoder.Token()
		if tokenErr != nil || closing != json.Delim(']') {
			return Invalid("malformed_json")
		}
	default:
		return Invalid("malformed_json")
	}
	return nil
}
