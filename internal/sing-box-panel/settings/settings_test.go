// SPDX-License-Identifier: GPL-3.0-or-later

package settings

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/testutil"
)

func TestLoadRejectsRemovedCatalogTTLField(t *testing.T) {
	value := Defaults()
	value.DataDir = t.TempDir()
	value.Auth.Email = testutil.AdminEmail
	value.Auth.PasswordHash = testutil.PasswordHash
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
	if value.Auth.PasswordHash == "" {
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
	if loaded.Auth.PasswordHash != value.Auth.PasswordHash {
		t.Fatal("Load() did not preserve the token")
	}
	if loaded.Panel.Appearance.Theme != "system" {
		t.Fatalf("initialized theme = %q, want system", loaded.Panel.Appearance.Theme)
	}
	if len(loaded.Subscription.PrivateSourceCIDRs) != 0 {
		t.Fatal("initialized private source allowlist must be empty")
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
  "auth":{"token":"one","token":"two"},
  "github":{"token":"","catalog_refresh_interval_hours":12},
  "traffic":{},
  "subscription":{"private_source_cidrs":[]},
  "logs":{"core_retention_days":7}
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
  "auth":{"email":"admin@example.com","password_hash":"` + testutil.PasswordHash + `"},
  "github":{"token":"","catalog_refresh_interval_hours":12},
  "traffic":{},
  "subscription":{"private_source_cidrs":[]},
  "logs":{"core_retention_days":7}
}`
	if err := os.WriteFile(path, []byte(legacy), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil || !strings.Contains(err.Error(), "sample_retention_days") {
		t.Fatalf("Load() missing retention error = %v", err)
	}
	loaded := Defaults()
	loaded.DataDir = root
	loaded.Auth.Email = testutil.AdminEmail
	loaded.Auth.PasswordHash = testutil.PasswordHash
	loaded.Traffic.SampleRetentionDays = 367
	if err := loaded.Validate(); err == nil || !strings.Contains(err.Error(), "sample_retention_days") {
		t.Fatalf("Validate() oversized retention error = %v", err)
	}

	invalid := strings.Replace(legacy, `"traffic":{}`, `"traffic":{"sample_retention_days":0}`, 1)
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
			value.Auth.Email = testutil.AdminEmail
			value.Auth.PasswordHash = testutil.PasswordHash
			value.Server.BasePath = basePath
			if err := value.Validate(); err == nil {
				t.Fatalf("Validate() accepted unsafe base path %q", basePath)
			}
		})
	}
}

func TestNodeEndpointFollowsServerSettings(t *testing.T) {
	value := Defaults()
	value.Auth.Email, value.Auth.PasswordHash = testutil.AdminEmail, testutil.PasswordHash
	for _, check := range []struct {
		name            string
		mutate          func(*Settings)
		endpoint, trust string
	}{
		{"web listener", func(*Settings) {}, "127.0.0.1:3000", NodeTrustPin},
		{"https origin", func(value *Settings) { value.Server.ExternalOrigin = "https://www.example.com" }, "www.example.com:443", NodeTrustSystem},
		{"origin port", func(value *Settings) { value.Server.ExternalOrigin = "https://www.example.com:8443" }, "www.example.com:8443", NodeTrustSystem},
		{"http origin", func(value *Settings) { value.Server.ExternalOrigin = "http://www.example.com" }, "127.0.0.1:3000", NodeTrustPin},
		{"override", func(value *Settings) {
			value.Server.ExternalOrigin = "https://www.example.com"
			value.Nodes = Nodes{Endpoint: "203.0.113.7:3000"}
		}, "203.0.113.7:3000", NodeTrustPin},
	} {
		next := value
		check.mutate(&next)
		if err := next.Validate(); err != nil {
			t.Fatalf("%s: %v", check.name, err)
		}
		if next.NodeEndpoint() != check.endpoint || next.NodeTrust() != check.trust {
			t.Errorf("%s: endpoint %q trust %q, want %q %q", check.name, next.NodeEndpoint(), next.NodeTrust(), check.endpoint, check.trust)
		}
	}
	for name, nodes := range map[string]Nodes{
		"endpoint without port":         {Endpoint: "www.example.com"},
		"system trust without endpoint": {Trust: NodeTrustSystem},
		"unknown trust":                 {Trust: "none"},
	} {
		next := value
		next.Nodes = nodes
		if err := next.Validate(); err == nil || !strings.Contains(err.Error(), "nodes.") {
			t.Errorf("%s: Validate() = %v", name, err)
		}
	}
}

func TestExternalOriginNeedsNoCookieSetting(t *testing.T) {
	value := Defaults()
	value.DataDir = t.TempDir()
	value.Auth.Email = testutil.AdminEmail
	value.Auth.PasswordHash = testutil.PasswordHash

	for _, origin := range []string{"", "http://panel.example.com", "https://panel.example.com"} {
		value.Server.ExternalOrigin = origin
		if err := value.Validate(); err != nil {
			t.Fatalf("Validate() rejected origin %q: %v", origin, err)
		}
	}
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "secure_cookie") {
		t.Fatal("generated settings contain removed cookie field")
	}
	path := filepath.Join(t.TempDir(), "setting.json")
	for _, legacy := range []string{"true", "false", "null"} {
		input := strings.Replace(string(raw), `"auth":{`, `"auth":{"secure_cookie":`+legacy+`,`, 1)
		if err := os.WriteFile(path, []byte(input), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := Load(path); err == nil || !strings.Contains(err.Error(), `unknown field "secure_cookie"`) {
			t.Fatalf("removed cookie field accepted: %v", err)
		}
		after, err := os.ReadFile(path)
		if err != nil || string(after) != input {
			t.Fatal("failed load changed settings", err)
		}
	}
}

func TestCoreLogPolicyDefaultsAndBounds(t *testing.T) {
	value := Defaults()
	value.Auth.Email = testutil.AdminEmail
	value.Auth.PasswordHash = testutil.PasswordHash
	value.DataDir = t.TempDir()
	raw, _ := json.Marshal(value)
	var legacy map[string]any
	if err := json.Unmarshal(raw, &legacy); err != nil {
		t.Fatal(err)
	}
	if _, exists := legacy["logs"].(map[string]any)["retention_days"]; exists {
		t.Fatal("new settings should not specify panel event expiration")
	}
	legacy["logs"] = map[string]any{}
	raw, _ = json.Marshal(legacy)
	loaded, err := Parse(filepath.Join(t.TempDir(), "setting.json"), raw)
	if err != nil || loaded.Logs != value.Logs {
		t.Fatalf("panel event retention must not be required: %v", err)
	}
	legacy["logs"] = map[string]any{"retention_days": 30}
	raw, _ = json.Marshal(legacy)
	if _, err := Parse(filepath.Join(t.TempDir(), "setting.json"), raw); err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("removed retention field accepted: %v", err)
	}
	for _, policy := range []Logs{
		{CoreRetentionDays: 0, CoreMaxFileSizeMiB: 32},
		{CoreRetentionDays: 3651, CoreMaxFileSizeMiB: 32},
		{CoreRetentionDays: 7, CoreMaxFiles: -1, CoreMaxFileSizeMiB: 32},
		{CoreRetentionDays: 7, CoreMaxFiles: 1025, CoreMaxFileSizeMiB: 32},
		{CoreRetentionDays: 7, CoreMaxFileSizeMiB: 0},
		{CoreRetentionDays: 7, CoreMaxFileSizeMiB: 1025},
	} {
		value.Logs = policy
		raw, _ := json.Marshal(value)
		if _, err := Parse(filepath.Join(t.TempDir(), "setting.json"), raw); err == nil {
			t.Fatalf("accepted invalid policy: %+v", policy)
		}
	}
}
