// SPDX-License-Identifier: GPL-3.0-or-later

package canonical

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

var (
	ErrEntityNotFound   = errors.New("canonical entity not found")
	ErrEntityExists     = errors.New("canonical entity already exists")
	ErrEntityReferenced = errors.New("canonical entity is still referenced")
)

type Collection string

const (
	CollectionNodes Collection = "nodes"
	CollectionRules Collection = "rules"
)

func (document *Document) Entities(collection Collection) ([]map[string]any, error) {
	root, values, err := document.collection(collection)
	_ = root
	if err != nil {
		return nil, err
	}
	result := make([]map[string]any, len(values))
	for index, raw := range values {
		result[index], err = cloneObject(raw.(map[string]any))
		if err != nil {
			return nil, err
		}
	}
	return result, nil
}

func (document *Document) Entity(collection Collection, identifier string) (map[string]any, error) {
	_, values, err := document.collection(collection)
	if err != nil {
		return nil, err
	}
	index := findEntity(values, identifier)
	if index < 0 {
		return nil, fmt.Errorf("%w: %s %q", ErrEntityNotFound, collection, identifier)
	}
	return cloneObject(values[index].(map[string]any))
}

func (document *Document) CreateEntity(collection Collection, entity map[string]any) (*Document, error) {
	root, values, err := document.collection(collection)
	if err != nil {
		return nil, err
	}
	identifier, _ := entity["id"].(string)
	if findEntity(values, identifier) >= 0 {
		return nil, fmt.Errorf("%w: %s %q", ErrEntityExists, collection, identifier)
	}
	clone, err := cloneObject(entity)
	if err != nil {
		return nil, err
	}
	root[string(collection)] = append(values, clone)
	return buildEdited(root)
}

func (document *Document) ReplaceEntity(collection Collection, identifier string, entity map[string]any) (*Document, error) {
	root, values, err := document.collection(collection)
	if err != nil {
		return nil, err
	}
	index := findEntity(values, identifier)
	if index < 0 {
		return nil, fmt.Errorf("%w: %s %q", ErrEntityNotFound, collection, identifier)
	}
	replacementID, _ := entity["id"].(string)
	if replacementID != identifier {
		return nil, fmt.Errorf("entity id is immutable: got %q, want %q", replacementID, identifier)
	}
	clone, err := cloneObject(entity)
	if err != nil {
		return nil, err
	}
	values[index] = clone
	root[string(collection)] = values
	return buildEdited(root)
}

func (document *Document) SetEntityEnabled(collection Collection, identifier string, enabled bool) (*Document, error) {
	root, values, err := document.collection(collection)
	if err != nil {
		return nil, err
	}
	index := findEntity(values, identifier)
	if index < 0 {
		return nil, fmt.Errorf("%w: %s %q", ErrEntityNotFound, collection, identifier)
	}
	entity, err := cloneObject(values[index].(map[string]any))
	if err != nil {
		return nil, err
	}
	entity["enabled"] = enabled
	values[index] = entity
	root[string(collection)] = values
	return buildEdited(root)
}

// MoveEntity places identifier immediately before beforeID. An empty beforeID
// moves the entity to the end. Array order is the only ordering authority.
func (document *Document) MoveEntity(collection Collection, identifier, beforeID string) (*Document, error) {
	root, values, err := document.collection(collection)
	if err != nil {
		return nil, err
	}
	from := findEntity(values, identifier)
	if from < 0 {
		return nil, fmt.Errorf("%w: %s %q", ErrEntityNotFound, collection, identifier)
	}
	if beforeID == identifier {
		return document, nil
	}
	moving := values[from]
	values = append(values[:from], values[from+1:]...)
	target := len(values)
	if beforeID != "" {
		target = findEntity(values, beforeID)
		if target < 0 {
			return nil, fmt.Errorf("%w: %s %q", ErrEntityNotFound, collection, beforeID)
		}
	}
	values = append(values, nil)
	copy(values[target+1:], values[target:])
	values[target] = moving
	root[string(collection)] = values
	return buildEdited(root)
}

func (document *Document) DeleteEntity(collection Collection, identifier string) (*Document, error) {
	root, values, err := document.collection(collection)
	if err != nil {
		return nil, err
	}
	index := findEntity(values, identifier)
	if index < 0 {
		return nil, fmt.Errorf("%w: %s %q", ErrEntityNotFound, collection, identifier)
	}
	root[string(collection)] = append(values[:index], values[index+1:]...)
	references := make([]string, 0)
	collectStringReferences(root, identifier, nil, &references)
	if len(references) != 0 {
		sort.Strings(references)
		return nil, fmt.Errorf("%w: %s", ErrEntityReferenced, strings.Join(references, ", "))
	}
	return buildEdited(root)
}

func (document *Document) collection(collection Collection) (map[string]any, []any, error) {
	if document == nil {
		return nil, nil, errors.New("canonical document is nil")
	}
	if collection != CollectionNodes && collection != CollectionRules {
		return nil, nil, fmt.Errorf("unknown canonical collection %q", collection)
	}
	root := document.Map()
	values, ok := root[string(collection)].([]any)
	if !ok {
		return nil, nil, fmt.Errorf("canonical collection %q is not an array", collection)
	}
	return root, values, nil
}

func findEntity(values []any, identifier string) int {
	for index, raw := range values {
		entity, ok := raw.(map[string]any)
		if ok && entity["id"] == identifier {
			return index
		}
	}
	return -1
}

func buildEdited(root map[string]any) (*Document, error) {
	encoded, err := json.Marshal(root)
	if err != nil {
		return nil, fmt.Errorf("encode edited canonical document: %w", err)
	}
	return Parse(encoded)
}

func cloneObject(object map[string]any) (map[string]any, error) {
	encoded, err := json.Marshal(object)
	if err != nil {
		return nil, fmt.Errorf("clone canonical entity: %w", err)
	}
	var clone map[string]any
	decoder := json.NewDecoder(strings.NewReader(string(encoded)))
	decoder.UseNumber()
	if err := decoder.Decode(&clone); err != nil {
		return nil, fmt.Errorf("clone canonical entity: %w", err)
	}
	return clone, nil
}

func collectStringReferences(value any, identifier string, tokens []string, paths *[]string) {
	switch typed := value.(type) {
	case map[string]any:
		keys := make([]string, 0, len(typed))
		for key := range typed {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			collectStringReferences(typed[key], identifier, appendEditorToken(tokens, key), paths)
		}
	case []any:
		for index, child := range typed {
			collectStringReferences(child, identifier, appendEditorToken(tokens, strconv.Itoa(index)), paths)
		}
	case string:
		if typed == identifier {
			*paths = append(*paths, editorPointer(tokens))
		}
	}
}

func appendEditorToken(tokens []string, token string) []string {
	result := make([]string, len(tokens)+1)
	copy(result, tokens)
	result[len(tokens)] = token
	return result
}

func editorPointer(tokens []string) string {
	encoded := make([]string, len(tokens))
	for index, token := range tokens {
		encoded[index] = strings.ReplaceAll(strings.ReplaceAll(token, "~", "~0"), "/", "~1")
	}
	return "/" + strings.Join(encoded, "/")
}
