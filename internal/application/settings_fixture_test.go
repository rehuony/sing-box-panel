// SPDX-License-Identifier: GPL-3.0-or-later
package application

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rehuony/sing-box-panel/internal/settings"
)

func settingsFileFixture(t *testing.T, value settings.Settings) settings.Settings {
	t.Helper()
	if value.Auth.Token == "" {
		value.Auth.Token = strings.Repeat("t", 32)
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
