package application

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/rehuony/sing-box-panel/internal/settings"
	"github.com/rehuony/sing-box-panel/internal/store"
)

func TestPanelBackupRestoresBothSourcesAndExactConfigurationText(t *testing.T) {
	source, target := panelFileApp(t), panelFileApp(t)
	text := "{\n  \"large\": 900719925474099312345,\n  \"unfinished\": "
	if _, err := source.SaveConfigurationFile(t.Context(), ConfigurationFileWrite{Content: text}); err != nil {
		t.Fatal(err)
	}
	backup, err := source.ExportPanelBackup(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{`"author"`, `"provider"`, `"retention_days"`} {
		if bytes.Contains(backup.PanelSettings, []byte(field)) {
			t.Fatalf("backup contains removed field %s", field)
		}
	}
	var native settings.Settings
	if err := json.Unmarshal(backup.PanelSettings, &native); err != nil {
		t.Fatal(err)
	}
	native.Auth.Token = strings.Repeat("restored-token-", 3)
	native.Panel.IdentityName = "restored-identity"
	native.Panel.IdentityKey = "do-not-rewrite-inbounds"
	native.Server.Port = 8080
	backup.PanelSettings, _ = json.Marshal(native)
	view, _ := target.PanelSettings(t.Context())
	file, _ := target.ConfigurationFile(t.Context())
	result, err := target.RestorePanelBackup(t.Context(), PanelRestoreRequest{Backup: backup, SettingsRevision: view.Revision, ConfigurationRevision: file.Revision})
	if err != nil {
		t.Fatal(err)
	}
	if !result.ReauthenticationRequired || !result.Settings.RestartRequired || result.Settings.Preferences.ListenPort != 8080 {
		t.Fatalf("restore impact: %+v", result)
	}
	got, err := target.ConfigurationFile(t.Context())
	if err != nil || got.Content != text || got.SyntaxValid {
		t.Fatalf("configuration text changed: %+v %v", got, err)
	}
	loaded, err := settings.Load(target.settingsPath)
	if err != nil || loaded.Auth.Token != native.Auth.Token || loaded.DataDir != native.DataDir || loaded.Panel.IdentityKey != native.Panel.IdentityKey {
		t.Fatal("incomplete settings restore", err)
	}
	state, err := target.database.Bootstrap(t.Context())
	if err != nil || state.Hub.DesiredRunning || state.Hub.AppliedBundleID != "" {
		t.Fatal("restore changed runtime intent", err)
	}
}

func TestPanelBackupConflictsAndInvalidInputLeaveBothSourcesUntouched(t *testing.T) {
	for _, kind := range []string{"settings_conflict", "configuration_conflict", "unsupported", "invalid_settings", "oversize", "removed_author", "removed_provider", "removed_retention"} {
		t.Run(kind, func(t *testing.T) {
			app := panelFileApp(t)
			original, err := app.SaveConfigurationFile(t.Context(), ConfigurationFileWrite{Content: `{"log":{"level":"info"}}`})
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
			before, _ := settings.Read(app.settingsPath)
			_, err = app.RestorePanelBackup(t.Context(), request)
			if err == nil {
				t.Fatal("invalid restore succeeded")
			}
			if strings.HasPrefix(kind, "removed_") && !errors.Is(err, ErrPanelBackupInvalid) {
				t.Fatalf("removed field was not rejected as an invalid backup: %v", err)
			}
			if kind == "configuration_conflict" && !errors.Is(err, store.ErrConfigurationFileConflict) {
				t.Fatal(err)
			}
			after, _ := settings.Read(app.settingsPath)
			file, _ := app.ConfigurationFile(t.Context())
			if !bytes.Equal(before, after) || file.Content != original.Content || file.Revision != original.Revision {
				t.Fatal("failed restore partially changed persisted configuration")
			}
		})
	}
}

func TestQuotaVisibleWithoutAppliedCoreAndDynamicPoliciesDoNotRequireRestart(t *testing.T) {
	app := panelFileApp(t)
	view, err := app.PanelSettings(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	quota := int64(123)
	view.Preferences.TrafficQuotaGiB = &quota
	view.Service.TrafficPeriodMonths = 3
	view.Service.SampleRetentionDays = 2
	changes, cancel := app.SettingsChanges()
	defer cancel()
	saved, err := app.SavePanelSettings(t.Context(), PanelSettingsWrite{Revision: view.Revision, Preferences: view.Preferences, Service: &view.Service})
	if err != nil || saved.RestartRequired {
		t.Fatalf("dynamic settings: %+v %v", saved, err)
	}
	select {
	case <-changes:
	default:
		t.Fatal("settings consumers were not notified")
	}
	retention, err := app.EnforceTrafficSampleRetention(t.Context())
	if err != nil || app.now().UTC().Sub(retention.Cutoff) < 48*time.Hour || app.now().UTC().Sub(retention.Cutoff) > 48*time.Hour+time.Second {
		t.Fatalf("retention did not hot reload: %+v %v", retention, err)
	}
	metrics, err := app.Metrics(t.Context())
	if err != nil || metrics.QuotaBytes == nil || *metrics.QuotaBytes != quota<<30 || metrics.TrafficAvailable {
		t.Fatalf("quota without traffic: %+v %v", metrics, err)
	}
}

func TestInterruptedBackupRestoreRecoversBothSources(t *testing.T) {
	for _, committed := range []bool{false, true} {
		t.Run(map[bool]string{false: "rollback", true: "commit"}[committed], func(t *testing.T) {
			app := panelFileApp(t)
			original, err := app.SaveConfigurationFile(t.Context(), ConfigurationFileWrite{Content: "original raw text"})
			if err != nil {
				t.Fatal(err)
			}
			before, _ := settings.Read(app.settingsPath)
			next, _ := settings.Load(app.settingsPath)
			next.Panel.Language = "en"
			after, _ := json.Marshal(next)
			journal, _ := json.Marshal(settingsJournal{ID: "backup-interrupted", Before: before, After: after})
			if err := settings.WriteAtomic(app.settingsPath+".pending", journal); err != nil {
				t.Fatal(err)
			}
			if err := settings.ReplaceLocked(app.settingsPath, after); err != nil {
				t.Fatal(err)
			}
			if committed {
				update, err := app.configurationFileUpdate(ConfigurationFileWrite{Revision: original.Revision, Content: "restored raw text"})
				if err != nil {
					t.Fatal(err)
				}
				if err := app.database.CommitPanelSettingsFile(t.Context(), app.settingsPath, "backup-interrupted", update, func() error { return nil }); err != nil {
					t.Fatal(err)
				}
			}
			if err := app.RecoverPanelSettingsFile(t.Context()); err != nil {
				t.Fatal(err)
			}
			view, _ := app.PanelSettings(t.Context())
			config, _ := app.ConfigurationFile(t.Context())
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
