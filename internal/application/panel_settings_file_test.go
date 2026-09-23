// SPDX-License-Identifier: GPL-3.0-or-later

package application

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/rehuony/sing-box-panel/internal/settings"
	"github.com/rehuony/sing-box-panel/internal/store"
)

func panelFileApp(t *testing.T) *Application {
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

func TestPanelSettingsFileSharedWithManualAndCLIChanges(t *testing.T) {
	app := panelFileApp(t)
	view, err := app.PanelSettings(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	before, _ := settings.Read(app.settingsPath)
	value, _ := settings.Load(app.settingsPath)
	value.Panel.Language = "en"
	value.Panel.PublicNodeHost = "nodes.example.com"
	value.Subscription.Provider = "custom"
	value.Server.BasePath = "/panel"
	quota := int64(8589934591)
	value.Traffic.QuotaGiB = &quota
	raw, _ := json.Marshal(value)
	if err := settings.Replace(app.settingsPath, raw); err != nil {
		t.Fatal(err)
	}
	if _, err := app.SavePanelSettings(t.Context(), PanelSettingsWrite{Revision: view.Revision, Preferences: view.Preferences}); !errors.Is(err, store.ErrPanelSettingsConflict) {
		t.Fatalf("stale Web save overwrote file: %v", err)
	}
	view, err = app.PanelSettings(t.Context())
	if err != nil || view.Preferences.Language != "en" || view.Preferences.PublicNodeHost != "nodes.example.com" || !view.RestartRequired {
		t.Fatalf("file edit invisible: %+v %v", view, err)
	}
	view.Preferences.Appearance.Radius = 0
	if _, err := app.SavePanelSettings(t.Context(), PanelSettingsWrite{Revision: view.Revision, Preferences: view.Preferences}); err != nil {
		t.Fatal(err)
	}
	after, _ := settings.Read(app.settingsPath)
	loaded, err := settings.Load(app.settingsPath)
	if err != nil || loaded.Panel.Appearance.Radius != 0 || loaded.Subscription.Provider != "custom" || loaded.Server.BasePath != "/panel" || bytes.Equal(before, after) {
		t.Fatal("Web save did not preserve complete file", err)
	}
	// Formatting-only external edits also invalidate the opaque revision.
	view, _ = app.PanelSettings(t.Context())
	if err := os.WriteFile(app.settingsPath, append(after, '\n'), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := app.SavePanelSettings(t.Context(), PanelSettingsWrite{Revision: view.Revision, Preferences: view.Preferences}); !errors.Is(err, store.ErrPanelSettingsConflict) {
		t.Fatal("manual edit was not detected", err)
	}
}

func TestConcurrentWebSettingsFileWritesUseCAS(t *testing.T) {
	app := panelFileApp(t)
	view, _ := app.PanelSettings(t.Context())
	var wg sync.WaitGroup
	var results [2]error
	for i := range results {
		wg.Go(func() {
			p := view.Preferences
			p.Appearance.Radius = i
			_, results[i] = app.SavePanelSettings(t.Context(), PanelSettingsWrite{Revision: view.Revision, Preferences: p})
		})
	}
	wg.Wait()
	success, conflicts := 0, 0
	for _, err := range results {
		if err == nil {
			success++
		} else if errors.Is(err, store.ErrPanelSettingsConflict) {
			conflicts++
		} else {
			t.Fatal(err)
		}
	}
	if success != 1 || conflicts != 1 {
		t.Fatalf("success=%d conflict=%d", success, conflicts)
	}
}

func TestStartupRecoveryPreservesSelectedSettingsFile(t *testing.T) {
	app := panelFileApp(t)
	before, err := settings.Read(app.settingsPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := app.RecoverPanelSettingsFile(t.Context()); err != nil {
		t.Fatal(err)
	}
	after, err := settings.Read(app.settingsPath)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatalf("startup rewrote settings: %v", err)
	}
}

func TestSettingsFileJournalRecoversCommitAndRollback(t *testing.T) {
	for _, committed := range []bool{false, true} {
		for _, published := range []bool{false, true} {
			t.Run(strings.Join([]string{map[bool]string{false: "rollback", true: "commit"}[committed], map[bool]string{false: "before publish", true: "after publish"}[published]}, "/"), func(t *testing.T) {
				app := panelFileApp(t)
				before, _ := settings.Read(app.settingsPath)
				value, _ := settings.Load(app.settingsPath)
				value.Panel.Language = "en"
				after, _ := json.Marshal(value)
				journal, _ := json.Marshal(settingsJournal{ID: "interrupted", Before: before, After: after})
				if err := settings.WriteAtomic(app.settingsPath+".pending", journal); err != nil {
					t.Fatal(err)
				}
				if published {
					if err := settings.ReplaceLocked(app.settingsPath, after); err != nil {
						t.Fatal(err)
					}
				}
				if committed {
					if err := app.database.CommitPanelSettingsFile(t.Context(), app.settingsPath, "interrupted", nil, func() error { return nil }); err != nil {
						t.Fatal(err)
					}
				}
				if _, err := settings.Load(app.settingsPath); !errors.Is(err, settings.ErrPending) {
					t.Fatal("file check exposed incomplete transaction", err)
				}
				if err := settings.Replace(app.settingsPath, before); !errors.Is(err, settings.ErrPending) {
					t.Fatal("CLI replaced pending update", err)
				}
				view, err := app.PanelSettings(t.Context())
				if err != nil {
					t.Fatal(err)
				}
				want := "zh-CN"
				if committed {
					want = "en"
				}
				if view.Preferences.Language != want {
					t.Fatalf("recovered %s want %s", view.Preferences.Language, want)
				}
				if _, err := os.Stat(app.settingsPath + ".pending"); !errors.Is(err, os.ErrNotExist) {
					t.Fatal("recovery journal retained", err)
				}
			})
		}
	}
}

func TestSettingsFileRecoveryPreservesConflictingManualEdit(t *testing.T) {
	app := panelFileApp(t)
	before, _ := settings.Read(app.settingsPath)
	value, _ := settings.Load(app.settingsPath)
	value.Panel.Language = "en"
	after, _ := json.Marshal(value)
	journal, _ := json.Marshal(settingsJournal{ID: "interrupted", Before: before, After: after})
	if err := settings.WriteAtomic(app.settingsPath+".pending", journal); err != nil {
		t.Fatal(err)
	}
	edited := append(append([]byte{}, before...), '\n')
	if err := os.WriteFile(app.settingsPath, edited, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := app.PanelSettings(t.Context()); err == nil {
		t.Fatal("external recovery conflict ignored")
	}
	current, _ := os.ReadFile(app.settingsPath)
	if !bytes.Equal(current, edited) {
		t.Fatal("recovery clobbered manual edit")
	}
}

func TestSettingsFilePermissionFailureDoesNotChangeIdentity(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("requires unprivileged permissions")
	}
	app := panelFileApp(t)
	view, _ := app.PanelSettings(t.Context())
	before, _ := app.ConfigurationFile(t.Context())
	dir := filepath.Dir(app.settingsPath)
	if err := os.Chmod(dir, 0500); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(dir, 0700)
	if _, err := app.SavePanelSettings(context.Background(), PanelSettingsWrite{Revision: view.Revision, Preferences: view.Preferences, IdentityKey: "not-saved"}); err == nil {
		t.Fatal("read-only settings saved")
	}
	after, _ := app.ConfigurationFile(t.Context())
	if before.Content != after.Content || before.Revision != after.Revision {
		t.Fatal("failed file save changed configuration")
	}
}

func TestWebSettingsSaveKeepsRelativeDataPath(t *testing.T) {
	app := panelFileApp(t)
	value := app.settings
	value.DataDir = "../data"
	raw, _ := json.Marshal(value)
	if err := settings.Replace(app.settingsPath, raw); err != nil {
		t.Fatal(err)
	}
	view, err := app.PanelSettings(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	view.Preferences.Language = "en"
	if _, err := app.SavePanelSettings(t.Context(), PanelSettingsWrite{Revision: view.Revision, Preferences: view.Preferences}); err != nil {
		t.Fatal(err)
	}
	actual, err := settings.Read(app.settingsPath)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(actual, &fields); err != nil {
		t.Fatal(err)
	}
	if string(fields["data_dir"]) != `"../data"` {
		t.Fatalf("relative path rewritten: %s", fields["data_dir"])
	}
}

func TestSettingsRecoveryRejectsSymlinkJournal(t *testing.T) {
	app := panelFileApp(t)
	target := filepath.Join(t.TempDir(), "unrelated")
	if err := os.WriteFile(target, []byte("unchanged"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, app.settingsPath+".pending"); err != nil {
		t.Fatal(err)
	}
	if _, err := app.PanelSettings(t.Context()); err == nil {
		t.Fatal("followed journal symlink")
	}
	actual, err := os.ReadFile(target)
	if err != nil || string(actual) != "unchanged" {
		t.Fatal("changed unrelated file", err)
	}
}

func TestSettingsRecoveryUsesCommitIdentityAcrossPathAliases(t *testing.T) {
	app := panelFileApp(t)
	before, _ := settings.Read(app.settingsPath)
	value, _ := settings.Load(app.settingsPath)
	value.Panel.Language = "en"
	after, _ := json.Marshal(value)
	journal, _ := json.Marshal(settingsJournal{ID: "committed-through-original-path", Before: before, After: after})
	if err := settings.WriteAtomic(app.settingsPath+".pending", journal); err != nil {
		t.Fatal(err)
	}
	if err := app.database.CommitPanelSettingsFile(t.Context(), app.settingsPath, "committed-through-original-path", nil, func() error {
		return settings.ReplaceLocked(app.settingsPath, after)
	}); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(t.TempDir(), "alias")
	if err := os.Symlink(filepath.Dir(app.settingsPath), alias); err != nil {
		t.Fatal(err)
	}
	app.settingsPath = filepath.Join(alias, filepath.Base(app.settingsPath))
	view, err := app.PanelSettings(t.Context())
	if err != nil || view.Preferences.Language != "en" {
		t.Fatalf("committed settings rolled back through alias: %+v %v", view, err)
	}
}

func TestPanelServiceSettingsRoundTripAndValidation(t *testing.T) {
	app := panelFileApp(t)
	ctx := t.Context()
	view, err := app.PanelSettings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	originalDir := view.Service.DataDir
	service := PanelServiceSettings{
		DataDir: filepath.Join(t.TempDir(), "panel-data"), BasePath: "/control", SecureCookie: true,
		CatalogRefreshIntervalHours: 24, TrafficPeriodMonths: 3, SampleRetentionDays: 120,
		SubscriptionAuthor: "Example", SubscriptionProvider: "Custom", PrivateSourceCIDRs: []string{"10.0.0.0/24", "fd00::/64"}, LogRetentionDays: 30,
	}
	view.Preferences.ExternalOrigin = "https://panel.example.com"
	input := PanelSettingsWrite{Revision: view.Revision, Preferences: view.Preferences, Service: &service, IdentityKey: "private-key"}
	saved, err := app.SavePanelSettings(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	service.CoreLogRetentionDays, service.CoreLogMaxFiles, service.CoreLogMaxFileSizeMiB = view.Service.CoreLogRetentionDays, view.Service.CoreLogMaxFiles, view.Service.CoreLogMaxFileSizeMiB
	if !reflect.DeepEqual(saved.Service, service) || !saved.RestartRequired || !saved.IdentityKeyConfigured {
		t.Fatalf("service settings not exposed: %+v", saved)
	}
	loaded, err := settings.Load(app.settingsPath)
	if err != nil || !reflect.DeepEqual(serviceSettings(loaded), service) {
		t.Fatalf("file round trip: %+v %v", loaded, err)
	}
	if app.settings.DataDir != originalDir {
		t.Fatal("running data directory changed before restart")
	}
	before, _ := settings.Read(app.settingsPath)
	invalid := service
	invalid.PrivateSourceCIDRs = []string{"not-a-network"}
	input.Revision, input.Service, input.IdentityKey = saved.Revision, &invalid, ""
	if _, err := app.SavePanelSettings(ctx, input); !errors.Is(err, ErrPanelSettingsInvalid) {
		t.Fatalf("invalid network accepted: %v", err)
	}
	after, _ := settings.Read(app.settingsPath)
	if !bytes.Equal(before, after) {
		t.Fatal("failed save changed settings file")
	}
	input.Service, input.ClearIdentityKey = &service, true
	cleared, err := app.SavePanelSettings(ctx, input)
	if err != nil || cleared.IdentityKeyConfigured {
		t.Fatalf("clear identity: %+v %v", cleared, err)
	}
	input.Revision, input.Service, input.ClearIdentityKey = cleared.Revision, nil, false
	input.Preferences.Appearance.Radius = 4
	preserved, err := app.SavePanelSettings(ctx, input)
	if err != nil || !reflect.DeepEqual(preserved.Service, service) {
		t.Fatalf("older client lost service settings: %+v %v", preserved, err)
	}
}

func TestCatalogRefreshIntervalAppliesWithoutRestart(t *testing.T) {
	app := panelFileApp(t)
	view, err := app.PanelSettings(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	view.Service.CatalogRefreshIntervalHours = 24
	saved, err := app.SavePanelSettings(t.Context(), PanelSettingsWrite{
		Revision:    view.Revision,
		Preferences: view.Preferences,
		Service:     &view.Service,
	})
	if err != nil {
		t.Fatal(err)
	}
	if saved.RestartRequired || saved.Service.CatalogRefreshIntervalHours != 24 {
		t.Fatalf("dynamic catalog refresh interval result=%+v", saved)
	}
}
