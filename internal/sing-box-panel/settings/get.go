// SPDX-License-Identifier: GPL-3.0-or-later

package settings

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"

	"github.com/rehuony/sing-box-panel/internal/settingsfile"
)

// GetField returns a stored field or section selected by its dotted JSON keys.
// It neither validates runtime settings nor resolves paths or omitted defaults.
func GetField(ctx context.Context, path, name string) (json.RawMessage, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if name == "auth.password" {
		return nil, errors.New("auth.password is write-only; plaintext passwords are not stored")
	}
	field, err := settingsfile.FieldOf(reflect.TypeFor[Settings](), name)
	if err != nil {
		return nil, err
	}
	data, err := Read(path)
	if err != nil {
		return nil, err
	}
	value, err := field.Get(data)
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return value, nil
}
