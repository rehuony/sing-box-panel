// SPDX-License-Identifier: GPL-3.0-or-later

package settings

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

	"github.com/rehuony/sing-box-panel/internal/auth"
	"github.com/rehuony/sing-box-panel/internal/testutil"
)

func settingsFieldFixture(t *testing.T) (string, Settings) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "setting.json")
	value := Defaults()
	value.DataDir = "./private-data"
	value.Auth.Email = testutil.AdminEmail
	value.Auth.PasswordHash = testutil.PasswordHash
	value.Server.Port = 8181
	value.GitHub.CatalogRefreshIntervalHours = 24
	value.GitHub.Token = "clear-optional-token"
	value.Logs.CoreRetentionDays = 30
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if err := Replace(path, raw); err != nil {
		t.Fatal(err)
	}
	return path, value
}

func TestSetFieldPreservesOtherValuesAndOmittedDefaults(t *testing.T) {
	path, original := settingsFieldFixture(t)
	before, _ := Read(path)
	var document map[string]json.RawMessage
	if err := json.Unmarshal(before, &document); err != nil {
		t.Fatal(err)
	}
	delete(document, "panel")
	delete(document, "logs")
	before, _ = json.Marshal(document)
	if err := Replace(path, before); err != nil {
		t.Fatal(err)
	}
	if err := SetField(t.Context(), path, "server.port", "8443"); err != nil {
		t.Fatal(err)
	}
	after, err := Read(path)
	if err != nil {
		t.Fatal(err)
	}
	var documentBefore, documentAfter map[string]any
	if err := json.Unmarshal(before, &documentBefore); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(after, &documentAfter); err != nil {
		t.Fatal(err)
	}
	documentBefore["server"].(map[string]any)["port"] = float64(8443)
	if !reflect.DeepEqual(documentBefore, documentAfter) {
		t.Fatal("unselected fields or omitted defaults changed")
	}
	loaded, err := Load(path)
	if err != nil || loaded.Server.Port != 8443 || loaded.Auth.PasswordHash != original.Auth.PasswordHash {
		t.Fatal("field not saved correctly", err)
	}
	if info, err := os.Stat(path); err != nil || info.Mode().Perm() != 0600 {
		t.Fatal("private permissions lost", err)
	}
	if _, err := os.Stat(loaded.DataDir); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("field update accessed storage", err)
	}
}

func TestSetFieldSupportsTypedValuesAndMissingSections(t *testing.T) {
	path, _ := settingsFieldFixture(t)
	for _, update := range []struct{ field, value string }{
		{"github.token", "12345"},
		{"panel.appearance", `{"theme":"dark","color":"#123456","radius":8}`},
		{"subscription.private_source_cidrs", `["10.0.0.0/8","fd00::/8"]`},
		{"traffic.quota_gib", "42"},
	} {
		if err := SetField(t.Context(), path, update.field, update.value); err != nil {
			t.Fatalf("set %s: %v", update.field, err)
		}
	}
	loaded, err := Load(path)
	if err != nil || loaded.GitHub.Token != "12345" || loaded.Panel.Appearance.Theme != "dark" || loaded.Panel.Appearance.Radius != 8 || len(loaded.Subscription.PrivateSourceCIDRs) != 2 || loaded.Traffic.QuotaGiB == nil || *loaded.Traffic.QuotaGiB != 42 {
		t.Fatal("typed values were not preserved", err)
	}
	if err := SetField(t.Context(), path, "traffic.quota_gib", "null"); err != nil {
		t.Fatal(err)
	}
	before, _ := Read(path)
	var document map[string]json.RawMessage
	if err := json.Unmarshal(before, &document); err != nil {
		t.Fatal(err)
	}
	delete(document, "panel")
	before, _ = json.Marshal(document)
	if err := Replace(path, before); err != nil {
		t.Fatal(err)
	}
	if err := SetField(t.Context(), path, "panel.appearance.theme", "light"); err != nil {
		t.Fatal(err)
	}
	after, _ := Read(path)
	if err := json.Unmarshal(after, &document); err != nil {
		t.Fatal(err)
	}
	var panel map[string]any
	if err := json.Unmarshal(document["panel"], &panel); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(panel, map[string]any{"appearance": map[string]any{"theme": "light"}}) {
		t.Fatal("missing section materialized unrelated defaults")
	}
	loaded, err = Load(path)
	if err != nil || loaded.Traffic.QuotaGiB != nil || loaded.Panel.Appearance.Theme != "light" {
		t.Fatal("nullable value or missing section update failed", err)
	}
}

