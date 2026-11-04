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

func TestPanelBackupRestoresBothSourcesAndExactConfigurationText(t *testing.T) {
	source, target := panelFileApp(t), panelFileApp(t)
	text := "{\n  \"large\": 900719925474099312345,\n  \"unfinished\": "
	if _, err := saveConfigurationFile(t.Context(), source, text); err != nil {
		t.Fatal(err)
	}
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
	native.Panel.PublicNodeHost = "restored.example.com"
	native.Server.Port = 8080
	backup.PanelSettings, _ = json.Marshal(native)
	view, _ := target.PanelSettings(t.Context())
	file, _ := target.database.ConfigurationFile(t.Context())
	result, err := target.RestorePanelBackup(t.Context(), PanelRestoreRequest{Backup: backup, SettingsRevision: view.Revision, ConfigurationRevision: file.Revision})
	if err != nil {
		t.Fatal(err)
	}
	if !result.ReauthenticationRequired || !result.Settings.RestartRequired || result.Settings.Preferences.ListenPort != 8080 {
		t.Fatalf("restore impact: %+v", result)
	}
	got, err := target.database.ConfigurationFile(t.Context())
	if err != nil || got.Content != text || got.CanonicalRevisionID != "" {
		t.Fatalf("configuration text changed: %+v %v", got, err)
	}
	loaded, err := Load(target.settingsPath)
	if err != nil || loaded.Auth.PasswordHash != native.Auth.PasswordHash || loaded.DataDir != native.DataDir || loaded.Panel.PublicNodeHost != native.Panel.PublicNodeHost {
		t.Fatal("incomplete settings restore", err)
	}
	state, err := target.database.Bootstrap(t.Context())
	if err != nil || state.Hub.DesiredRunning || state.Hub.AppliedBundleID != "" {
		t.Fatal("restore changed runtime intent", err)
	}
}

func TestPanelBackupConflictsAndInvalidInputLeaveBothSourcesUntouched(t *testing.T) {
	for _, kind := range []string{"settings_conflict", "configuration_conflict", "unsupported", "invalid_settings", "identity_name", "identity_key", "oversize", "removed_author", "removed_provider", "removed_retention", "removed_cookie"} {
		t.Run(kind, func(t *testing.T) {
			app := panelFileApp(t)
			original, err := saveConfigurationFile(t.Context(), app, `{"log":{"level":"info"}}`)
			if err != nil {
				t.Fatal(err)
			}
			backup, _ := app.ExportPanelBackup(t.Context())
			backup.SingBoxConfiguration = `{"log":{"level":"debug"}}`
			view, _ := app.PanelSettings(t.Context())
			request := PanelRestoreRequest{Backup: backup, SettingsRevision: view.Revision, ConfigurationRevision: original.Revision}
			switch kind {
			case "settings_conflict":
				request.SettingsRevision++
			case "configuration_conflict":
				request.ConfigurationRevision++
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
				request.Backup.SingBoxConfiguration = strings.Repeat("x", 2<<20+1)
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
			_, err = app.RestorePanelBackup(t.Context(), request)
			if err == nil {
				t.Fatal("invalid restore succeeded")
			}
			if (strings.HasPrefix(kind, "removed_") || kind == "identity_name" || kind == "identity_key") && !errors.Is(err, ErrPanelBackupInvalid) {
				t.Fatalf("removed field was not rejected as an invalid backup: %v", err)
			}
			if kind == "configuration_conflict" && !errors.Is(err, store.ErrConfigurationFileConflict) {
				t.Fatal(err)
			}
			after, _ := Read(app.settingsPath)
			file, _ := app.database.ConfigurationFile(t.Context())
			if !bytes.Equal(before, after) || file.Content != original.Content || file.Revision != original.Revision {
				t.Fatal("failed restore partially changed persisted configuration")
			}
		})
	}
}

func TestInterruptedBackupRestoreRecoversBothSources(t *testing.T) {
	for _, committed := range []bool{false, true} {
		t.Run(map[bool]string{false: "rollback", true: "commit"}[committed], func(t *testing.T) {
			app := panelFileApp(t)
			original, err := saveConfigurationFile(t.Context(), app, "original raw text")
			if err != nil {
				t.Fatal(err)
			}
			before, _ := Read(app.settingsPath)
			next, _ := Load(app.settingsPath)
			next.Panel.Language = "en"
			after, _ := json.Marshal(next)
			journal, _ := json.Marshal(settingsJournal{ID: "backup-interrupted", Before: before, After: after})
			if err := WriteAtomic(app.settingsPath+".pending", journal); err != nil {
				t.Fatal(err)
			}
			if err := ReplaceLocked(app.settingsPath, after); err != nil {
				t.Fatal(err)
			}
			if committed {
				update, err := store.NewConfigurationFileUpdate(app.random, app.now(), original.Revision, "restored raw text")
				if err != nil {
					t.Fatal(err)
				}
				if err := app.database.CommitPanelSettingsFile(t.Context(), app.settingsPath, "backup-interrupted", update, false, func() error { return nil }); err != nil {
					t.Fatal(err)
				}
			}
			if err := app.RecoverPanelSettingsFile(t.Context()); err != nil {
				t.Fatal(err)
			}
			view, _ := app.PanelSettings(t.Context())
			config, _ := app.database.ConfigurationFile(t.Context())
			wantLanguage, wantText := "zh-CN", "original raw text"
			if committed {
				wantLanguage, wantText = "en", "restored raw text"
			}
			if view.Preferences.Language != wantLanguage || config.Content != wantText {
				t.Fatalf("partial recovery: %s %s", view.Preferences.Language, config.Content)
			}
		})
	}
}
