package subscription

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/settings"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/store"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/testutil"
)

func TestPublicNodeHostOverrideAndDetectionHint(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(ctx, filepath.Join(t.TempDir(), "panel.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	detected := "1.1.1.1"
	path := filepath.Join(t.TempDir(), "setting.json")
	value := settings.Defaults()
	value.Auth.Email, value.Auth.PasswordHash = testutil.AdminEmail, testutil.PasswordHash
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if err := settings.Replace(path, raw); err != nil {
		t.Fatal(err)
	}
	if value, err = settings.Load(path); err != nil {
		t.Fatal(err)
	}
	panelSettings := settings.NewService(db, path, value, settings.Options{
		PublicIP: func(context.Context) string { return detected },
	})
	app := New(db, panelSettings, nil, Options{PublicIP: func(context.Context) string { return detected }})
	view, err := panelSettings.PanelSettings(ctx)
	if err != nil || view.DetectedPublicIP != "1.1.1.1" || view.Preferences.PublicNodeHost != "" {
		t.Fatal(view, err)
	}
	got, err := app.publicationHost(ctx, store.SubscriptionNodeControls{}, "")
	if err != nil || got != "1.1.1.1" {
		t.Fatal(got, err)
	}
	view.Preferences.PublicNodeHost = "nodes.example.com"
	saved, err := panelSettings.SavePanelSettings(ctx, settings.PanelSettingsWrite{Revision: view.Revision, Preferences: view.Preferences})
	if err != nil || saved.Settings.DetectedPublicIP != "1.1.1.1" {
		t.Fatal(saved, err)
	}
	got, err = app.publicationHost(ctx, store.SubscriptionNodeControls{}, "legacy.example.com")
	if err != nil || got != "nodes.example.com" {
		t.Fatal(got, err)
	}
	detected = ""
	got, err = app.publicationHost(ctx, store.SubscriptionNodeControls{}, "")
	if err != nil || got != "nodes.example.com" {
		t.Fatal("configured host lost when detection failed", got, err)
	}
}