func TestSetFieldClearsOptionalValuesWithoutRestoringDefaults(t *testing.T) {
	path, _ := settingsFieldFixture(t)
	for _, update := range []struct{ field, value, want string }{
		{"github.token", "", `""`},
		{"server.external_origin", "", `""`},
		{"traffic.quota_gib", "null", "null"},
		{"subscription.private_source_cidrs", "[]", "[]"},
		{"subscription.private_source_cidrs", "null", "null"},
	} {
		if err := SetField(t.Context(), path, update.field, update.value); err != nil {
			t.Fatalf("clear %s: %v", update.field, err)
		}
		value, err := GetField(t.Context(), path, update.field)
		if err != nil || string(value) != update.want {
			t.Fatalf("clear %s saved %s, want %s: %v", update.field, value, update.want, err)
		}
	}
}

func TestSetFieldRepairsInvalidValueButRequiresExistingJSON(t *testing.T) {
	path, value := settingsFieldFixture(t)
	value.Server.Port = 0
	invalid, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, invalid, 0600); err != nil {
		t.Fatal(err)
	}
	if err := SetField(t.Context(), path, "server.port", "9090"); err != nil {
		t.Fatal("could not repair an invalid settings value", err)
	}
	loaded, err := Load(path)
	if err != nil || loaded.Server.Port != 9090 {
		t.Fatal("repaired settings are invalid", err)
	}
	for _, malformed := range []string{"{", "null", "[]"} {
		if err := os.WriteFile(path, []byte(malformed), 0600); err != nil {
			t.Fatal(err)
		}
		if err := SetField(t.Context(), path, "server.port", "9090"); err == nil {
			t.Fatal("field update accepted a malformed settings document")
		}
		after, err := os.ReadFile(path)
		if err != nil || string(after) != malformed {
			t.Fatal("field update replaced malformed settings", err)
		}
	}
	missing := filepath.Join(t.TempDir(), "setting.json")
	if err := SetField(t.Context(), missing, "server.port", "9090"); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("field update accepted a missing settings file", err)
	}
	if _, err := os.Stat(missing); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("field update initialized settings", err)
	}
}

func TestSetFieldRejectsInvalidChangesWithoutEchoingValues(t *testing.T) {
	path, _ := settingsFieldFixture(t)
	before, _ := Read(path)
	for _, update := range []struct{ field, value string }{
		{"", "private-value"},
		{"/", "private-value"},
		{"/server/port", "8443"},
		{"server/port", "8443"},
		{"/auth/password", testutil.ChangedPassword},
		{"server..port", "private-value"},
		{"server.Port", "private-value"},
		{"unknown", "private-value"},
		{"InitialPassword", "private-value"},
		{"subscription.private_source_cidrs.0", "private-value"},
		{"server.port", `"private-value"`},
		{"server.port", ""},
		{"server.port", "null"},
		{"server.port", "8443 8444"},
		{"server.port", "65536"},
		{"server", "null"},
		{"server", `{"host":"127.0.0.1","port":8443,"private-value":1}`},
		{"server", `{"host":"127.0.0.1","port":8443,"port":8444}`},
		{"server.external_origin", "https://private-value/%"},
		{"subscription.private_source_cidrs", `["private-value"]`},
		{"traffic.quota_gib", `"private-value"`},
		{"auth.password_hash", "private-value"},
		{"auth.password", "too-short"},
		{"auth.password", ""},
		{"auth", `{"email":"admin@example.com","password":"private-value"}`},
	} {
		t.Run(update.field+"="+update.value, func(t *testing.T) {
			err := SetField(t.Context(), path, update.field, update.value)
			if err == nil {
				t.Fatal("invalid field change accepted")
			}
			if strings.Contains(err.Error(), "private-value") || strings.Contains(err.Error(), "too-short") {
				t.Fatalf("error disclosed supplied value: %v", err)
			}
			after, err := Read(path)
			if err != nil || !bytes.Equal(before, after) {
				t.Fatal("invalid change modified the settings file", err)
			}
		})
	}
}

