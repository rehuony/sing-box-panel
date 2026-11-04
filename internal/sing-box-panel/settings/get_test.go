// SPDX-License-Identifier: GPL-3.0-or-later

package settings

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGetFieldReturnsStoredValuesWithoutResolvingOrValidatingSettings(t *testing.T) {
	path := filepath.Join(t.TempDir(), "setting.json")
	// This incomplete document is readable even though it cannot start a panel.
	document := `{"server":{"port":0,"host":" localhost "},"data_dir":"./private-data","auth":{"password_hash":"stored-hash"},"traffic":{"quota_gib":null},"subscription":{"private_source_cidrs":["10.0.0.0/8"]}}`
	if err := os.WriteFile(path, []byte(document), 0600); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct{ field, want string }{
		{"server.port", "0"},
		{"server.host", `" localhost "`},
		{"server", `{"port":0,"host":" localhost "}`},
		{"data_dir", `"./private-data"`},
		{"auth.password_hash", `"stored-hash"`},
		{"traffic.quota_gib", "null"},
		{"subscription.private_source_cidrs", `["10.0.0.0/8"]`},
	} {
		t.Run(test.field, func(t *testing.T) {
			got, err := GetField(t.Context(), path, test.field)
			if err != nil || string(got) != test.want {
				t.Fatalf("got %s, want %s: %v", got, test.want, err)
			}
		})
	}
	for _, field := range []string{"panel", "panel.appearance.theme", "server.base_path"} {
		if _, err := GetField(t.Context(), path, field); err == nil || !strings.Contains(err.Error(), "not present") {
			t.Fatalf("missing field %q materialized a default: %v", field, err)
		}
	}
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil || len(entries) != 1 || entries[0].Name() != "setting.json" {
		t.Fatal("reading settings created lock, metadata or storage files", err)
	}
	after, err := os.ReadFile(path)
	if err != nil || string(after) != document {
		t.Fatal("reading settings changed its contents", err)
	}
}

func TestGetFieldRejectsInvalidPathsAndWriteOnlyPassword(t *testing.T) {
	path, _ := settingsFieldFixture(t)
	for _, field := range []string{"", "/", "/server/port", "server/port", "/auth/password", "server..port", "server.Port", "server.port.value", "unknown", "InitialPassword", "subscription.private_source_cidrs.0"} {
		t.Run(field, func(t *testing.T) {
			if _, err := GetField(t.Context(), path, field); err == nil {
				t.Fatal("invalid field path accepted")
			}
		})
	}
	if _, err := GetField(t.Context(), path, "auth.password"); err == nil || !strings.Contains(err.Error(), "write-only") {
		t.Fatal("plaintext password was readable", err)
	}
}

func TestGetFieldRejectsAmbiguousOrNonObjectJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "setting.json")
	for _, document := range []string{
		"{", "null", "[]", `{"server":{"port":3000}} {}`,
		`{"server":{"port":3000,"port":4000}}`,
		`{"server":{"port":3000},"other":{"private-value":1,"private-value":2}}`,
		`{"server":null}`, `{"server":[]}`, `{"server":3000}`,
		`{"server":{},"port":3000}`,
	} {
		t.Run(document, func(t *testing.T) {
			if err := os.WriteFile(path, []byte(document), 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := GetField(t.Context(), path, "server.port"); err == nil || strings.Contains(err.Error(), "private-value") {
				t.Fatal("invalid document accepted or leaked contents", err)
			}
		})
	}
}

func TestGetFieldHonorsRecoveryCancellationAndMissingFiles(t *testing.T) {
	path := filepath.Join(t.TempDir(), "setting.json")
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := GetField(ctx, path, "server.port"); !errors.Is(err, context.Canceled) {
		t.Fatal("read ignored cancellation", err)
	}
	if _, err := GetField(t.Context(), path, "server.port"); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("read accepted missing settings", err)
	}
	if err := os.WriteFile(path+".pending", []byte("pending"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := GetField(t.Context(), path, "server.port"); !errors.Is(err, ErrPending) {
		t.Fatal("read bypassed recovery", err)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("read initialized missing settings", err)
	}
}
