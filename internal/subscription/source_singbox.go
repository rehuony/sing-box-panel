// SPDX-License-Identifier: GPL-3.0-or-later

package subscription

import (
	"errors"
)

func parseSingBoxSource(raw []byte) ([]map[string]any, error) {
	value, err := DecodeDocument(raw)
	if err != nil {
		return nil, err
	}
	var items []any
	switch root := value.(type) {
	case []any:
		items = root
	case map[string]any:
		if outbounds, exists := root["outbounds"]; exists {
			var ok bool
			items, ok = outbounds.([]any)
			if !ok {
				return nil, errors.New("outbounds is not an array")
			}
		} else {
			items = []any{root}
		}
	default:
		return nil, errors.New("source root is not an object or array")
	}
	return sourceObjectList(items)
}

func sourceObjectList(items []any) ([]map[string]any, error) {
	if len(items) > MaximumNodes {
		return nil, errors.New("too many source nodes")
	}
	values := make([]map[string]any, 0, len(items))
	for _, item := range items {
		object, ok := item.(map[string]any)
		if !ok {
			return nil, errors.New("source node is not an object")
		}
		values = append(values, object)
	}
	return values, nil
}
