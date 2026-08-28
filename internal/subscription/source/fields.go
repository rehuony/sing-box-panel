// SPDX-License-Identifier: GPL-3.0-or-later

package source

import (
	"encoding/json"
	"strconv"
)

func sourceInteger(value any, minimum, maximum int64) (int64, bool) {
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

func firstString(value map[string]any, names ...string) string {
	for _, name := range names {
		if text, ok := value[name].(string); ok && text != "" {
			return text
		}
	}
	return ""
}

func copyRenamed(target, source map[string]any, targetName, sourceName string) {
	if value, exists := source[sourceName]; exists {
		target[targetName] = value
	}
}
