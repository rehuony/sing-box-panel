// SPDX-License-Identifier: GPL-3.0-or-later
package application

import (
	"context"
	"encoding/base64"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rehuony/sing-box-panel/internal/configuration"
	"github.com/rehuony/sing-box-panel/internal/settings"
	"github.com/rehuony/sing-box-panel/internal/store"
)

func TestProtocolIdentityPreservesOtherCredentialsAndOptions(t *testing.T) {
	input := `{"future":9007199254740993,"inbounds":[
    {"type":"mixed","tag":"open","listen":"127.0.0.1","listen_port":1080},
    {"type":"vless","tag":"v","users":[{"name":"legacy","uuid":"existing"}],"tls":{"enabled":true,"reality":{"private_key":"private","short_id":["aa"]}}},
    {"type":"hysteria2","tag":"h","users":[{"name":"panel","password":"old"}],"obfs":{"type":"salamander","password":"separate"}},
    {"type":"shadowsocks","tag":"ss","method":"2022-blake3-aes-128-gcm","password":"server-key","users":[{"name":"legacy","password":"user-key"}]}
  ],"outbounds":[{"type":"trojan","password":"external"}]}`
	raw, changed, err := applyProtocolIdentity(input, "panel", "identity-secret")
	if err != nil || !changed || !strings.Contains(raw, "9007199254740993") {
		t.Fatal(changed, err)
	}
	doc, _ := configuration.Parse([]byte(raw))
	root := doc.Configuration()
	inbounds := root["inbounds"].([]any)
	if inbounds[0].(map[string]any)["users"] != nil {
		t.Fatal("enabled authentication on unauthenticated listener")
	}
	v := inbounds[1].(map[string]any)["users"].([]any)
	if len(v) != 2 || v[0].(map[string]any)["uuid"] != "existing" {
		t.Fatal("replaced an existing user")
	}
	if len(v[1].(map[string]any)["uuid"].(string)) != 36 {
		t.Fatal("missing native UUID")
	}
	h := inbounds[2].(map[string]any)
	if h["users"].([]any)[0].(map[string]any)["password"] != "identity-secret" || h["obfs"].(map[string]any)["password"] != "separate" {
		t.Fatal("identity mixed with obfuscation")
	}
	ss := inbounds[3].(map[string]any)
	decoded, err := base64.StdEncoding.DecodeString(ss["users"].([]any)[1].(map[string]any)["password"].(string))
	if err != nil || len(decoded) != 16 || ss["password"] != "server-key" {
		t.Fatal("invalid SS2022 key projection")
	}
	if root["outbounds"].([]any)[0].(map[string]any)["password"] != "external" {
		t.Fatal("rewrote external credential")
	}
	same, changed, err := applyProtocolIdentity(raw, "panel", "identity-secret")
	if err != nil || changed || same != raw {
		t.Fatal("identity update is not idempotent")
	}
}

func TestNewInboundUsesSavedIdentityWithoutSavingConfiguration(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(ctx, filepath.Join(t.TempDir(), "panel.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	app := FromStoreWithSettings(db, settingsFileFixture(t, settings.Defaults()))
	view, _ := app.PanelSettings(ctx)
	view.Preferences.IdentityName = "shared"
	if _, err := app.SavePanelSettings(ctx, PanelSettingsWrite{Revision: view.Revision, Preferences: view.Preferences, IdentityKey: "new-shared-key"}); err != nil {
		t.Fatal(err)
	}
	before, _ := app.ConfigurationFile(ctx)
	for _, protocol := range []string{"vless", "tuic", "hysteria2", "anytls", "shadowsocks", "mixed"} {
		entry, err := app.NewInboundDefaults(ctx, protocol)
		if err != nil {
			t.Fatal(err)
		}
		switch protocol {
		case "mixed":
			if entry["users"] != nil {
				t.Fatal("enabled optional authentication")
			}
		case "shadowsocks":
			key, err := base64.StdEncoding.DecodeString(entry["password"].(string))
			if err != nil || len(key) != 16 {
				t.Fatal("invalid cipher identity")
			}
		default:
			users := entry["users"].([]any)
			if len(users) != 1 || users[0].(map[string]any)["name"] != "shared" {
				t.Fatal("missing saved identity")
			}
		}
	}
	after, _ := app.ConfigurationFile(ctx)
	if before.Content != after.Content || before.Revision != after.Revision {
		t.Fatal("editor preparation saved configuration")
	}
}

func TestIdentityAndSettingsSaveAtomicallyWithoutLaunching(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(ctx, filepath.Join(t.TempDir(), "panel.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	app := FromStoreWithSettings(db, settingsFileFixture(t, settings.Defaults()))
	file, err := app.SaveConfigurationFile(ctx, ConfigurationFileWrite{Content: `{"inbounds":[{"type":"anytls","tag":"a","users":[]}]}`})
	if err != nil {
		t.Fatal(err)
	}
	view, _ := app.PanelSettings(ctx)
	view.Preferences.IdentityName = "panel"
	saved, err := app.SavePanelSettings(ctx, PanelSettingsWrite{Revision: view.Revision, Preferences: view.Preferences, IdentityKey: "new-secret"})
	if err != nil {
		t.Fatal(err)
	}
	updated, _ := app.ConfigurationFile(ctx)
	if updated.Revision != file.Revision+1 || !strings.Contains(updated.Content, `"password": "new-secret"`) {
		t.Fatal("settings did not update saved configuration")
	}
	bootstrap, _ := db.Bootstrap(ctx)
	if bootstrap.Hub.DesiredRunning || bootstrap.Hub.AppliedBundleID != "" {
		t.Fatal("identity save launched runtime")
	}
	_, err = app.SaveConfigurationFile(ctx, ConfigurationFileWrite{Revision: updated.Revision, Content: "{"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = app.SavePanelSettings(ctx, PanelSettingsWrite{Revision: saved.Revision, Preferences: saved.Preferences, IdentityKey: "rejected-secret"})
	if !errors.Is(err, ErrIdentityConfiguration) {
		t.Fatal(err)
	}
	after, _, _ := app.storedPanelSettings(ctx)
	if after.IdentityKey != "new-secret" {
		t.Fatal("identity changed without updating invalid config")
	}
	raw, _ := app.ConfigurationFile(ctx)
	if raw.Content != "{" {
		t.Fatal("invalid file replaced with older valid config")
	}
	// A configuration edit racing the prepared identity save rolls back settings.
	_, err = app.SaveConfigurationFile(ctx, ConfigurationFileWrite{Revision: raw.Revision, Content: "{}"})
	if err != nil {
		t.Fatal(err)
	}
	change, _ := app.configurationFileUpdate(ConfigurationFileWrite{Revision: updated.Revision, Content: `{"inbounds":[]}`})
	err = db.CommitPanelSettingsFile(ctx, app.settingsPath, "rejected", nil, change, func() error { t.Fatal("stale identity update published settings"); return nil })
	if !errors.Is(err, store.ErrConfigurationFileConflict) {
		t.Fatal(err)
	}
	after, _, _ = app.storedPanelSettings(ctx)
	if after.IdentityKey != "new-secret" {
		t.Fatal("partial settings transaction")
	}
}
