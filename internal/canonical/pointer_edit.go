// SPDX-License-Identifier: GPL-3.0-or-later

package canonical

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

var ErrPointerNotFound = errors.New("canonical JSON pointer not found")

func parseEditPointer(pointer string) ([]string, error) {
	if pointer == "" {
		return []string{}, nil
	}
	if !strings.HasPrefix(pointer, "/") {
		return nil, errors.New("JSON pointer must be empty or start with /")
	}
	rawTokens := strings.Split(strings.TrimPrefix(pointer, "/"), "/")
	tokens := make([]string, len(rawTokens))
	for index, raw := range rawTokens {
		var decoded strings.Builder
		for position := 0; position < len(raw); position++ {
			if raw[position] != '~' {
				decoded.WriteByte(raw[position])
				continue
			}
			if position+1 >= len(raw) || (raw[position+1] != '0' && raw[position+1] != '1') {
				return nil, errors.New("JSON pointer contains an invalid escape")
			}
			position++
			if raw[position] == '0' {
				decoded.WriteByte('~')
			} else {
				decoded.WriteByte('/')
			}
		}
		tokens[index] = decoded.String()
	}
	return tokens, nil
}

func pointerParent(root any, tokens []string) (any, error) {
	current := root
	var err error
	for _, token := range tokens {
		current, err = pointerChild(current, token)
		if err != nil {
			return nil, err
		}
	}
	return current, nil
}

func pointerChild(parent any, token string) (any, error) {
	switch typed := parent.(type) {
	case map[string]any:
		value, exists := typed[token]
		if !exists {
			return nil, fmt.Errorf("%w: object member %q", ErrPointerNotFound, token)
		}
		return value, nil
	case []any:
		index, err := pointerIndex(token, len(typed))
		if err != nil {
			return nil, err
		}
		return typed[index], nil
	default:
		return nil, fmt.Errorf("%w: value is not a container", ErrPointerNotFound)
	}
}

func pointerIndex(token string, length int) (int, error) {
	if token == "" || (len(token) > 1 && token[0] == '0') {
		return 0, fmt.Errorf("%w: invalid array index %q", ErrPointerNotFound, token)
	}
	index, err := strconv.Atoi(token)
	if err != nil || index < 0 || index >= length {
		return 0, fmt.Errorf("%w: array index %q", ErrPointerNotFound, token)
	}
	return index, nil
}

func replaceArrayAtParent(root map[string]any, tokens []string, replacement []any) error {
	if len(tokens) == 0 {
		return errors.New("canonical root is not an array")
	}
	parent, err := pointerParent(root, tokens[:len(tokens)-1])
	if err != nil {
		return err
	}
	last := tokens[len(tokens)-1]
	switch typed := parent.(type) {
	case map[string]any:
		typed[last] = replacement
	case []any:
		index, err := pointerIndex(last, len(typed))
		if err != nil {
			return err
		}
		typed[index] = replacement
	default:
		return fmt.Errorf("%w: array parent is not a container", ErrPointerNotFound)
	}
	return nil
}

func clonePointerValue(value any) (any, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("encode canonical pointer value: %w", err)
	}
	var clone any
	decoder := json.NewDecoder(strings.NewReader(string(encoded)))
	decoder.UseNumber()
	if err := decoder.Decode(&clone); err != nil {
		return nil, fmt.Errorf("decode canonical pointer value: %w", err)
	}
	return clone, nil
}
