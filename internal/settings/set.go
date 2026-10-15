// SPDX-License-Identifier: GPL-3.0-or-later

package settings

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"

	"github.com/rehuony/sing-box-panel/internal/auth"
	"github.com/rehuony/sing-box-panel/internal/jsonstrict"
)

// SetField updates one field or section selected by its dotted JSON keys without
// materializing omitted defaults or resolving literal paths. Strings are supplied
// literally; other values use JSON. The virtual auth.password field accepts
// plaintext and saves only its hash.
func SetField(ctx context.Context, path, field, rawValue string) error {
	tokens := settingsFieldPath(field)
	if len(tokens) == 2 && tokens[0] == "auth" && tokens[1] == "password" {
		hash, err := auth.HashPassword(ctx, rawValue)
		if err != nil {
			return err
		}
		tokens[1], rawValue = "password_hash", hash
	}
	fieldType, err := settingsFieldType(tokens)
	if err != nil {
		return err
	}
	var value json.RawMessage
	if fieldType.Kind() == reflect.String {
		value, err = json.Marshal(rawValue)
	} else {
		value = json.RawMessage(rawValue)
		if strings.TrimSpace(rawValue) == "null" && fieldType.Kind() != reflect.Pointer && fieldType.Kind() != reflect.Slice {
			return errors.New("settings field does not accept null")
		}
		if err := jsonstrict.Decode(value, MaximumBytes, reflect.New(fieldType).Interface()); err != nil {
			// Decoder errors can contain supplied values or keys. Never echo them:
			// section values may include credentials as well as ordinary settings.
			return errors.New("settings field value must be valid JSON of the declared type")
		}
	}
	if err != nil {
		return err
	}
	lock, err := Lock(ctx, path)
	if err != nil {
		return err
	}
	defer lock.Close()
	before, err := Read(path)
	if err != nil {
		return err
	}
	var current map[string]json.RawMessage
	if err := jsonstrict.Decode(before, MaximumBytes, &current); err != nil || current == nil {
		return errors.New("panel settings must be a valid JSON object")
	}
	if err := setField(current, tokens, value); err != nil {
		return err
	}
	after, err := json.MarshalIndent(current, "", "  ")
	if err != nil {
		return err
	}
	after = append(after, '\n')
	if _, err := parse(path, after); err != nil {
		// Validation can include invalid URLs, CIDRs, and other input values.
		return errors.New("update would invalidate panel settings; check the field type and allowed values")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := rememberDataLocationBeforeReplace(path); err != nil {
		return err
	}
	return ReplaceLocked(path, after)
}

func settingsFieldPath(field string) []string {
	return strings.Split(field, ".")
}

func settingsFieldType(tokens []string) (reflect.Type, error) {
	current := reflect.TypeFor[Settings]()
	for _, name := range tokens {
		if current.Kind() != reflect.Struct {
			return nil, errors.New("field is not a settings section")
		}
		var next reflect.Type
		for index := range current.NumField() {
			field := current.Field(index)
			if field.IsExported() && field.Tag.Get("json") == name && name != "-" {
				next = field.Type
				break
			}
		}
		if next == nil {
			return nil, errors.New("unknown settings field")
		}
		current = next
	}
	return current, nil
}

func setField(current map[string]json.RawMessage, tokens []string, value json.RawMessage) error {
	name := tokens[0]
	if len(tokens) == 1 {
		current[name] = value
		return nil
	}
	child := make(map[string]json.RawMessage)
	if raw, exists := current[name]; exists {
		if err := json.Unmarshal(raw, &child); err != nil {
			return fmt.Errorf("settings section %q must be a JSON object", name)
		}
		if child == nil {
			child = make(map[string]json.RawMessage)
		}
	}
	if err := setField(child, tokens[1:], value); err != nil {
		return err
	}
	encoded, err := json.Marshal(child)
	if err != nil {
		return err
	}
	current[name] = encoded
	return nil
}
