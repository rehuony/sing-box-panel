// SPDX-License-Identifier: GPL-3.0-or-later

package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rehuony/sing-box-panel/internal/application"
	"github.com/rehuony/sing-box-panel/internal/capability"
	"github.com/rehuony/sing-box-panel/internal/coreartifact"
)

func TestCapabilityPackWritesCanonicalGenerationWithoutOpeningApplication(t *testing.T) {
	directory := t.TempDir()
	writeCapabilityManifestFixture(t, directory, "1.10.0", true)
	writeCapabilityManifestFixture(t, directory, "1.2.9", false)
	opened := false
	var stdout bytes.Buffer
	root := NewRootCommand(Dependencies{
		Stdout: &stdout, Stderr: &bytes.Buffer{},
		OpenApplication: func(context.Context, string) (*application.Application, error) {
			opened = true
			return nil, errors.New("must not open application")
		},
	})
	commit := strings.Repeat("a", 40)
	root.SetArgs([]string{
		"--config", filepath.Join(t.TempDir(), "missing-settings.json"),
		"core", "capability", "pack",
		"--directory", directory, "--commit", commit, "--file", "-",
	})
	if err := root.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("pack stdout: %v", err)
	}
	if opened {
		t.Fatal("pack opened application or settings")
	}
	generation, err := capability.DecodeGeneration(stdout.Bytes())
	if err != nil {
		t.Fatalf("stdout is not a canonical generation: %v; %s", err, stdout.Bytes())
	}
	entries := generation.Manifests()
	if generation.Commit() != commit || len(entries) != 2 ||
		entries[0].Path() != "capabilities/1.2.9.json" ||
		entries[1].Path() != "capabilities/1.10.0.json" {
		t.Fatalf("generation = commit %q entries %+v", generation.Commit(), entries)
	}
	canonical, err := generation.CanonicalJSON()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(stdout.Bytes(), canonical) {
		t.Fatalf("stdout differs from canonical generation\nstdout=%s\ncanonical=%s", stdout.Bytes(), canonical)
	}
}

func TestCapabilityPackFileRefusesOverwriteWithoutForce(t *testing.T) {
	directory := t.TempDir()
	writeCapabilityManifestFixture(t, directory, "1.2.3", false)
	destination := filepath.Join(t.TempDir(), "generation.json")
	firstCommit := strings.Repeat("b", 40)
	stdout, err := executeCapabilityPack(
		directory, firstCommit, destination,
	)
	if err != nil {
		t.Fatalf("first pack: %v", err)
	}
	var result capabilityPackResult
	if err := json.Unmarshal(stdout, &result); err != nil || result.ManifestCount != 1 || result.CommitSHA != firstCommit {
		t.Fatalf("result = %+v, err=%v; %s", result, err, stdout)
	}
	first, err := os.ReadFile(destination)
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(destination)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("destination mode = %o, want 600", info.Mode().Perm())
	}

	_, err = executeCapabilityPack(directory, strings.Repeat("c", 40), destination)
	var classified *Error
	if !errors.As(err, &classified) || classified.Code != "capability_pack_output_exists" || ExitCode(err) != 4 {
		t.Fatalf("overwrite error = %#v, exit=%d", err, ExitCode(err))
	}
	afterRefusal, err := os.ReadFile(destination)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(afterRefusal, first) {
		t.Fatal("refused overwrite changed destination")
	}

	secondCommit := strings.Repeat("d", 40)
	_, err = executeCapabilityPack(directory, secondCommit, destination, "--force")
	if err != nil {
		t.Fatalf("forced pack: %v", err)
	}
	replaced, err := os.ReadFile(destination)
	if err != nil {
		t.Fatal(err)
	}
	generation, err := capability.DecodeGeneration(replaced)
	if err != nil || generation.Commit() != secondCommit {
		t.Fatalf("forced generation commit = %q, err=%v", generation.Commit(), err)
	}
}

