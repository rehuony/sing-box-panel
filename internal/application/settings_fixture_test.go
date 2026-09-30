// SPDX-License-Identifier: GPL-3.0-or-later
package application

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/rehuony/sing-box-panel/internal/settings"
	"github.com/rehuony/sing-box-panel/internal/testutil"
)

func settingsFileFixture(t *testing.T, value settings.Settings) settings.Settings {
	t.Helper()
	if value.Auth.PasswordHash == "" {
		value.Auth.Email = testutil.AdminEmail
		value.Auth.PasswordHash = testutil.PasswordHash
	}
	path := filepath.Join(t.TempDir(), "setting.json")
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if err := settings.Replace(path, raw); err != nil {
		t.Fatal(err)
	}
	loaded, err := settings.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	return loaded
}
