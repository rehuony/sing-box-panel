// SPDX-License-Identifier: GPL-3.0-or-later

// Package configuration owns canonical configuration documents and the
// compiled adapter contracts that project them for exact sing-box versions.
package configuration

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

const (
	MaximumBytes      = 2 << 20
	maximumDepth      = 32
	maximumValues     = 100_000
	maximumEntities   = 10_000
	maximumIdentifier = 128
)

var ErrInvalidDocument = errors.New("invalid canonical document")

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
