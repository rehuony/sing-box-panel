// SPDX-License-Identifier: GPL-3.0-or-later

// Package settingsfile holds what the panel's and the node's settings files
// share: bounded reads, the writer lock, data-directory resolution and the
// dotted field names that the config commands read and write.
package settingsfile

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"

	"github.com/rehuony/sing-box-panel/internal/jsonstrict"
)

// MaximumBytes bounds a settings document.
const MaximumBytes = 1 << 20

// Read returns the settings file unchanged, including invalid JSON, and
// refuses one over MaximumBytes.
func Read(path string) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("read settings %q: %w", path, err)
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, MaximumBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read settings %q: %w", path, err)
	}
	if len(data) > MaximumBytes {
		return nil, fmt.Errorf("settings %q exceeds %d bytes", path, MaximumBytes)
	}
	return data, nil
}

// Decode strictly decodes settings bytes into target.
func Decode(path string, data []byte, target any) error {
	if err := jsonstrict.Decode(data, MaximumBytes, target); err != nil {
		return fmt.Errorf("parse settings %q: %w", path, err)
	}
	return nil
}

// DataDirOf reads only data_dir from the settings document data, read from
// path, and resolves it against the file's directory. Unrelated fields are
// ignored, but malformed, ambiguous or oversized JSON is still refused.
func DataDirOf(path string, data []byte) (string, error) {
	var fields map[string]json.RawMessage
	if err := Decode(path, data, &fields); err != nil {
		return "", err
	}
	var dataDir string
	if err := json.Unmarshal(fields["data_dir"], &dataDir); err != nil {
		return "", fmt.Errorf("validate settings %q: data_dir must be a non-empty path: %w", path, err)
	}
	return ResolveDataDir(path, dataDir)
}

// ResolveDataDir makes dataDir absolute, resolving a relative one against
// the directory of the settings file at path.
func ResolveDataDir(path, dataDir string) (string, error) {
	if strings.TrimSpace(dataDir) == "" || strings.ContainsRune(dataDir, 0) {
		return "", fmt.Errorf("validate settings %q: data_dir must be a non-empty path without NUL bytes", path)
	}
	if !filepath.IsAbs(dataDir) {
		dataDir = filepath.Join(filepath.Dir(path), dataDir)
	}
	resolved, err := filepath.Abs(dataDir)
	if err != nil {
		return "", fmt.Errorf("resolve settings %q data_dir: %w", path, err)
	}
	return resolved, nil
}

// Field is one settings field or section named by dotted JSON keys.
type Field struct {
	Keys []string
	Type reflect.Type
	// root is the settings type the field belongs to, whose field order Set
	// writes the document in.
	root reflect.Type
}

// FieldOf finds the field that name selects in the settings type root.
func FieldOf(root reflect.Type, name string) (Field, error) {
	keys := strings.Split(name, ".")
	current := root
	for _, key := range keys {
		if current.Kind() != reflect.Struct {
			return Field{}, errors.New("field is not a settings section")
		}
		var next reflect.Type
		for index := range current.NumField() {
			field := current.Field(index)
			if field.IsExported() && field.Tag.Get("json") == key && key != "-" {
				next = field.Type
				break
			}
		}
		if next == nil {
			return Field{}, errors.New("unknown settings field")
		}
		current = next
	}
	return Field{Keys: keys, Type: current, root: root}, nil
}

// Encode turns a command-line value into the field's JSON: strings are taken
// literally, other types must be strict JSON of the field's type. Errors
// never echo the value, which may be a credential.
func (field Field) Encode(raw string) (json.RawMessage, error) {
	if field.Type.Kind() == reflect.String {
		return json.Marshal(raw)
	}
	if strings.TrimSpace(raw) == "null" && field.Type.Kind() != reflect.Pointer && field.Type.Kind() != reflect.Slice {
		return nil, errors.New("settings field does not accept null")
	}
	if err := jsonstrict.Decode([]byte(raw), MaximumBytes, reflect.New(field.Type).Interface()); err != nil {
		return nil, errors.New("settings field value must be valid JSON of the declared type")
	}
	return json.RawMessage(raw), nil
}

// Get returns the field's stored value from a settings document.
func (field Field) Get(data []byte) (json.RawMessage, error) {
	current, err := object(data)
	if err != nil {
		return nil, err
	}
	for index, key := range field.Keys {
		value, exists := current[key]
		if !exists {
			return nil, errors.New("settings field is not present in the file")
		}
		if index == len(field.Keys)-1 {
			return value, nil
		}
		var section map[string]json.RawMessage
		if err := json.Unmarshal(value, &section); err != nil || section == nil {
			return nil, fmt.Errorf("settings section %q must be a JSON object", key)
		}
		current = section
	}
	return nil, errors.New("unknown settings field")
}

