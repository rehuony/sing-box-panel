// SPDX-License-Identifier: GPL-3.0-or-later

// Package settingsfile holds what the panel's and the node's settings files
// share: bounded reads, the writer lock, data-directory resolution and the
// dotted field names that the config commands read and write.
package settingsfile

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
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
	return Field{Keys: keys, Type: current}, nil
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
// every other field kept as stored.
func (field Field) Set(data []byte, value json.RawMessage) ([]byte, error) {
	current, err := object(data)
	if err != nil {
		return nil, err
	}
	if err := set(current, field.Keys, value); err != nil {
		return nil, err
	}
	after, err := json.MarshalIndent(current, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(after, '\n'), nil
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
