// SPDX-License-Identifier: GPL-3.0-or-later

package configuration

import (
	"bytes"
	"encoding/json"
	"sort"

	apiassets "github.com/rehuony/sing-box-panel/api"
)

var presentationOrder = func() struct{ Root, Fields []string } {
	var order struct{ Root, Fields []string }
	if err := json.Unmarshal(apiassets.ConfigurationOrder, &order); err != nil {
		panic(err)
	}
	return order
}()

// FormatJSON changes only object field order and whitespace. Raw scalar values
// and array order survive; runtime canonical bytes and digests use their own codec.
func FormatJSON(raw []byte) ([]byte, error) {
	var value json.RawMessage
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, err
	}
	var compact bytes.Buffer
	var write func(json.RawMessage, bool) error
	write = func(value json.RawMessage, root bool) error {
		value = bytes.TrimSpace(value)
		switch value[0] {
		case '{':
			var object map[string]json.RawMessage
			if err := json.Unmarshal(value, &object); err != nil {
				return err
			}
			order := presentationOrder.Fields
			if root {
				order = presentationOrder.Root
			}
			rank := map[string]int{}
			for i, key := range order {
				rank[key] = i + 1
			}
			keys := make([]string, 0, len(object))
			for key := range object {
				keys = append(keys, key)
			}
			sort.Slice(keys, func(i, j int) bool {
				a, b := rank[keys[i]], rank[keys[j]]
				if a == 0 {
					a = len(order) + 1
				}
				if b == 0 {
					b = len(order) + 1
				}
				if a != b {
					return a < b
				}
				return keys[i] < keys[j]
			})
			compact.WriteByte('{')
			for i, key := range keys {
				if i > 0 {
					compact.WriteByte(',')
				}
				encoded, _ := json.Marshal(key)
				compact.Write(encoded)
				compact.WriteByte(':')
				if err := write(object[key], false); err != nil {
					return err
				}
			}
			compact.WriteByte('}')
		case '[':
			var items []json.RawMessage
			if err := json.Unmarshal(value, &items); err != nil {
				return err
			}
			compact.WriteByte('[')
			for i, item := range items {
				if i > 0 {
					compact.WriteByte(',')
				}
				if err := write(item, false); err != nil {
					return err
				}
			}
			compact.WriteByte(']')
		default:
			compact.Write(value)
		}
		return nil
	}
	if err := write(value, true); err != nil {
		return nil, err
	}
	var pretty bytes.Buffer
	if err := json.Indent(&pretty, compact.Bytes(), "", "  "); err != nil {
		return nil, err
	}
	return pretty.Bytes(), nil
}
