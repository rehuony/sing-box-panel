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
)

func TestConfigImportShowExportPreserveRawTextAndUseFileRevisionCAS(t *testing.T) {
	settingsPath := commandSettingsFixture(t)
	valid := "{\n  \"log\": {\"level\": \"info\"},\n  \"big\": 9007199254740993\n}\n"

	importOutput := runApplicationCommand(t, settingsPath, valid,
		"--output", "json", "config", "import", "--file", "-", "--revision", "0",
	)
	var saved application.ConfigurationFile
	if err := json.Unmarshal(importOutput, &saved); err != nil {
		t.Fatalf("decode import output: %v; %s", err, importOutput)
	}
	if saved.Revision != 1 || !saved.SyntaxValid || saved.CanonicalRevisionID == "" || saved.Content != valid {
		t.Fatalf("import result = %+v", saved)
	}

	shown := runApplicationCommand(t, settingsPath, "", "config", "show")
	if string(shown) != valid {
		t.Fatalf("config show text = %q, want exact stored text %q", shown, valid)
	}

	invalid := "{\n  \"log\": {\"level\": \"info\"},\n"
	draftOutput := runApplicationCommand(t, settingsPath, invalid,
		"--output", "json", "config", "import", "--file", "-", "--revision", "1",
	)
	var draft application.ConfigurationFile
	if err := json.Unmarshal(draftOutput, &draft); err != nil {
		t.Fatal(err)
	}
	if draft.Revision != 2 || draft.SyntaxValid || draft.CanonicalRevisionID != "" || draft.Content != invalid {
		t.Fatalf("invalid draft result = %+v", draft)
	}
	textOutput := runApplicationCommand(t, settingsPath, "{", "config", "import", "--file", "-", "--revision", "2")
	if !strings.Contains(string(textOutput), "revision 3 (invalid JSON)") || !strings.Contains(string(textOutput), "blocked") {
		t.Fatalf("invalid import text = %q", textOutput)
	}

	exported := runApplicationCommand(t, settingsPath, "", "config", "export", "--file", "-")
	if string(exported) != "{" {
		t.Fatalf("export stdout = %q, want exact invalid draft", exported)
	}
	shown = runApplicationCommand(t, settingsPath, "", "config", "show")
	if string(shown) != "{" {
		t.Fatalf("show changed unterminated draft bytes: %q", shown)
	}
	exportPath := filepath.Join(t.TempDir(), "config.json")
	runApplicationCommand(t, settingsPath, "", "config", "export", "--file", exportPath)
	contents, err := os.ReadFile(exportPath)
	if err != nil || string(contents) != "{" {
		t.Fatalf("export file = %q err=%v", contents, err)
	}

	// Field edits and checks are guarded while the saved file is invalid.
	var stdout bytes.Buffer
	command := NewRootCommand(Dependencies{Stdin: strings.NewReader(`"debug"`), Stdout: &stdout, Stderr: &bytes.Buffer{}, OpenApplication: application.Open})
	command.SetArgs([]string{"--config", settingsPath, "config", "set", "/log/level", "--file", "-", "--base-revision", saved.CanonicalRevisionID})
	err = command.ExecuteContext(context.Background())
	if ExitCode(err) != 3 || !strings.Contains(err.Error(), "not valid JSON") {
		t.Fatalf("set on invalid file error=%v exit=%d", err, ExitCode(err))
	}
	stale := NewRootCommand(Dependencies{Stdin: strings.NewReader("{}"), Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{}, OpenApplication: application.Open})
	stale.SetArgs([]string{"--config", settingsPath, "config", "import", "--file", "-", "--revision", "1"})
	err = stale.ExecuteContext(context.Background())
	if ExitCode(err) != 4 {
		t.Fatalf("stale import error = %v, exit = %d", err, ExitCode(err))
	}

	missing := NewRootCommand(Dependencies{Stdin: strings.NewReader("{}"), Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{}, OpenApplication: application.Open})
	missing.SetArgs([]string{"--config", settingsPath, "config", "import", "--file", "-"})
	err = missing.ExecuteContext(context.Background())
	if ExitCode(err) != 2 {
		t.Fatalf("import without --revision error = %v, exit = %d", err, ExitCode(err))
	}
}

func TestCoreSelectionRejectsExplicitBlankArtifactBeforeOpeningApplication(t *testing.T) {
	for _, args := range [][]string{
		{"core", "enable", ""}, {"core", "enable", " \t "},
		{"config", "check", "--core", ""}, {"config", "check", "--core", " \t "},
		{"config", "apply", "--core", ""}, {"config", "apply", "--core", " \t "},
	} {
		opened := false
		command := NewRootCommand(Dependencies{
			Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{},
			OpenApplication: func(context.Context, string) (*application.Application, error) {
				opened = true
				return nil, errors.New("unexpected application open")
			},
		})
		command.SetArgs(args)
		err := command.ExecuteContext(context.Background())
		if ExitCode(err) != 2 || opened {
			t.Fatalf("blank artifact %q: opened=%t error=%v exit=%d", args, opened, err, ExitCode(err))
		}
	}
}

func TestConfigCheckAndApplyRequireExplicitCoreBeforeFirstApply(t *testing.T) {
	settingsPath := commandSettingsFixture(t)
	runApplicationCommand(t, settingsPath, "{}", "config", "import", "--file", "-", "--revision", "0")

	for _, args := range [][]string{{"config", "check"}, {"config", "apply"}} {
		command := NewRootCommand(Dependencies{Stdin: strings.NewReader(""), Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{}, OpenApplication: application.Open})
		command.SetArgs(append([]string{"--config", settingsPath}, args...))
		err := command.ExecuteContext(context.Background())
		if ExitCode(err) != 2 || !strings.Contains(err.Error(), "--core") {
			t.Fatalf("%v error=%v exit=%d", args, err, ExitCode(err))
		}
	}
	for _, args := range [][]string{{"config", "check", "--core", "core-missing"}, {"config", "apply", "--core", "core-missing"}, {"core", "enable", "core-missing"}} {
		command := NewRootCommand(Dependencies{Stdin: strings.NewReader(""), Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{}, OpenApplication: application.Open})
		command.SetArgs(append([]string{"--config", settingsPath, "--output", "json"}, args...))
		err := command.ExecuteContext(context.Background())
		var classified *Error
		if ExitCode(err) != 1 || !errors.As(err, &classified) || classified.Code != "core_artifact_not_found" {
			t.Fatalf("%v error=%v exit=%d", args, err, ExitCode(err))
		}
	}
}
