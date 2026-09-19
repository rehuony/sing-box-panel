// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rehuony/sing-box-panel/internal/singbox"
)

func TestReviewedCatalogIsValidAndGenerated(t *testing.T) {
	t.Parallel()
	root, err := repositoryRoot()
	if err != nil {
		t.Fatal(err)
	}
	if err := check(root); err != nil {
		t.Fatal(err)
	}
}

func TestCatalogValidationRejectsInvalidMetadata(t *testing.T) {
	t.Parallel()
	root, err := repositoryRoot()
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := loadCatalog(filepath.Join(root, catalogPath))
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name   string
		mutate func(*sourceCatalog)
		match  string
	}{
		{name: "duplicate version", match: "strictly ascending", mutate: func(value *sourceCatalog) {
			value.Versions = append(value.Versions, value.Versions[len(value.Versions)-1])
		}},
		{name: "missing architecture", match: "exactly amd64 and arm64", mutate: func(value *sourceCatalog) {
			delete(value.Versions[0].Profiles, singbox.ArchitectureAMD64)
		}},
		{name: "invalid digest", match: "invalid amd64 asset metadata", mutate: func(value *sourceCatalog) {
			profile := value.Versions[0].Profiles[singbox.ArchitectureAMD64]
			profile.SHA256 = "invalid"
			value.Versions[0].Profiles[singbox.ArchitectureAMD64] = profile
		}},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			candidate := cloneCatalog(t, catalog)
			test.mutate(&candidate)
			if err := validateCatalog(candidate); err == nil || !strings.Contains(err.Error(), test.match) {
				t.Fatalf("validateCatalog error = %v, want %q", err, test.match)
			}
		})
	}
}

func TestCheckRejectsStaleGeneratedCatalog(t *testing.T) {
	t.Parallel()
	root, err := repositoryRoot()
	if err != nil {
		t.Fatal(err)
	}
	temporary := t.TempDir()
	destination := filepath.Join(temporary, "internal", "singbox")
	if err := os.MkdirAll(destination, 0o755); err != nil {
		t.Fatal(err)
	}
	catalog, err := os.ReadFile(filepath.Join(root, catalogPath))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(destination, "catalog.json"), catalog, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(destination, "catalog_generated.go"), []byte("stale\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := check(temporary); err == nil || !strings.Contains(err.Error(), "stale") {
		t.Fatalf("check error = %v, want stale generated catalog", err)
	}
}

func cloneCatalog(t *testing.T, value sourceCatalog) sourceCatalog {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	var clone sourceCatalog
	if err := json.Unmarshal(encoded, &clone); err != nil {
		t.Fatal(err)
	}
	return clone
}
