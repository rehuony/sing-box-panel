// SPDX-License-Identifier: GPL-3.0-or-later
package settings

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/store"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/testutil"
)

func settingsFileFixture(t *testing.T, value Settings) Settings {
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
	if err := Replace(path, raw); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	return loaded
}

func newTestService(database *store.Store, value Settings) *Service {
	return NewService(database, value.Path(), value, Options{})
}

// saveConfigurationFile saves content as the first configuration file revision.
func saveConfigurationFile(ctx context.Context, app *Service, content string) (store.ConfigurationFile, error) {
	update, err := store.NewConfigurationFileUpdate(app.random, app.now(), 0, content)
	if err != nil {
		return store.ConfigurationFile{}, err
	}
	return app.database.SaveConfigurationFile(ctx, update.ExpectedRevision, update.Content, update.Revision)
}