func TestCapabilityPackStableValidationErrors(t *testing.T) {
	commit := strings.Repeat("e", 40)
	t.Run("required flags", func(t *testing.T) {
		root := NewRootCommand(Dependencies{Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{}})
		root.SetArgs([]string{"core", "capability", "pack"})
		err := root.ExecuteContext(context.Background())
		assertCapabilityPackError(t, err, "capability_pack_flag_required", 2)
		if err == nil || err.Error() != "--directory, --commit, --file are required" {
			t.Fatalf("required error = %v", err)
		}
	})

	tests := []struct {
		name  string
		setup func(*testing.T) string
		code  string
	}{
		{
			name:  "empty directory",
			setup: func(t *testing.T) string { return t.TempDir() },
			code:  "capability_pack_directory_invalid",
		},
		{
			name: "non-regular entry",
			setup: func(t *testing.T) string {
				directory := t.TempDir()
				if err := os.Mkdir(filepath.Join(directory, "1.2.3.json"), 0o700); err != nil {
					t.Fatal(err)
				}
				return directory
			},
			code: "capability_pack_directory_invalid",
		},
		{
			name: "symlink entry",
			setup: func(t *testing.T) string {
				directory := t.TempDir()
				target := filepath.Join(t.TempDir(), "target.json")
				if err := os.WriteFile(target, capabilityPackManifestJSON(t, "1.2.3", false), 0o600); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(target, filepath.Join(directory, "1.2.3.json")); err != nil {
					t.Fatal(err)
				}
				return directory
			},
			code: "capability_pack_directory_invalid",
		},
		{
			name: "oversized entry",
			setup: func(t *testing.T) string {
				directory := t.TempDir()
				value := make([]byte, capability.MaximumManifestBytes+1)
				if err := os.WriteFile(filepath.Join(directory, "1.2.3.json"), value, 0o600); err != nil {
					t.Fatal(err)
				}
				return directory
			},
			code: "capability_pack_directory_invalid",
		},
		{
			name: "strict file name",
			setup: func(t *testing.T) string {
				directory := t.TempDir()
				if err := os.WriteFile(filepath.Join(directory, "manifest.json"), capabilityPackManifestJSON(t, "1.2.3", false), 0o600); err != nil {
					t.Fatal(err)
				}
				return directory
			},
			code: "capability_pack_invalid",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			directory := test.setup(t)
			_, err := executeCapabilityPack(directory, commit, "-")
			assertCapabilityPackError(t, err, test.code, 3)
		})
	}

	t.Run("directory symlink", func(t *testing.T) {
		target := t.TempDir()
		writeCapabilityManifestFixture(t, target, "1.2.3", false)
		link := filepath.Join(t.TempDir(), "manifests")
		if err := os.Symlink(target, link); err != nil {
			t.Fatal(err)
		}
		_, err := executeCapabilityPack(link, commit, "-")
		assertCapabilityPackError(t, err, "capability_pack_directory_invalid", 3)
	})
}

func executeCapabilityPack(directory, commit, destination string, extra ...string) ([]byte, error) {
	var stdout bytes.Buffer
	root := NewRootCommand(Dependencies{Stdout: &stdout, Stderr: &bytes.Buffer{}})
	arguments := []string{
		"--output", "json", "core", "capability", "pack",
		"--directory", directory, "--commit", commit, "--file", destination,
	}
	root.SetArgs(append(arguments, extra...))
	err := root.ExecuteContext(context.Background())
	return stdout.Bytes(), err
}

func assertCapabilityPackError(t *testing.T, err error, code string, exitCode int) {
	t.Helper()
	var classified *Error
	if !errors.As(err, &classified) || classified.Code != code || ExitCode(err) != exitCode {
		t.Fatalf("error = %#v, classified=%+v, exit=%d; want code %q exit %d", err, classified, ExitCode(err), code, exitCode)
	}
}

func writeCapabilityManifestFixture(t *testing.T, directory, version string, pretty bool) {
	t.Helper()
	if err := os.WriteFile(
		filepath.Join(directory, version+".json"),
		capabilityPackManifestJSON(t, version, pretty),
		0o600,
	); err != nil {
		t.Fatal(err)
	}
}

func capabilityPackManifestJSON(t *testing.T, value string, pretty bool) []byte {
	t.Helper()
	version, err := coreartifact.ParseExactVersion(value)
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := capability.NewManifest(capability.ManifestSpec{
		SchemaVersion: capability.ManifestSchemaVersion,
		CoreVersion:   version,
		SupportLevel:  capability.SupportManualJSON,
	})
	if err != nil {
		t.Fatal(err)
	}
	canonical, err := manifest.CanonicalJSON()
	if err != nil {
		t.Fatal(err)
	}
	if !pretty {
		return canonical
	}
	var indented bytes.Buffer
	if err := json.Indent(&indented, canonical, "", "  "); err != nil {
		t.Fatal(err)
	}
	return indented.Bytes()
}
