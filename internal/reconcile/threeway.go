// SPDX-License-Identifier: GPL-3.0-or-later

// Package reconcile performs deterministic base/current/manual three-way
// reconciliation. It never guesses at a conflicting semantic path.
package reconcile

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
)

var ErrUnresolvedConflict = errors.New("three-way reconciliation has unresolved conflicts")

type Choice string

const (
	ChooseCurrent Choice = "current"
	ChooseManual  Choice = "manual"
)

type EncodedValue struct {
	Present bool            `json:"present"`
	Value   json.RawMessage `json:"value,omitempty"`
}

type Conflict struct {
	Path    string       `json:"path"`
	Base    EncodedValue `json:"base"`
	Current EncodedValue `json:"current"`
	Manual  EncodedValue `json:"manual"`
}

type Preview struct {
	Merged    map[string]any `json:"merged"`
	Conflicts []Conflict     `json:"conflicts"`
}

type presenceValue struct {
	present bool
	value   any
}

// ThreeWay automatically merges non-overlapping or one-sided changes. At a
// conflict it keeps current in the preview and records all three presences and
// values for an explicit decision.
func ThreeWay(base, current, manual map[string]any) (Preview, error) {
	if base == nil || current == nil || manual == nil {
		return Preview{}, errors.New("three-way roots must all be objects")
	}
	base, current, manual, err := normalizeRoots(base, current, manual)
	if err != nil {
		return Preview{}, err
	}
	merged, conflicts, err := mergeValue(
		presenceValue{present: true, value: base},
		presenceValue{present: true, value: current},
		presenceValue{present: true, value: manual},
		nil,
		nil,
	)
	if err != nil {
		return Preview{}, err
	}
	result, ok := merged.value.(map[string]any)
	if !merged.present || !ok {
		return Preview{}, errors.New("three-way merge did not produce an object")
	}
	sort.Slice(conflicts, func(left, right int) bool { return conflicts[left].Path < conflicts[right].Path })
	return Preview{Merged: result, Conflicts: conflicts}, nil
}

// Resolve applies one explicit decision for every conflict. Extra or missing
// decisions are rejected so a stale UI cannot silently resolve the wrong set.
func Resolve(base, current, manual map[string]any, decisions map[string]Choice) (map[string]any, error) {
	var err error
	base, current, manual, err = normalizeRoots(base, current, manual)
	if err != nil {
		return nil, err
	}
	if decisions == nil {
		decisions = map[string]Choice{}
	}
	used := make(map[string]struct{}, len(decisions))
	merged, conflicts, err := mergeValue(
		presenceValue{present: true, value: base},
		presenceValue{present: true, value: current},
		presenceValue{present: true, value: manual},
		nil,
		func(path string, current, manual presenceValue) (presenceValue, bool) {
			choice, ok := decisions[path]
			if !ok {
				return presenceValue{}, false
			}
			used[path] = struct{}{}
			switch choice {
			case ChooseCurrent:
				return current, true
			case ChooseManual:
				return manual, true
			default:
				return presenceValue{}, false
			}
		},
	)
	if err != nil {
		return nil, err
	}
	if len(conflicts) != 0 {
		paths := make([]string, len(conflicts))
		for index, conflict := range conflicts {
			paths[index] = conflict.Path
		}
		return nil, fmt.Errorf("%w: %s", ErrUnresolvedConflict, strings.Join(paths, ", "))
	}
	for path := range decisions {
		if _, ok := used[path]; !ok {
			return nil, fmt.Errorf("decision path %q is not a current conflict", path)
		}
	}
	result, ok := merged.value.(map[string]any)
	if !merged.present || !ok {
		return nil, errors.New("resolved merge did not produce an object")
	}
	return result, nil
}

type resolver func(string, presenceValue, presenceValue) (presenceValue, bool)

