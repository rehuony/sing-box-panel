package application

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rehuony/sing-box-panel/internal/settings"
	"github.com/rehuony/sing-box-panel/internal/store"
	"github.com/rehuony/sing-box-panel/internal/testutil"
)

func TestPanelSettingsPersistCASAndRedact(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "panel.db")
	db, err := store.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	bootstrap := settings.Defaults()
	bootstrap.Auth.Email = testutil.AdminEmail
	bootstrap.Auth.PasswordHash = testutil.PasswordHash
	bootstrap.GitHub.Token = "github-original-secret"
	bootstrap = settingsFileFixture(t, bootstrap)
	app := FromStoreWithSettings(db, bootstrap)
	view, err := app.PanelSettings(ctx)
	if err != nil || view.Revision == 0 || view.Preferences.Appearance.Radius != 12 {
		t.Fatalf("defaults: %+v %v", view, err)
	}
	p := view.Preferences
	p.Appearance.Color = "#ABCDEF"
	p.Appearance.Radius = 0
	p.PublicNodeHost = "2001:db8::1"
	p.ExternalOrigin = "https://panel.example.com"
	input := PanelSettingsWrite{Preferences: p, Revision: view.Revision, Credentials: &CredentialsWrite{Email: new(testutil.AdminEmail), NewPassword: testutil.ChangedPassword}}
	saved, err := app.SavePanelSettings(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(saved)
	for _, secret := range []string{"github-original-secret", strings.Repeat("b", 32)} {
		if strings.Contains(string(data), secret) {
			t.Fatal("response contains secret")
		}
	}
	if !saved.Settings.GitHubTokenConfigured || saved.Settings.Revision == view.Revision {
		t.Fatalf("save: %+v", saved)
	}
	if _, err := app.SavePanelSettings(ctx, input); !errors.Is(err, store.ErrPanelSettingsConflict) {
		t.Fatalf("stale save: %v", err)
	}
	input.Revision = saved.Settings.Revision
	input.ClearGitHubToken = true
	input.Credentials = nil
	saved, err = app.SavePanelSettings(ctx, input)
	if err != nil || saved.Settings.GitHubTokenConfigured {
		t.Fatalf("clear: %+v %v", saved, err)
	}
	db2, err := store.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer db2.Close()
	reloaded := FromStoreWithSettings(db2, bootstrap)
	effective, err := reloaded.EffectiveSettings(ctx)
	if err != nil || effective.GitHub.Token != "" || effective.Auth.PasswordHash == testutil.PasswordHash {
		t.Fatal("effective credentials did not persist")
	}
	if effective.Server.ExternalOrigin != p.ExternalOrigin || !saved.Settings.RestartRequired {
		t.Fatal("HTTPS origin did not persist with a pending restart")
	}
	got, err := reloaded.PanelSettings(ctx)
	if err != nil || got.Preferences.Appearance.Color != "#ABCDEF" || got.Preferences.Appearance.Radius != 0 {
		t.Fatal("appearance did not persist")
	}
}

func TestPanelSettingsValidation(t *testing.T) {
	base := PanelSettingsWrite{Preferences: PanelPreferences{ListenHost: "127.0.0.1", ListenPort: 3000, Language: "zh-CN", Appearance: AppearanceSettings{Theme: "light", Color: "#6D4ED1", Radius: 24}}}
	cases := map[string]func(*PanelSettingsWrite){
		"wildcard published host": func(v *PanelSettingsWrite) { v.Preferences.PublicNodeHost = "0.0.0.0" },
		"loopback published host": func(v *PanelSettingsWrite) { v.Preferences.PublicNodeHost = "::1" },
		"URL instead of host":     func(v *PanelSettingsWrite) { v.Preferences.PublicNodeHost = "https://example.com" },
		"port in host":            func(v *PanelSettingsWrite) { v.Preferences.PublicNodeHost = "example.com:443" },
		"radius":                  func(v *PanelSettingsWrite) { v.Preferences.Appearance.Radius = 33 },
		"color":                   func(v *PanelSettingsWrite) { v.Preferences.Appearance.Color = "red" },
		"short password": func(v *PanelSettingsWrite) {
			v.Credentials = &CredentialsWrite{Email: new(testutil.AdminEmail), NewPassword: "short"}
		},
		"empty email":        func(v *PanelSettingsWrite) { v.Credentials = &CredentialsWrite{Email: new("")} },
		"invalid email":      func(v *PanelSettingsWrite) { v.Credentials = &CredentialsWrite{Email: new("invalid")} },
		"ambiguous secret":   func(v *PanelSettingsWrite) { v.GitHubToken = "secret"; v.ClearGitHubToken = true },
		"credential newline": func(v *PanelSettingsWrite) { v.GitHubToken = "bad\nsecret" },
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			v := base
			change(&v)
			if !errors.Is(validatePanelSettings(v), ErrPanelSettingsInvalid) {
				t.Fatal("accepted invalid settings")
			}
		})
	}
}

func TestPublicNodeHostOverrideAndDetectionHint(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(ctx, filepath.Join(t.TempDir(), "panel.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	app := FromStoreWithSettings(db, settingsFileFixture(t, settings.Defaults()))
	app.SetPublicIPResolver(func(context.Context) string { return "1.1.1.1" })
	view, err := app.PanelSettings(ctx)
	if err != nil || view.DetectedPublicIP != "1.1.1.1" || view.Preferences.PublicNodeHost != "" {
		t.Fatal(view, err)
	}
	got, err := app.publicationHost(ctx, store.SubscriptionNodeControls{}, "")
	if err != nil || got != "1.1.1.1" {
		t.Fatal(got, err)
	}
	view.Preferences.PublicNodeHost = "nodes.example.com"
	saved, err := app.SavePanelSettings(ctx, PanelSettingsWrite{Revision: view.Revision, Preferences: view.Preferences})
	if err != nil || saved.Settings.DetectedPublicIP != "1.1.1.1" {
		t.Fatal(saved, err)
	}
	got, err = app.publicationHost(ctx, store.SubscriptionNodeControls{}, "legacy.example.com")
	if err != nil || got != "nodes.example.com" {
		t.Fatal(got, err)
	}
	app.SetPublicIPResolver(func(context.Context) string { return "" })
	got, err = app.publicationHost(ctx, store.SubscriptionNodeControls{}, "")
	if err != nil || got != "nodes.example.com" {
		t.Fatal("configured host lost when detection failed", got, err)
	}
}
