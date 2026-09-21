// SPDX-License-Identifier: GPL-3.0-or-later

package httpapi

import (
	"errors"
	"strings"
)

func parseIfMatch(value string) (string, error) {
	value = strings.TrimSpace(value)
	if len(value) < 2 || value[0] != '"' || value[len(value)-1] != '"' || strings.HasPrefix(value, "W/") {
		return "", errors.New(`If-Match must contain one quoted resource revision`)
	}
	identifier := value[1 : len(value)-1]
	if len(identifier) > 256 || identifier == "" || strings.ContainsAny(identifier, "\"\\,\r\n") {
		return "", errors.New("If-Match contains an invalid resource revision")
	}
	return identifier, nil
}

func quoteETag(identifier string) string {
	return `"` + identifier + `"`
}
