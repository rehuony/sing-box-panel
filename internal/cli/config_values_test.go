// SPDX-License-Identifier: GPL-3.0-or-later

package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rehuony/sing-box-panel/internal/application"
	"github.com/spf13/cobra"
)

func TestConfigValueImportExportValidateAndEditCommands(t *testing.T) {
	settingsPath := commandSettingsFixture(t)
	initialOutput := runApplicationCommand(t, settingsPath,
		`{"experimental":{}}`,
		"--output", "json", "config", "import", "--file", "-", "--revision", "0",
	)
	var initial application.ConfigurationFile
	if err := json.Unmarshal(initialOutput, &initial); err != nil {
		t.Fatal(err)
	}
	if initial.Revision != 1 || initial.CanonicalRevisionID == "" {
		t.Fatalf("initial file = %+v", initial)
	}

	setOutput := runApplicationCommand(t, settingsPath, `"warn"`,
		"--output", "json", "config", "set", "/experimental/log_level", "--file", "-",
		"--base-revision", initial.CanonicalRevisionID,
	)
	var set application.CanonicalSave
	if err := json.Unmarshal(setOutput, &set); err != nil {
		t.Fatal(err)
	}
	if set.Revision.ID == "" || set.Revision.ID == initial.CanonicalRevisionID || set.NoChange {
		t.Fatalf("set result = %+v", set)
	}
	getOutput := runApplicationCommand(t, settingsPath, "",
		"--output", "json", "config", "get", "/experimental/log_level",
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
		"--output", "json", "config", "unset", "/experimental/log_level",
		"--base-revision", set.Revision.ID,
	)
	var unset application.CanonicalSave
	if err := json.Unmarshal(unsetOutput, &unset); err != nil {
		t.Fatal(err)
	}
	if unset.Revision.ID == "" || unset.Revision.ID == set.Revision.ID || unset.NoChange {
		t.Fatalf("unset result = %+v", unset)
	}
	file := runApplicationCommand(t, settingsPath, "", "--output", "json", "config", "show")
	var current application.ConfigurationFile
	if err := json.Unmarshal(file, &current); err != nil || current.Revision != 3 || current.CanonicalRevisionID != unset.Revision.ID {
		t.Fatalf("file after unset=%+v err=%v", current, err)
	}

	// A stale base revision is rejected as a conflict: the saved file was
	// already replaced by the unset above.
	stale := NewRootCommand(Dependencies{Stdin: strings.NewReader(`"debug"`), Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{}, OpenApplication: application.Open})
	stale.SetArgs([]string{"--config", settingsPath, "config", "set", "/experimental/log_level", "--file", "-", "--base-revision", set.Revision.ID})
	err = stale.ExecuteContext(context.Background())
	if ExitCode(err) != 4 {
		t.Fatalf("stale base revision exit=%d err=%v", ExitCode(err), err)
	}
}

func TestConfigHasNoHistoryCommands(t *testing.T) {
	for _, args := range [][]string{
		{"config", "history"}, {"config", "revision", "#1"}, {"config", "diff", "#1", "#2"}, {"config", "restore", "#1"},
	} {
		root := NewRootCommand(Dependencies{Stdin: strings.NewReader(""), Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{}, OpenApplication: application.Open})
		root.SetArgs(append([]string{"--config", commandSettingsFixture(t)}, args...))
		err := root.ExecuteContext(context.Background())
		if err == nil || !strings.Contains(err.Error(), "unknown command") {
			t.Fatalf("%v: err=%v, want unknown command", args, err)
		}
	}
}

func TestConfigEditResultDoesNotClaimAnotherWritersFileRevision(t *testing.T) {
	ctx := context.Background()
	instance, err := application.Open(ctx, commandSettingsFixture(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = instance.Close() })
	initial, err := instance.SaveConfigurationFile(ctx, application.ConfigurationFileWrite{Content: `{"log":{"level":"info"}}`})
	if err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	command := &cobra.Command{}
	command.SetContext(ctx)
	command.SetOut(&output)
	var own application.CanonicalSave
	err = renderConfigurationEdit(command, &options{format: outputJSON}, "set_failed", func(ctx context.Context) (application.CanonicalSave, error) {
		var err error
		own, err = instance.SetCanonicalValue(ctx, initial.CanonicalRevisionID, "/log/level", []byte(`"warn"`))
		if err != nil {
			return own, err
		}
		// Another writer commits between our write and rendering its result.
		_, err = instance.SaveConfigurationFile(ctx, application.ConfigurationFileWrite{Revision: 2, Content: `{"log":{"level":"error"}}`})
		return own, err
	})
	if err != nil {
		t.Fatal(err)
	}
	var result application.CanonicalSave
	if err := json.Unmarshal(output.Bytes(), &result); err != nil || result.Revision.ID != own.Revision.ID {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	current, err := instance.ConfigurationFile(ctx)
	if err != nil || current.Revision != 3 || current.CanonicalRevisionID == result.Revision.ID {
		t.Fatalf("later save=%+v err=%v", current, err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(output.Bytes(), &fields); err != nil {
		t.Fatal(err)
	}
	if _, exists := fields["file_revision"]; exists {
		t.Fatalf("edit output associates another writer's file revision with this save: %s", output.Bytes())
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