func TestSetPasswordHashesPlaintextAndAcceptsExistingHash(t *testing.T) {
	path, _ := settingsFieldFixture(t)
	if err := SetField(t.Context(), path, "auth.password", testutil.ChangedPassword); err != nil {
		t.Fatal(err)
	}
	raw, _ := Read(path)
	if bytes.Contains(raw, []byte(testutil.ChangedPassword)) || bytes.Contains(raw, []byte(`"password"`)) {
		t.Fatal("plaintext password persisted")
	}
	loaded, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	matches, err := auth.VerifyPassword(t.Context(), testutil.ChangedPassword, loaded.Auth.PasswordHash)
	if err != nil || !matches {
		t.Fatal("password cannot authenticate", err)
	}
	if err := SetField(t.Context(), path, "auth.password_hash", testutil.PasswordHash); err != nil {
		t.Fatal(err)
	}
	loaded, err = Load(path)
	if err != nil || loaded.Auth.PasswordHash != testutil.PasswordHash {
		t.Fatal("pre-hashed password was changed", err)
	}
}

func TestSetFieldPreservesRecoveryAndCancellation(t *testing.T) {
	path, _ := settingsFieldFixture(t)
	before, _ := Read(path)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	for _, field := range []string{"server.port", "auth.password"} {
		value := "8443"
		if field == "auth.password" {
			value = testutil.ChangedPassword
		}
		if err := SetField(ctx, path, field, value); !errors.Is(err, context.Canceled) {
			t.Fatal("field update ignored cancellation", err)
		}
	}
	if err := os.WriteFile(path+".pending", []byte("pending"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := SetField(t.Context(), path, "server.port", "8443"); !errors.Is(err, ErrPending) {
		t.Fatal("field update bypassed recovery", err)
	}
	after, _ := ReadRaw(path)
	if !bytes.Equal(before, after) {
		t.Fatal("blocked field update changed settings")
	}
}

func TestSetDataDirRetainsPreviousLocationForMigration(t *testing.T) {
	path, _ := settingsFieldFixture(t)
	previous, err := ConfiguredDataDir(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := SetField(t.Context(), path, "data_dir", "./next-data"); err != nil {
		t.Fatal(err)
	}
	configured, err := ConfiguredDataDir(path)
	if err != nil || configured == previous {
		t.Fatal("data directory not changed", err)
	}
	active, err := LoadDataDir(path)
	if err != nil || active != previous {
		t.Fatal("previous location lost before migration", err)
	}
}

func TestConcurrentFieldUpdatesPreserveEveryEdit(t *testing.T) {
	path, _ := settingsFieldFixture(t)
	var workers sync.WaitGroup
	for _, update := range []struct{ field, value string }{
		{"server.port", "8443"},
		{"github.token", "new-token"},
		{"logs.core_retention_days", "60"},
		{"github.catalog_refresh_interval_hours", "12"},
		{"auth.password", testutil.ChangedPassword},
	} {
		workers.Go(func() {
			if err := SetField(t.Context(), path, update.field, update.value); err != nil {
				t.Error(err)
			}
		})
	}
	workers.Wait()
	loaded, err := Load(path)
	if err != nil || loaded.Server.Port != 8443 || loaded.GitHub.Token != "new-token" || loaded.GitHub.CatalogRefreshIntervalHours != 12 || loaded.Logs.CoreRetentionDays != 60 || loaded.Auth.PasswordHash == testutil.PasswordHash {
		t.Fatal("concurrent settings write lost a field", err)
	}
}
