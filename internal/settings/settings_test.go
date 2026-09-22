// SPDX-License-Identifier: GPL-3.0-or-later

package settings

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadRejectsRemovedCatalogTTLField(t *testing.T) {
	value := Defaults()
	value.DataDir = t.TempDir()
	value.Auth.Token = "test-token"
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	legacy := strings.Replace(
		string(encoded),
		"catalog_refresh_interval_hours",
		"catalog_ttl_hours",
		1,
	)
	path := filepath.Join(t.TempDir(), "setting.json")
	if err := os.WriteFile(path, []byte(legacy), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("Load() removed catalog TTL field error = %v", err)
	}
}

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
	if loaded.Subscription.Provider != "default" {
		t.Fatalf("initialized provider = %q, want default", loaded.Subscription.Provider)
	}
	if loaded.Traffic.SampleRetentionDays != 90 {
		t.Fatalf("initialized sample retention = %d, want 90", loaded.Traffic.SampleRetentionDays)
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
  "github":{"token":"","catalog_refresh_interval_hours":12},
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

func TestLoadRejectsMissingOrInvalidTrafficSampleRetention(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "setting.json")
	legacy := `{
  "server":{"host":"127.0.0.1","port":3000,"base_path":""},
  "data_dir":"data",
  "auth":{"token":"token","secure_cookie":false},
  "github":{"token":"","catalog_refresh_interval_hours":12},
  "traffic":{"quota_gib":null,"period_months":1},
  "subscription":{"author":"a","provider":"p","private_source_cidrs":[]},
  "logs":{"retention_days":7}
}`
	if err := os.WriteFile(path, []byte(legacy), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil || !strings.Contains(err.Error(), "sample_retention_days") {
		t.Fatalf("Load() missing retention error = %v", err)
	}
	loaded := Defaults()
	loaded.DataDir = root
	loaded.Auth.Token = "token"
	loaded.Traffic.SampleRetentionDays = 367
	if err := loaded.Validate(); err == nil || !strings.Contains(err.Error(), "sample_retention_days") {
		t.Fatalf("Validate() oversized retention error = %v", err)
	}

	invalid := strings.Replace(legacy, `"period_months":1`, `"period_months":1,"sample_retention_days":0`, 1)
	if err := os.WriteFile(path, []byte(invalid), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil || !strings.Contains(err.Error(), "sample_retention_days") {
		t.Fatalf("Load() explicit invalid retention error = %v", err)
	}
}

func TestValidateRejectsUnsafeBasePath(t *testing.T) {
	for _, basePath := range []string{"relative", "/trailing/", "/double//slash", "/../escape", "/panel?<script>"} {
		t.Run(basePath, func(t *testing.T) {
			value := Defaults()
			value.DataDir = t.TempDir()
			value.Auth.Token = "token"
			value.Server.BasePath = basePath
			if err := value.Validate(); err == nil {
				t.Fatalf("Validate() accepted unsafe base path %q", basePath)
			}
		})
	}
}

func TestNormalizeOrigin(t *testing.T) {
	tests := map[string]string{
		"HTTPS://Example.COM:443": "https://example.com",
		"http://EXAMPLE.com:80":   "http://example.com",
		"https://[2001:db8::1]":   "https://[2001:db8::1]",
		"http://localhost:3000":   "http://localhost:3000",
	}
	for input, want := range tests {
		t.Run(input, func(t *testing.T) {
			got, err := NormalizeOrigin(input)
			if err != nil || got != want {
				t.Fatalf("NormalizeOrigin(%q) = %q, %v; want %q", input, got, err, want)
			}
		})
	}
	for _, input := range []string{"", "ftp://example.com", "https://user@example.com", "https://example.com/path", "https://example.com?x=1", "null"} {
		t.Run("reject_"+input, func(t *testing.T) {
			if _, err := NormalizeOrigin(input); err == nil {
				t.Fatalf("NormalizeOrigin(%q) unexpectedly succeeded", input)
			}
		})
	}
}

func TestValidateExternalOriginAndSecureCookie(t *testing.T) {
	value := Defaults()
	value.DataDir = t.TempDir()
	value.Auth.Token = "token"

	value.Server.ExternalOrigin = "https://panel.example.com"
	if err := value.Validate(); err == nil {
		t.Fatal("Validate() accepted HTTPS external origin without secure cookies")
	}
	value.Auth.SecureCookie = true
	if err := value.Validate(); err != nil {
		t.Fatalf("Validate() rejected matching HTTPS external origin: %v", err)
	}
	value.Server.ExternalOrigin = ""
	if err := value.Validate(); err == nil {
		t.Fatal("Validate() accepted secure cookies without an external origin")
	}
}
