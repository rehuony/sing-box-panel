// SPDX-License-Identifier: GPL-3.0-or-later
package localnode

import (
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/settings"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/store"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/testutil"
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

func panelFileApp(t *testing.T) *Node {
	t.Helper()
	db, err := store.Open(t.Context(), filepath.Join(t.TempDir(), "panel.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	value := settings.Defaults()
	value.DataDir = filepath.Dir(db.Path())
	return FromStoreWithSettings(db, settingsFileFixture(t, value))
}

func newSubscriptionTestApplication(database *store.Store) *Node {
	app := newApplication(database)
	now := time.Date(2026, time.August, 26, 12, 0, 0, 0, time.UTC)
	app.now = func() time.Time { return now }
	sequence := byte(0)
	app.random = func(destination []byte) (int, error) {
		sequence++
		for index := range destination {
			destination[index] = sequence
		}
		return len(destination), nil
	}
	return app
}
