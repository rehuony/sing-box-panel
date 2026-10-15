// SPDX-License-Identifier: GPL-3.0-or-later

package settings

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/rehuony/sing-box-panel/internal/jsonstrict"
)

// GetField returns a stored field or section selected by its dotted JSON keys.
// It neither validates runtime settings nor resolves paths or omitted defaults.
func GetField(ctx context.Context, path, field string) (json.RawMessage, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	tokens := settingsFieldPath(field)
	if len(tokens) == 2 && tokens[0] == "auth" && tokens[1] == "password" {
		return nil, errors.New("auth.password is write-only; plaintext passwords are not stored")
	}
	if _, err := settingsFieldType(tokens); err != nil {
		return nil, err
	}
	data, err := Read(path)
	if err != nil {
		return nil, err
	}
	var current map[string]json.RawMessage
	if err := jsonstrict.Decode(data, MaximumBytes, &current); err != nil || current == nil {
		return nil, errors.New("panel settings must be a valid JSON object")
	}
	for index, name := range tokens {
		value, exists := current[name]
		if !exists {
			return nil, errors.New("settings field is not present in the file")
		}
		if index == len(tokens)-1 {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			return value, nil
		}
		var section map[string]json.RawMessage
		if err := json.Unmarshal(value, &section); err != nil || section == nil {
			return nil, fmt.Errorf("settings section %q must be a JSON object", name)
		}
		current = section
	}
	return nil, errors.New("unknown settings field")
}
