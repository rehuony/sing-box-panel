// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
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
		{name: "unknown schema source", match: "unknown schema source", mutate: func(value *sourceCatalog) {
			value.Versions[0].SchemaSource = "unknown"
		}},
		{name: "native schema before 1.14", match: "no native schema command", mutate: func(value *sourceCatalog) {
			value.Versions[0].SchemaSource = singbox.SchemaSourceNative
		}},
		{name: "reviewed schema outside 1.13", match: "outside the reviewed 1.13 schema contract", mutate: func(value *sourceCatalog) {
			value.Versions[len(value.Versions)-1].SchemaSource = singbox.SchemaSourceReviewed113
		}},
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
		{name: "non-musl fingerprint", match: "must include with_musl", mutate: func(value *sourceCatalog) {
			profile := value.Versions[0].Profiles[singbox.ArchitectureAMD64]
			profile.Features = slices.DeleteFunc(profile.Features, func(feature string) bool { return feature == "with_musl" })
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

func TestSupportGuidePreservesAuthoredTextAndDetectsStaleVersions(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	filename := filepath.Join(root, supportGuidePath)
	if err := os.MkdirAll(filepath.Dir(filename), 0o755); err != nil {
		t.Fatal(err)
	}
	prefix, suffix := "# Core versions\n\n", "\n\nKeep the reviewed migration notes.\n"
	if err := os.WriteFile(filename, []byte(prefix+supportTableStart+"\nstale\n"+supportTableEnd+suffix), 0o600); err != nil {
		t.Fatal(err)
	}
	catalog := sourceCatalog{Versions: []singbox.Version{
		{ExactVersion: "1.14.0", SchemaSource: singbox.SchemaSourceNative, InboundFamily: "1.14"},
		{ExactVersion: "1.14.1", SchemaSource: singbox.SchemaSourceNative, InboundFamily: "1.14"},
	}}
	if err := syncSupportGuide(root, catalog, true); err == nil || !strings.Contains(err.Error(), "stale") {
		t.Fatalf("check stale support table: %v", err)
	}
	if err := syncSupportGuide(root, catalog, false); err != nil {
		t.Fatal(err)
	}
	if err := syncSupportGuide(root, catalog, true); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filename)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(raw), prefix+supportTableStart) || !strings.HasSuffix(string(raw), supportTableEnd+suffix) {
		t.Fatal("generation changed authored text outside the support table")
	}
	if !strings.Contains(string(raw), "| 1.14.0, 1.14.1 | Official binary's native `schema` command | 1.14 |") {
		t.Fatal("support table does not group the exact catalog versions")
	}
	catalog.Versions = catalog.Versions[:1]
	if err := syncSupportGuide(root, catalog, true); err == nil || !strings.Contains(err.Error(), "stale") {
		t.Fatalf("check changed version catalog: %v", err)
	}
	for _, malformed := range []string{supportTableStart, supportTableEnd + supportTableStart, supportTableStart + supportTableStart + supportTableEnd} {
		if _, err := renderSupportGuide(malformed, catalog); err == nil {
			t.Fatalf("accepted invalid table markers: %s", malformed)
		}
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
