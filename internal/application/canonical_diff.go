// SPDX-License-Identifier: GPL-3.0-or-later

package application

import (
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"
)

func decodeCanonicalValue(raw json.RawMessage, target *any) error {
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.UseNumber()
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("decode stored canonical revision: %w", err)
	}
	return nil
}

func collectCanonicalDiff(
	path string,
	from any,
	fromPresent bool,
	to any,
	toPresent bool,
	changes *[]CanonicalDiffEntry,
) {
	if !fromPresent || !toPresent {
		*changes = append(*changes, CanonicalDiffEntry{
			Path: displayPointer(path),
			From: DiffValue{Present: fromPresent, Value: from},
			To:   DiffValue{Present: toPresent, Value: to},
		})
		return
	}
	fromObject, fromIsObject := from.(map[string]any)
	toObject, toIsObject := to.(map[string]any)
	if fromIsObject && toIsObject {
		keys := make([]string, 0, len(fromObject)+len(toObject))
		seen := make(map[string]struct{}, len(fromObject)+len(toObject))
		for key := range fromObject {
			seen[key] = struct{}{}
			keys = append(keys, key)
		}
		for key := range toObject {
			if _, ok := seen[key]; !ok {
				keys = append(keys, key)
			}
		}
		sort.Strings(keys)
		for _, key := range keys {
			fromChild, fromOK := fromObject[key]
			toChild, toOK := toObject[key]
			collectCanonicalDiff(path+"/"+escapePointerToken(key), fromChild, fromOK, toChild, toOK, changes)
		}
		return
	}
	// Ordered collections are semantic. Treat each array as one atomic value so
	// moves remain readable instead of producing misleading index edits.
	if reflect.DeepEqual(from, to) {
		return
	}
	*changes = append(*changes, CanonicalDiffEntry{
		Path: displayPointer(path),
		From: DiffValue{Present: true, Value: from},
		To:   DiffValue{Present: true, Value: to},
	})
}

func escapePointerToken(value string) string {
	return strings.ReplaceAll(strings.ReplaceAll(value, "~", "~0"), "/", "~1")
}

func displayPointer(value string) string {
	if value == "" {
		return "/"
	}
	return value
}
