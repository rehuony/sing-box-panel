// SPDX-License-Identifier: GPL-3.0-or-later

package settings

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/rehuony/sing-box-panel/internal/webauth"
)

func TestFirstStartCreatesPrivateSettingsWithAConsolePassword(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sing-box-node", "setting.json")
	created, isNew, err := LoadOrInitialize(t.Context(), path)
	if err != nil || !isNew || created.InitialPassword == "" {
		t.Fatalf("created = %+v, %v, %v", created, isNew, err)
	}
	if info, err := os.Stat(path); err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("settings file mode: %v, %v", info, err)
	}
	loaded, isNew, err := LoadOrInitialize(t.Context(), path)
	if err != nil || isNew || loaded.InitialPassword != "" || loaded.Listen() != "127.0.0.1:8000" {
		t.Fatalf("loaded = %+v, %v, %v", loaded, isNew, err)
	}
	if ok, err := webauth.VerifyPassword(t.Context(), created.InitialPassword, loaded.Auth.PasswordHash); err != nil || !ok {
		t.Fatalf("the printed password does not match the saved hash: %v", err)
	}
}

func TestSetFieldHashesThePasswordAndRefusesInvalidValues(t *testing.T) {
	path := filepath.Join(t.TempDir(), "setting.json")
	if _, err := Initialize(t.Context(), path, false); err != nil {
		t.Fatal(err)
	}
	if err := SetField(t.Context(), path, "auth.password", "a new console password"); err != nil {
		t.Fatal(err)
	}
	if err := SetField(t.Context(), path, "server.host", "0.0.0.0"); err != nil {
		t.Fatal(err)
	}
	value, err := Load(path)
	if err != nil || value.Server.Host != "0.0.0.0" || value.ConsoleURL() != "http://127.0.0.1:8000/" {
		t.Fatalf("value = %+v, %v", value, err)
	}
	if ok, err := webauth.VerifyPassword(t.Context(), "a new console password", value.Auth.PasswordHash); err != nil || !ok {
		t.Fatalf("auth.password did not save its hash: %v", err)
	}
	if _, err := GetField(t.Context(), path, "auth.password"); err == nil {
		t.Fatal("auth.password is readable")
	}
	for field, raw := range map[string]string{"server.port": "0", "server.host": "node.example", "server.external_origin": "https://node.example/path"} {
		if err := SetField(t.Context(), path, field, raw); err == nil {
			t.Errorf("%s accepted %q", field, raw)
		}
	}
}
