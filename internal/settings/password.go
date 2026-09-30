// SPDX-License-Identifier: GPL-3.0-or-later

package settings

import (
	"context"
	"encoding/json"

	"github.com/rehuony/sing-box-panel/internal/auth"
)

// ResetPassword changes only the password hash in an existing valid settings
// file. It does not open storage. Authentication rejects old fingerprints at its
// next boundary, using the same lock to serialize reads with this update.
func ResetPassword(ctx context.Context, path, password string) (Settings, error) {
	hash, err := auth.HashPassword(ctx, password)
	if err != nil {
		return Settings{}, err
	}
	lock, err := Lock(ctx, path)
	if err != nil {
		return Settings{}, err
	}
	defer lock.Close()
	before, err := Read(path)
	if err != nil {
		return Settings{}, err
	}
	value, err := Parse(path, before)
	if err != nil {
		return Settings{}, err
	}
	// Retain literal settings values, including a relative data_dir and omitted
	// defaults, instead of serializing the resolved runtime representation.
	var document map[string]json.RawMessage
	if err := json.Unmarshal(before, &document); err != nil {
		return Settings{}, err
	}
	var credentials map[string]json.RawMessage
	if err := json.Unmarshal(document["auth"], &credentials); err != nil {
		return Settings{}, err
	}
	credentials["password_hash"], err = json.Marshal(hash)
	if err != nil {
		return Settings{}, err
	}
	document["auth"], err = json.Marshal(credentials)
	if err != nil {
		return Settings{}, err
	}
	after, err := json.MarshalIndent(document, "", "  ")
	if err != nil {
		return Settings{}, err
	}
	if err := ctx.Err(); err != nil {
		return Settings{}, err
	}
	if err := ReplaceLocked(path, append(after, '\n')); err != nil {
		return Settings{}, err
	}
	value.Auth.PasswordHash = hash
	return value, nil
}
