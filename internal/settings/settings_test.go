// SPDX-License-Identifier: GPL-3.0-or-later

package settings

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInitializeAndLoad(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "config", "setting.json")
	value, err := Initialize(path, false)
	if err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	if value.Auth.Token == "" {
		t.Fatal("Initialize() generated an empty token")
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("settings mode = %o, want 600", info.Mode().Perm())
	}
	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if loaded.Auth.Token != value.Auth.Token {
		t.Fatal("Load() did not preserve the token")
	}
	if _, err := Initialize(path, false); err == nil {
		t.Fatal("Initialize() unexpectedly overwrote settings")
	}
}

func TestLoadRejectsAmbiguousSettings(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "setting.json")
	input := `{
  "server":{"host":"127.0.0.1","port":3000,"base_path":""},
  "data_dir":"data",
  "auth":{"token":"one","token":"two","secure_cookie":false},
  "github":{"token":"","catalog_ttl_hours":12},
  "traffic":{"quota_gib":null,"period_months":1},
  "subscription":{"author":"a","provider":"p","private_source_cidrs":[]},
  "logs":{"retention_days":7}
}`
	if err := os.WriteFile(path, []byte(input), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := Load(path)
	if err == nil || !strings.Contains(err.Error(), "duplicate object key") {
		t.Fatalf("Load() error = %v", err)
	}
}

func TestValidateRejectsUnsafeBasePath(t *testing.T) {
	for _, basePath := range []string{"relative", "/trailing/", "/double//slash", "/../escape", "/panel?<script>"} {
		t.Run(basePath, func(t *testing.T) {
			value := Defaults(filepath.Join(t.TempDir(), "setting.json"))
			value.DataDir = t.TempDir()
			value.Auth.Token = "token"
			value.Server.BasePath = basePath
			if err := value.Validate(); err == nil {
				t.Fatalf("Validate() accepted unsafe base path %q", basePath)
			}
		})
	}
}
