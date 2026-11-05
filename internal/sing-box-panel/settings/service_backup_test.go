package settings

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/store"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/testutil"
)

func TestPanelBackupRestoresSettings(t *testing.T) {
	source, target := panelFileApp(t), panelFileApp(t)
	backup, err := source.ExportPanelBackup(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{`"author"`, `"provider"`, `"retention_days"`, `"identity_name"`, `"identity_key"`, `"secure_cookie"`} {
		if bytes.Contains(backup.PanelSettings, []byte(field)) {
			t.Fatalf("backup contains removed field %s", field)
		}
	}
	var native Settings
	if err := json.Unmarshal(backup.PanelSettings, &native); err != nil {
		t.Fatal(err)
	}
	native.Auth.Email = testutil.AdminEmail
	native.Auth.PasswordHash = testutil.ChangedPasswordHash
	native.Panel.Language = "en"
	native.Server.Port = 8080
	backup.PanelSettings, _ = json.Marshal(native)
	view, _ := target.PanelSettings(t.Context())
	result, err := target.RestorePanelBackup(t.Context(), PanelRestoreRequest{Backup: backup, SettingsRevision: view.Revision})
	if err != nil {
		t.Fatal(err)
	}
	if !result.ReauthenticationRequired || !result.Settings.RestartRequired || result.Settings.Preferences.ListenPort != 8080 {
		t.Fatalf("restore impact: %+v", result)
	}
	loaded, err := Load(target.settingsPath)
	if err != nil || loaded.Auth.PasswordHash != native.Auth.PasswordHash || loaded.DataDir != native.DataDir || loaded.Panel.Language != native.Panel.Language {
		t.Fatal("incomplete settings restore", err)
	}
}

func TestPanelBackupConflictsAndInvalidInputLeaveSettingsUntouched(t *testing.T) {
	for _, kind := range []string{"settings_conflict", "unsupported", "invalid_settings", "identity_name", "identity_key", "oversize", "removed_author", "removed_provider", "removed_retention", "removed_cookie"} {
		t.Run(kind, func(t *testing.T) {
			app := panelFileApp(t)
			backup, _ := app.ExportPanelBackup(t.Context())
			view, _ := app.PanelSettings(t.Context())
			request := PanelRestoreRequest{Backup: backup, SettingsRevision: view.Revision}
			switch kind {
			case "settings_conflict":
				request.SettingsRevision++
			case "unsupported":
				request.Backup.Version = 999
			case "invalid_settings":
				request.Backup.PanelSettings = json.RawMessage(`{"data_dir":"bad"}`)
			case "identity_name", "identity_key", "removed_cookie":
				var fields map[string]any
				if err := json.Unmarshal(request.Backup.PanelSettings, &fields); err != nil {
					t.Fatal(err)
				}
				if kind == "removed_cookie" {
					fields["auth"].(map[string]any)["secure_cookie"] = false
				} else {
					fields["panel"].(map[string]any)[kind] = "removed"
				}
				request.Backup.PanelSettings, _ = json.Marshal(fields)
			case "oversize":
				request.Backup.PanelSettings = json.RawMessage(`"` + strings.Repeat("x", MaximumBytes) + `"`)
			case "removed_author", "removed_provider", "removed_retention":
				var fields map[string]json.RawMessage
				if err := json.Unmarshal(request.Backup.PanelSettings, &fields); err != nil {
					t.Fatal(err)
				}
				switch kind {
				case "removed_author":
					fields["subscription"] = json.RawMessage(`{"author":"old","private_source_cidrs":[]}`)
				case "removed_provider":
					fields["subscription"] = json.RawMessage(`{"provider":"old","private_source_cidrs":[]}`)
				case "removed_retention":
					fields["logs"] = json.RawMessage(`{"retention_days":7}`)
				}
				request.Backup.PanelSettings, _ = json.Marshal(fields)
			}
			before, _ := Read(app.settingsPath)
			_, err := app.RestorePanelBackup(t.Context(), request)
			if err == nil {
				t.Fatal("invalid restore succeeded")
			}
			if (strings.HasPrefix(kind, "removed_") || kind == "identity_name" || kind == "identity_key") && !errors.Is(err, ErrPanelBackupInvalid) {
				t.Fatalf("removed field was not rejected as an invalid backup: %v", err)
			}
			if kind == "settings_conflict" && !errors.Is(err, store.ErrPanelSettingsConflict) {
				t.Fatal(err)
			}
			after, _ := Read(app.settingsPath)
			if !bytes.Equal(before, after) {
				t.Fatal("failed restore changed the settings file")
			}
		})
	}
}
