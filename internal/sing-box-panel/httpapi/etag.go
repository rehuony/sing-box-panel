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

// noneMatchHits reports whether an If-None-Match header matches current.
// If-None-Match uses weak comparison, including ETags weakened by an
// intermediary when it compresses the response (RFC 9110 §13.1.2).
func noneMatchHits(header string, current string) bool {
	for _, candidate := range strings.Split(header, ",") {
		candidate = strings.TrimSpace(candidate)
		if candidate == "*" || strings.TrimPrefix(candidate, "W/") == strings.TrimPrefix(current, "W/") {
			return true
		}
	}
	return false
}
