// SPDX-License-Identifier: GPL-3.0-or-later

package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/auth"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/settings"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/store"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/testutil"
)

func commandSettingsFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	dataDir := filepath.Join(root, "data")
	if err := os.Mkdir(dataDir, 0o700); err != nil {
		t.Fatal(err)
	}
	settingsPath := filepath.Join(root, "setting.json")
	settingsJSON := fmt.Sprintf(`{
      "server":{"host":"127.0.0.1","port":3000,"base_path":""},
      "data_dir":%q,
      "auth":{"email":"admin@example.com","password_hash":%q},
      "github":{"token":"","catalog_refresh_interval_hours":12},
      "traffic":{"quota_gib":null,"period_months":1,"sample_retention_days":90},
      "subscription":{"private_source_cidrs":[]},
      "logs":{"core_retention_days":7}
    }`, dataDir, testutil.PasswordHash)
	if err := os.WriteFile(settingsPath, []byte(settingsJSON), 0o600); err != nil {
		t.Fatal(err)
	}
	return settingsPath
}

// createPanelDatabaseFixture creates the panel database of the settings
// file at path.
func createPanelDatabaseFixture(t *testing.T, path string) {
	t.Helper()
	database, _ := openPanelFixture(t, path)
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}
}

// openPanelFixture opens the panel database of the settings file at path and
// serves that file the way local commands do.
func openPanelFixture(t *testing.T, path string) (*store.Store, *settings.Service) {
	t.Helper()
	dataDir, err := settings.LoadDataDir(path)
	if err != nil {
		t.Fatal(err)
	}
	database, err := store.Open(t.Context(), filepath.Join(dataDir, "panel.db"))
	if err != nil {
		t.Fatal(err)
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		t.Fatal(err)
	}
	return database, settings.NewService(database, absolute, settings.Settings{DataDir: dataDir}, settings.Options{})
}

func initialPasswordFromOutput(t *testing.T, output, hash string) string {
	t.Helper()
	var event map[string]any
	password := ""
	if json.Unmarshal([]byte(output), &event) == nil {
		password, _ = event["login_password"].(string)
	} else {
		match := regexp.MustCompile(`(?m)^  (?:Initial )?Password +([^\r\n]+)$`).FindStringSubmatch(output)
		if len(match) == 2 {
			password = match[1]
		}
	}
	matches, err := auth.VerifyPassword(t.Context(), password, hash)
	if err != nil || !matches || strings.Count(output, password) != 1 || strings.Contains(output, hash) {
		t.Fatal("initial password was not printed exactly once or does not match stored hash", err)
	}
	return password
}
