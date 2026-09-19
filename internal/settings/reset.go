// SPDX-License-Identifier: GPL-3.0-or-later

package settings

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/rehuony/sing-box-panel/internal/jsonstrict"
)

// ResetFields restores named fields or sections from Defaults in one validated
// write. The writer lock covers reading and editing, so concurrent cooperating
// writers cannot overwrite unrelated changes. No database is opened.
func ResetFields(ctx context.Context, path string, fields []string) error {
	if len(fields) == 0 {
		return errors.New("at least one settings field is required")
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
	if err := jsonstrict.Decode(before, MaximumBytes, &current); err != nil {
		return err
	}
	if current == nil {
		return errors.New("panel settings must be a JSON object")
	}
	encodedDefaults, err := json.Marshal(Defaults())
	if err != nil {
		return err
	}
	var defaults map[string]json.RawMessage
	if err := json.Unmarshal(encodedDefaults, &defaults); err != nil {
		return err
	}
	for _, field := range fields {
		tokens := strings.Split(field, ".")
		if strings.HasPrefix(field, "/") {
			tokens = strings.Split(field[1:], "/")
		}
		if err := resetField(current, defaults, tokens); err != nil {
			return fmt.Errorf("reset settings field %q: %w", field, err)
		}
	}
	after, err := json.MarshalIndent(current, "", "  ")
	if err != nil {
		return err
	}
	after = append(after, '\n')
	if _, err := parse(path, after); err != nil {
		return fmt.Errorf("reset would invalidate panel settings: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := rememberDataLocationBeforeReplace(path); err != nil {
		return err
	}
	return ReplaceLocked(path, after)
}

func resetField(current, defaults map[string]json.RawMessage, tokens []string) error {
	name := tokens[0]
	value, exists := defaults[name]
	if !exists {
		return errors.New("unknown settings field")
	}
	if len(tokens) == 1 {
		current[name] = value
		return nil
	}
	var defaultChild map[string]json.RawMessage
	if err := json.Unmarshal(value, &defaultChild); err != nil || defaultChild == nil {
		return errors.New("field is not a settings section")
	}
	child := make(map[string]json.RawMessage)
	if raw, exists := current[name]; exists {
		if err := json.Unmarshal(raw, &child); err != nil {
			return fmt.Errorf("reset the containing section instead: %w", err)
		}
		if child == nil {
			child = make(map[string]json.RawMessage)
		}
	}
	if err := resetField(child, defaultChild, tokens[1:]); err != nil {
		return err
	}
	encoded, err := json.Marshal(child)
	if err != nil {
		return err
	}
	current[name] = encoded
	return nil
}
