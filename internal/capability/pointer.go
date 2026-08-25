// SPDX-License-Identifier: GPL-3.0-or-later

package capability

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

func parsePointer(pointer string) ([]string, error) {
	if pointer == "" || pointer[0] != '/' {
		return nil, fmt.Errorf("JSON Pointer %q must start with / and cannot own the document root", pointer)
	}
	if len(pointer) > 512 || !utf8.ValidString(pointer) {
		return nil, fmt.Errorf("JSON Pointer %q is too long or invalid UTF-8", pointer)
	}
	rawTokens := strings.Split(pointer[1:], "/")
	tokens := make([]string, len(rawTokens))
	for index, token := range rawTokens {
		if token == "" {
			return nil, fmt.Errorf("JSON Pointer %q contains an empty token", pointer)
		}
		var decoded strings.Builder
		for offset := 0; offset < len(token); offset++ {
			if token[offset] != '~' {
				decoded.WriteByte(token[offset])
				continue
			}
			if offset+1 >= len(token) {
				return nil, fmt.Errorf("JSON Pointer %q contains an incomplete escape", pointer)
			}
			offset++
			switch token[offset] {
			case '0':
				decoded.WriteByte('~')
			case '1':
				decoded.WriteByte('/')
			default:
				return nil, fmt.Errorf("JSON Pointer %q contains an invalid escape", pointer)
			}
		}
		tokens[index] = decoded.String()
		if tokens[index] == "-" {
			return nil, fmt.Errorf("JSON Pointer %q uses the non-deterministic array append token", pointer)
		}
		for _, character := range tokens[index] {
			if unicode.IsControl(character) {
				return nil, fmt.Errorf("JSON Pointer %q contains a control character", pointer)
			}
		}
	}
	if encodePointer(tokens) != pointer {
		return nil, fmt.Errorf("JSON Pointer %q is not canonically escaped", pointer)
	}
	return tokens, nil
}

func encodePointer(tokens []string) string {
	encoded := make([]string, len(tokens))
	for index, token := range tokens {
		token = strings.ReplaceAll(token, "~", "~0")
		token = strings.ReplaceAll(token, "/", "~1")
		encoded[index] = token
	}
	return "/" + strings.Join(encoded, "/")
}

func pointerContains(owner, candidate string) bool {
	return owner == candidate || strings.HasPrefix(candidate, owner+"/")
}

func pointersOverlap(left, right string) bool {
	return pointerContains(left, right) || pointerContains(right, left)
}

func arrayIndex(token string) (int, bool) {
	if token == "0" {
		return 0, true
	}
	if token == "" || token[0] == '0' {
		return 0, false
	}
	for _, character := range token {
		if character < '0' || character > '9' {
			return 0, false
		}
	}
	value, err := strconv.ParseInt(token, 10, 32)
	if err != nil {
		return 0, false
	}
	return int(value), true
}