// Set returns the settings document with the field replaced by value and
// every other field kept as stored, written in the settings type's field
// order.
func (field Field) Set(data []byte, value json.RawMessage) ([]byte, error) {
	current, err := object(data)
	if err != nil {
		return nil, err
	}
	if err := set(current, field.Keys, value); err != nil {
		return nil, err
	}
	compact, err := json.Marshal(current)
	if err != nil {
		return nil, err
	}
	return Format(field.root, compact)
}

// Format returns the settings document data indented by two spaces and
// ending in a newline, with each object's keys in the order the settings
// type root declares them, the order a settings value encodes in, so every
// writer of a settings file leaves the same order. Keys root does not
// declare follow in sorted order, and every value keeps its content.
func Format(root reflect.Type, data []byte) ([]byte, error) {
	ordered, err := inFieldOrder(root, data)
	if err != nil {
		return nil, err
	}
	indented, err := json.MarshalIndent(json.RawMessage(ordered), "", "  ")
	if err != nil {
		return nil, err
	}
	return append(indented, '\n'), nil
}

// inFieldOrder returns the JSON value raw, which encodes a value of kind,
// with the keys of every object that encodes a struct in the struct's field
// order and the keys of a map, or ones kind does not declare, sorted.
func inFieldOrder(kind reflect.Type, raw json.RawMessage) (json.RawMessage, error) {
	for kind != nil && kind.Kind() == reflect.Pointer {
		kind = kind.Elem()
	}
	value := bytes.TrimSpace(raw)
	if kind == nil || len(value) == 0 {
		return raw, nil
	}
	switch {
	case value[0] == '{' && (kind.Kind() == reflect.Struct || kind.Kind() == reflect.Map):
		var members map[string]json.RawMessage
		if err := json.Unmarshal(value, &members); err != nil {
			return nil, err
		}
		var output bytes.Buffer
		output.WriteByte('{')
		write := func(key string, member reflect.Type) error {
			ordered, err := inFieldOrder(member, members[key])
			if err != nil {
				return err
			}
			name, err := json.Marshal(key)
			if err != nil {
				return err
			}
			if output.Len() > 1 {
				output.WriteByte(',')
			}
			output.Write(name)
			output.WriteByte(':')
			output.Write(ordered)
			delete(members, key)
			return nil
		}
		var rest reflect.Type
		if kind.Kind() == reflect.Map {
			rest = kind.Elem()
		} else {
			for index := range kind.NumField() {
				field := kind.Field(index)
				name, _, _ := strings.Cut(field.Tag.Get("json"), ",")
				if name == "" {
					name = field.Name
				}
				if _, present := members[name]; !present || !field.IsExported() || name == "-" {
					continue
				}
				if err := write(name, field.Type); err != nil {
					return nil, err
				}
			}
		}
		for _, key := range slices.Sorted(maps.Keys(members)) {
			if err := write(key, rest); err != nil {
				return nil, err
			}
		}
		output.WriteByte('}')
		return output.Bytes(), nil
	case value[0] == '[' && (kind.Kind() == reflect.Slice || kind.Kind() == reflect.Array):
		var items []json.RawMessage
		if err := json.Unmarshal(value, &items); err != nil {
			return nil, err
		}
		output := []byte{'['}
		for index, item := range items {
			ordered, err := inFieldOrder(kind.Elem(), item)
			if err != nil {
				return nil, err
			}
			if index > 0 {
				output = append(output, ',')
			}
			output = append(output, ordered...)
		}
		return append(output, ']'), nil
	}
	return raw, nil
}

func object(data []byte) (map[string]json.RawMessage, error) {
	var current map[string]json.RawMessage
	if err := jsonstrict.Decode(data, MaximumBytes, &current); err != nil || current == nil {
		return nil, errors.New("settings must be a valid JSON object")
	}
	return current, nil
}

func set(current map[string]json.RawMessage, keys []string, value json.RawMessage) error {
	key := keys[0]
	if len(keys) == 1 {
		current[key] = value
		return nil
	}
	child := make(map[string]json.RawMessage)
	if raw, exists := current[key]; exists {
		if err := json.Unmarshal(raw, &child); err != nil {
			return fmt.Errorf("settings section %q must be a JSON object", key)
		}
		if child == nil {
			child = make(map[string]json.RawMessage)
		}
	}
	if err := set(child, keys[1:], value); err != nil {
		return err
	}
	encoded, err := json.Marshal(child)
	if err != nil {
		return err
	}
	current[key] = encoded
	return nil
}
