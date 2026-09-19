// SPDX-License-Identifier: GPL-3.0-or-later

package settings

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func resetFixture(t *testing.T) (string, Settings) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "setting.json")
	value := Defaults()
	value.DataDir = "./private-data"
	value.Auth.Token = "keep-required-token"
	value.Server.Port = 8181
	value.Subscription.Provider = "custom-provider"
	value.GitHub.Token = "clear-optional-token"
	value.Logs.RetentionDays = 30
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if err := Replace(path, raw); err != nil {
		t.Fatal(err)
	}
	return path, value
}

func TestResetFieldsKeepsUnselectedValuesAndStorageUntouched(t *testing.T) {
	path, original := resetFixture(t)
	if err := ResetFields(t.Context(), path, []string{"server.port", "/subscription/provider", "github.token"}); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Server.Port != Defaults().Server.Port || loaded.Subscription.Provider != "default" || loaded.GitHub.Token != "" || loaded.Auth.Token != original.Auth.Token || loaded.Logs != original.Logs {
		t.Fatal("reset changed unselected fields or did not restore defaults")
	}
	raw, _ := Read(path)
	var file Settings
	if err := json.Unmarshal(raw, &file); err != nil || file.DataDir != original.DataDir {
		t.Fatal("relative data directory was rewritten", err)
	}
	if _, err := os.Stat(loaded.DataDir); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("reset accessed the data directory", err)
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatal("reset did not preserve private permissions", err)
	}
}

func TestResetFieldsValidatesWholeEditBeforeSaving(t *testing.T) {
	path, value := resetFixture(t)
	value.Server.ExternalOrigin = "https://panel.example.com"
	value.Auth.SecureCookie = true
	raw, _ := json.Marshal(value)
	if err := Replace(path, raw); err != nil {
		t.Fatal(err)
	}
	for _, fields := range [][]string{{}, {"server.port", "unknown"}, {""}, {"/"}, {"server..port"}, {"auth.token"}, {"auth"}, {"server.external_origin"}, {"subscription.private_source_cidrs.0"}} {
		if err := ResetFields(t.Context(), path, fields); err == nil {
			t.Fatalf("invalid reset accepted: %v", fields)
		}
		after, err := Read(path)
		if err != nil || !bytes.Equal(after, raw) {
			t.Fatalf("invalid reset changed the file: %v", fields)
		}
	}
	if err := ResetFields(t.Context(), path, []string{"server.external_origin", "auth.secure_cookie"}); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(path)
	if err != nil || loaded.Server.ExternalOrigin != "" || loaded.Auth.SecureCookie {
		t.Fatal("related fields could not reset together", err)
	}
}

func TestResetDataDirRetainsPreviousLocationForMigration(t *testing.T) {
	path, _ := resetFixture(t)
	previous, err := ConfiguredDataDir(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := ResetFields(t.Context(), path, []string{"data_dir"}); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(path)
	if err != nil || loaded.DataDir != Defaults().DataDir {
		t.Fatal("default data directory not selected", err)
	}
	active, err := LoadDataDir(path)
	if err != nil || active != previous {
		t.Fatal("original location lost before migration", err)
	}
}

func TestResetFieldsRestoresSectionsArraysAndNullableDefaults(t *testing.T) {
	path, value := resetFixture(t)
	value.Panel.Appearance.Theme = "dark"
	value.Panel.Appearance.Radius = 0
	value.Panel.Language = "en"
	value.Subscription.PrivateSourceCIDRs = []string{"10.0.0.0/8"}
	quota := int64(8589934591)
	value.Traffic.QuotaGiB = &quota
	raw, _ := json.Marshal(value)
	if err := Replace(path, raw); err != nil {
		t.Fatal(err)
	}
	if err := ResetFields(t.Context(), path, []string{"/panel/appearance", "subscription.private_source_cidrs", "traffic.quota_gib"}); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(path)
	if err != nil || loaded.Panel.Appearance != DefaultPanel().Appearance || loaded.Panel.Language != "en" || len(loaded.Subscription.PrivateSourceCIDRs) != 0 || loaded.Traffic.QuotaGiB != nil {
		t.Fatal("section, array or nullable default was not restored", err)
	}
}

func TestResetFieldsPreservesRecoveryAndCancellation(t *testing.T) {
	path, _ := resetFixture(t)
	before, _ := Read(path)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := ResetFields(ctx, path, []string{"server.port"}); !errors.Is(err, context.Canceled) {
		t.Fatal("reset ignored cancellation", err)
	}
	if err := os.WriteFile(path+".pending", []byte("pending"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := ResetFields(t.Context(), path, []string{"server.port"}); !errors.Is(err, ErrPending) {
		t.Fatal("reset bypassed recovery", err)
	}
	after, _ := ReadRaw(path)
	if !bytes.Equal(before, after) {
		t.Fatal("blocked reset changed settings")
	}
}

func TestConcurrentFieldResetsPreserveEveryEdit(t *testing.T) {
	path, _ := resetFixture(t)
	var group sync.WaitGroup
	for _, field := range []string{"server.port", "subscription.provider", "github.token", "logs.retention_days"} {
		group.Go(func() {
			if err := ResetFields(t.Context(), path, []string{field}); err != nil {
				t.Error(err)
			}
		})
	}
	group.Wait()
	loaded, err := Load(path)
	if err != nil || loaded.Server.Port != 3000 || loaded.Subscription.Provider != "default" || loaded.GitHub.Token != "" || loaded.Logs.RetentionDays != 7 {
		t.Fatal("concurrent reset lost a field", err)
	}
}
