// SPDX-License-Identifier: GPL-3.0-or-later

package cli

import (
	"context"
	"encoding/json"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/rehuony/sing-box-panel/internal/application"
	"github.com/rehuony/sing-box-panel/internal/catalog"
	"github.com/rehuony/sing-box-panel/internal/settings"
	"github.com/rehuony/sing-box-panel/internal/store"
)

func TestCoreVersionParsingAndSelectionDefaults(t *testing.T) {
	for _, value := range []string{"1.13.21", "v1.13.21"} {
		if got, err := parseCoreVersion(value); err != nil || got != "1.13.21" {
			t.Fatalf("%s: %s, %v", value, got, err)
		}
	}
	for _, value := range []string{"", "1.13", "1.13.*", "latest", "1.13.21-beta.1", "01.13.21", " 1.13.21", "vv1.13.21", "12345", "core_abcdef012345", "0.0.0"} {
		if _, err := parseCoreVersion(value); err == nil {
			t.Fatalf("accepted %q", value)
		}
	}
	for _, operation := range []string{"install", "show", "enable", "remove", "list", "catalog", "import"} {
		root := NewRootCommand(Dependencies{})
		command, _, err := root.Find([]string{"core", operation})
		if err != nil {
			t.Fatal(err)
		}
		if got := command.Flags().Lookup("arch").DefValue; got != runtime.GOARCH {
			t.Fatalf("%s arch default = %s", operation, got)
		}
	}
	for _, args := range [][]string{{"show", "core_abcdef012345"}, {"install", "12345"}, {"enable", "1.13.*"}, {"remove", "1.13.21", "--arch", "386"}, {"import", "--sha256", "obsolete"}, {"catalog", "--installable"}} {
		root := NewRootCommand(Dependencies{OpenApplication: func(context.Context, string) (*application.Application, error) {
			t.Fatal("invalid input opened application")
			return nil, nil
		}})
		root.SetArgs(append([]string{"core"}, args...))
		if err := root.ExecuteContext(t.Context()); err == nil {
			t.Fatalf("accepted obsolete/invalid input %v", args)
		}
	}
}

func TestCoreBuildSelectionNeverGuesses(t *testing.T) {
	items := []application.CoreArtifact{
		{ID: "core_abcdef0123450aaa", ExactVersion: "1.13.21", Architecture: "arm64", SourceKind: store.CoreArtifactSourceOfficial},
		{ID: "core_abcdef0123451bbb", ExactVersion: "1.13.21", Architecture: "arm64", SourceKind: store.CoreArtifactSourceUserVerified},
	}
	builds := shortBuildIDs(items)
	if builds[items[0].ID] != "abcdef0123450" || builds[items[1].ID] != "abcdef0123451" {
		t.Fatalf("colliding builds: %v", builds)
	}
	for _, build := range []string{"", "abcdef012345", "abcdef", "missing"} {
		_, err := selectCoreBuild(items, "1.13.21", coreSelection{architecture: "arm64", build: build}, "enable")
		if err == nil || !strings.Contains(err.Error(), "sing-box-panel core enable 1.13.21 --arch arm64 --build abcdef0123450") || !strings.Contains(err.Error(), "abcdef0123451") {
			t.Fatalf("ambiguous selection = %v", err)
		}
	}
	for _, item := range items {
		got, err := selectCoreBuild(items, "1.13.21", coreSelection{architecture: "arm64", build: builds[item.ID]}, "show")
		if err != nil || got.ID != item.ID {
			t.Fatalf("build selection = %+v, %v", got, err)
		}
	}
	if _, err := selectCoreBuild(nil, "1.13.21", coreSelection{architecture: "arm64"}, "remove"); err == nil || !strings.Contains(err.Error(), "sing-box-panel core install 1.13.21 --arch arm64") {
		t.Fatalf("missing installation = %v", err)
	}
}

