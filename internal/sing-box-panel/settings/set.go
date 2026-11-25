// SPDX-License-Identifier: GPL-3.0-or-later

package settings

import (
	"context"
	"errors"
	"reflect"

	"github.com/rehuony/sing-box-panel/internal/settingsfile"
	"github.com/rehuony/sing-box-panel/internal/webauth"
)

// SetField updates one field or section selected by its dotted JSON keys without
// materializing omitted defaults or resolving literal paths. Strings are supplied
// literally; other values use JSON. The virtual auth.password field accepts
// plaintext and saves only its hash.
func SetField(ctx context.Context, path, name, rawValue string) error {
	if name == "auth.password" {
		hash, err := webauth.HashPassword(ctx, rawValue)
		if err != nil {
			return err
		}
		name, rawValue = "auth.password_hash", hash
	}
	field, err := settingsfile.FieldOf(reflect.TypeFor[Settings](), name)
	if err != nil {
		return err
	}
	value, err := field.Encode(rawValue)
	if err != nil {
		return err
	}
	lock, err := settingsfile.Lock(ctx, path)
	if err != nil {
		return err
	}
	defer lock.Close()
	before, err := Read(path)
	if err != nil {
		return err
	}
	after, err := field.Set(upgradeDocument(before), value)
	if err != nil {
		return err
	}
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