func mergeValue(
	base, current, manual presenceValue,
	tokens []string,
	resolve resolver,
) (presenceValue, []Conflict, error) {
	if equalValue(current, manual) {
		return clonePresence(current), nil, nil
	}
	if equalValue(current, base) {
		return clonePresence(manual), nil, nil
	}
	if equalValue(manual, base) {
		return clonePresence(current), nil, nil
	}

	baseObject, baseIsObject := objectValue(base)
	currentObject, currentIsObject := objectValue(current)
	manualObject, manualIsObject := objectValue(manual)
	if currentIsObject && manualIsObject && (baseIsObject || !base.present) {
		if !baseIsObject {
			baseObject = map[string]any{}
		}
		keys := unionKeys(baseObject, currentObject, manualObject)
		result := make(map[string]any, len(keys))
		conflicts := make([]Conflict, 0)
		for _, key := range keys {
			merged, childConflicts, err := mergeValue(
				lookup(baseObject, key),
				lookup(currentObject, key),
				lookup(manualObject, key),
				appendToken(tokens, key),
				resolve,
			)
			if err != nil {
				return presenceValue{}, nil, err
			}
			if merged.present {
				result[key] = merged.value
			}
			conflicts = append(conflicts, childConflicts...)
		}
		return presenceValue{present: true, value: result}, conflicts, nil
	}

	path := encodePointer(tokens)
	if resolve != nil {
		if selected, ok := resolve(path, current, manual); ok {
			return clonePresence(selected), nil, nil
		}
	}
	conflict, err := newConflict(path, base, current, manual)
	if err != nil {
		return presenceValue{}, nil, err
	}
	return clonePresence(current), []Conflict{conflict}, nil
}

func objectValue(value presenceValue) (map[string]any, bool) {
	if !value.present {
		return nil, false
	}
	object, ok := value.value.(map[string]any)
	return object, ok
}

func lookup(object map[string]any, key string) presenceValue {
	value, ok := object[key]
	return presenceValue{present: ok, value: value}
}

func unionKeys(objects ...map[string]any) []string {
	set := make(map[string]struct{})
	for _, object := range objects {
		for key := range object {
			set[key] = struct{}{}
		}
	}
	keys := make([]string, 0, len(set))
	for key := range set {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func equalValue(left, right presenceValue) bool {
	if left.present != right.present {
		return false
	}
	if !left.present {
		return true
	}
	leftJSON, leftErr := json.Marshal(left.value)
	rightJSON, rightErr := json.Marshal(right.value)
	return leftErr == nil && rightErr == nil && string(leftJSON) == string(rightJSON)
}

func clonePresence(value presenceValue) presenceValue {
	if !value.present {
		return presenceValue{}
	}
	encoded, err := json.Marshal(value.value)
	if err != nil {
		panic(err)
	}
	var clone any
	decoder := json.NewDecoder(strings.NewReader(string(encoded)))
	decoder.UseNumber()
	if err := decoder.Decode(&clone); err != nil {
		panic(err)
	}
	return presenceValue{present: true, value: clone}
}

func normalizeRoots(values ...map[string]any) (map[string]any, map[string]any, map[string]any, error) {
	if len(values) != 3 {
		panic("normalizeRoots requires exactly three roots")
	}
	result := make([]map[string]any, 3)
	for index, value := range values {
		if value == nil {
			return nil, nil, nil, errors.New("three-way roots must all be objects")
		}
		encoded, err := json.Marshal(value)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("encode three-way root %d: %w", index, err)
		}
		decoder := json.NewDecoder(strings.NewReader(string(encoded)))
		decoder.UseNumber()
		if err := decoder.Decode(&result[index]); err != nil {
			return nil, nil, nil, fmt.Errorf("decode three-way root %d: %w", index, err)
		}
	}
	return result[0], result[1], result[2], nil
}

func newConflict(path string, base, current, manual presenceValue) (Conflict, error) {
	baseValue, err := encodeValue(base)
	if err != nil {
		return Conflict{}, err
	}
	currentValue, err := encodeValue(current)
	if err != nil {
		return Conflict{}, err
	}
	manualValue, err := encodeValue(manual)
	if err != nil {
		return Conflict{}, err
	}
	return Conflict{Path: path, Base: baseValue, Current: currentValue, Manual: manualValue}, nil
}

func encodeValue(value presenceValue) (EncodedValue, error) {
	if !value.present {
		return EncodedValue{}, nil
	}
	encoded, err := json.Marshal(value.value)
	if err != nil {
		return EncodedValue{}, err
	}
	return EncodedValue{Present: true, Value: encoded}, nil
}

func appendToken(tokens []string, token string) []string {
	result := make([]string, len(tokens)+1)
	copy(result, tokens)
	result[len(tokens)] = token
	return result
}

func encodePointer(tokens []string) string {
	if len(tokens) == 0 {
		return ""
	}
	encoded := make([]string, len(tokens))
	for index, token := range tokens {
		encoded[index] = strings.ReplaceAll(strings.ReplaceAll(token, "~", "~0"), "/", "~1")
	}
	return "/" + strings.Join(encoded, "/")
}