func TestCoreCLIUsesExactVersionsAndPreservesMachineIDs(t *testing.T) {
	path := commandSettingsFixture(t)
	for _, op := range []string{"catalog", "install"} {
		args := []string{"core", op}
		if op == "install" {
			args = append(args, "1.13.21")
		}
		_, _, err := executeSubscriptionCLI(t, path, "", args...)
		if err == nil || !strings.Contains(err.Error(), "sing-box-panel core refresh") || !strings.Contains(err.Error(), "sing-box-panel core catalog") {
			t.Fatalf("missing cache: %v", err)
		}
	}
	config, err := settings.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	database, err := store.Open(t.Context(), filepath.Join(config.DataDir, "panel.db"))
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(catalog.Catalog{RepositoryID: catalog.OfficialRepositoryID})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = database.SaveCatalogState(t.Context(), store.CatalogState{Catalog: data}); err != nil {
		t.Fatal(err)
	}
	id := "core_abcdef0123456789012345678901234567890123456789012345678901234567"
	if _, err = database.UpsertCoreArtifact(t.Context(), store.CoreArtifact{
		ID: id, ExactVersion: "1.13.21", ReportedVersion: "1.13.21", OperatingSystem: "linux", Architecture: runtime.GOARCH, Variant: "musl", SourceKind: store.CoreArtifactSourceOfficial,
		RepositoryID: 1, ReleaseID: 2, AssetID: 3, ArchiveSHA256: strings.Repeat("a", 64), BinarySHA256: strings.Repeat("b", 64), BinaryPath: filepath.Join(config.DataDir, "sing-box"),
	}); err != nil {
		t.Fatal(err)
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}
	_, _, err = executeSubscriptionCLI(t, path, "", "core", "install", "v1.13.21")
	if err == nil || !strings.Contains(err.Error(), "refresh --force") {
		t.Fatalf("missing exact catalog version: %v", err)
	}
	output := runApplicationCommand(t, path, "", "core", "show", "v1.13.21", "--output", "json")
	var artifact application.CoreArtifact
	if err := json.Unmarshal(output, &artifact); err != nil || artifact.ID != id {
		t.Fatalf("machine identity: %s, %v", output, err)
	}
	output = runApplicationCommand(t, path, "", "core", "list")
	for _, column := range []string{"VERSION", "ARCH", "SOURCE", "STATUS", "BUILD", "abcdef012345"} {
		if !strings.Contains(string(output), column) {
			t.Fatalf("list missing %s: %s", column, output)
		}
	}
	output = runApplicationCommand(t, path, "", "core", "show", "1.13.21")
	for _, label := range []string{"Version: 1.13.21", "Architecture:", "Source:", "Installation ID:"} {
		if !strings.Contains(string(output), label) {
			t.Fatalf("show missing %s: %s", label, output)
		}
	}
	completion, _, completionErr := executeSubscriptionCLI(t, path, "", "__complete", "core", "show", "1.13.")
	if completionErr != nil || !strings.Contains(string(completion), "1.13.21") {
		t.Fatalf("version completion: %s, %v", completion, completionErr)
	}
	completion, _, completionErr = executeSubscriptionCLI(t, path, "", "__complete", "core", "show", "v1.13.")
	if completionErr != nil || !strings.Contains(string(completion), "v1.13.21") {
		t.Fatalf("release tag completion: %s, %v", completion, completionErr)
	}
	completion, _, completionErr = executeSubscriptionCLI(t, path, "", "__complete", "core", "show", "1.13.21", "--build", "")
	if completionErr != nil || !strings.Contains(string(completion), "abcdef012345") {
		t.Fatalf("build completion: %s, %v", completion, completionErr)
	}
	_, _, err = executeSubscriptionCLI(t, path, "", "core", "show", "1.13.20")
	if err == nil || !strings.Contains(err.Error(), "core install 1.13.20") {
		t.Fatalf("neighbor selected: %v", err)
	}
}
