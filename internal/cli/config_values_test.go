// SPDX-License-Identifier: GPL-3.0-or-later

package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/rehuony/sing-box-panel/internal/application"
)

func TestConfigValueImportExportValidateAndDiffCommands(t *testing.T) {
	settingsPath := commandSettingsFixture(t)
	initialOutput := runApplicationCommand(t, settingsPath,
		`{"schema_version":2,"configuration":{"experimental":{}}}`,
		"--output", "json", "config", "import", "--file", "-", "--base-revision", "none",
	)
	var initial application.CanonicalSave
	if err := json.Unmarshal(initialOutput, &initial); err != nil {
		t.Fatal(err)
	}

	setOutput := runApplicationCommand(t, settingsPath, `"warn"`,
		"--output", "json", "config", "set", "/configuration/experimental/log_level", "--file", "-",
		"--base-revision", initial.Revision.ID,
	)
	var set application.CanonicalSave
	if err := json.Unmarshal(setOutput, &set); err != nil {
		t.Fatal(err)
	}
	getOutput := runApplicationCommand(t, settingsPath, "",
		"--output", "json", "config", "get", "/configuration/experimental/log_level",
	)
	var value application.CanonicalValue
	if err := json.Unmarshal(getOutput, &value); err != nil || value.Value != "warn" {
		t.Fatalf("value=%+v err=%v output=%s", value, err, getOutput)
	}

	exportPath := filepath.Join(t.TempDir(), "configuration.json")
	runApplicationCommand(t, settingsPath, "",
		"--output", "json", "config", "export", "--file", exportPath,
	)
	info, err := os.Stat(exportPath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("export permissions=%#o", info.Mode().Perm())
	}
	validateOutput := runApplicationCommand(t, settingsPath, "",
		"--output", "json", "config", "validate", "--file", exportPath,
	)
	if string(validateOutput) == "" || !json.Valid(validateOutput) {
		t.Fatalf("validate output=%s", validateOutput)
	}

	unsetOutput := runApplicationCommand(t, settingsPath, "",
		"--output", "json", "config", "unset", "/configuration/experimental/log_level",
		"--base-revision", set.Revision.ID,
	)
	var unset application.CanonicalSave
	if err := json.Unmarshal(unsetOutput, &unset); err != nil {
		t.Fatal(err)
	}
	diffOutput := runApplicationCommand(t, settingsPath, "",
		"--output", "json", "config", "diff", "--from", set.Revision.ID, "--to", unset.Revision.ID,
	)
	var diff application.CanonicalRevisionDiff
	if err := json.Unmarshal(diffOutput, &diff); err != nil || len(diff.Changes) != 1 || diff.Changes[0].Path != "/configuration/experimental/log_level" {
		t.Fatalf("diff=%+v err=%v output=%s", diff, err, diffOutput)
	}
}

func TestWritePrivateExportNeverOverwritesWithoutForce(t *testing.T) {
	directory := t.TempDir()
	destination := filepath.Join(directory, "configuration.json")
	if err := os.WriteFile(destination, []byte("original\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := writePrivateExport(destination, []byte("replacement\n"), false); err == nil {
		t.Fatal("writePrivateExport unexpectedly replaced an existing file")
	}
	contents, err := os.ReadFile(destination)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(contents, []byte("original\n")) {
		t.Fatalf("destination changed without force: %q", contents)
	}

	if err := writePrivateExport(destination, []byte("replacement\n"), true); err != nil {
		t.Fatal(err)
	}
	contents, err = os.ReadFile(destination)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(contents, []byte("replacement\n")) {
		t.Fatalf("forced destination = %q", contents)
	}
	info, err := os.Stat(destination)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("forced export permissions=%#o", info.Mode().Perm())
	}
}
