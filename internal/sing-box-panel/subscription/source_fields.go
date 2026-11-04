// SPDX-License-Identifier: GPL-3.0-or-later

package subscription

import ()

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
