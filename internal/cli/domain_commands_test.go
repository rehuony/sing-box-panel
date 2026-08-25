// SPDX-License-Identifier: GPL-3.0-or-later

package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rehuony/sing-box-panel/internal/application"
)

func TestEntityRevisionAndTaskCommands(t *testing.T) {
	settingsPath := commandSettingsFixture(t)
	initial := runApplicationCommand(t, settingsPath,
		`{"schema_version":1,"global":{},"nodes":[],"rules":[],"subscription":{}}`,
		"--output", "json", "config", "replace", "--file", "-", "--base-revision", "none",
	)
	var first application.CanonicalSave
	if err := json.Unmarshal(initial, &first); err != nil {
		t.Fatalf("decode initial save: %v; output=%s", err, initial)
	}

	createdOutput := runApplicationCommand(t, settingsPath,
		`{"id":"node-a","kind":"outbound","enabled":true,"server":"example.com"}`,
		"--output", "json", "node", "create", "--file", "-", "--base-revision", first.Revision.ID,
	)
	var created application.CanonicalSave
	if err := json.Unmarshal(createdOutput, &created); err != nil {
		t.Fatalf("decode entity create: %v; output=%s", err, createdOutput)
	}
	if created.Revision.Sequence != 2 || created.TaskID == "" {
		t.Fatalf("entity create = %+v", created)
	}

	listedOutput := runApplicationCommand(t, settingsPath, "",
		"--output", "json", "node", "list",
	)
	var listed application.EntityList
	if err := json.Unmarshal(listedOutput, &listed); err != nil {
		t.Fatal(err)
	}
	if len(listed.Entities) != 1 || listed.Entities[0]["id"] != "node-a" {
		t.Fatalf("node list = %+v", listed)
	}

	diffOutput := runApplicationCommand(t, settingsPath, "",
		"--output", "json", "config", "revision", "diff", "#1", "#2",
	)
	var diff application.CanonicalRevisionDiff
	if err := json.Unmarshal(diffOutput, &diff); err != nil {
		t.Fatal(err)
	}
	if len(diff.Changes) != 1 || diff.Changes[0].Path != "/nodes" {
		t.Fatalf("revision diff = %+v", diff.Changes)
	}

	restoredOutput := runApplicationCommand(t, settingsPath, "",
		"--output", "json", "config", "revision", "restore", "#1", "--base-revision", created.Revision.ID,
	)
	var restored application.CanonicalSave
	if err := json.Unmarshal(restoredOutput, &restored); err != nil {
		t.Fatal(err)
	}
	if restored.Revision.Sequence != 3 {
		t.Fatalf("restored = %+v", restored)
	}

	canceledOutput := runApplicationCommand(t, settingsPath, "",
		"--output", "json", "task", "cancel", first.TaskID,
	)
	var canceled application.Task
	if err := json.Unmarshal(canceledOutput, &canceled); err != nil {
		t.Fatal(err)
	}
	if string(canceled.Status) != "canceled" {
		t.Fatalf("canceled task = %+v", canceled)
	}

	var stdout bytes.Buffer
	wait := NewRootCommand(Dependencies{
		Stdin:           strings.NewReader(""),
		Stdout:          &stdout,
		Stderr:          &bytes.Buffer{},
		OpenApplication: application.Open,
	})
	wait.SetArgs([]string{"--config", settingsPath, "--output", "json", "task", "wait", first.TaskID})
	err := wait.ExecuteContext(context.Background())
	if err == nil || ExitCode(err) != 4 || !strings.Contains(stdout.String(), `"status":"canceled"`) {
		t.Fatalf("wait canceled error=%v exit=%d output=%s", err, ExitCode(err), stdout.String())
	}
}

func TestEntityWritesRequireFileAndCurrentBase(t *testing.T) {
	settingsPath := commandSettingsFixture(t)
	initial := runApplicationCommand(t, settingsPath,
		`{"schema_version":1,"global":{},"nodes":[],"rules":[],"subscription":{}}`,
		"--output", "json", "config", "replace", "--file", "-", "--base-revision", "none",
	)
	var first application.CanonicalSave
	if err := json.Unmarshal(initial, &first); err != nil {
		t.Fatal(err)
	}

	command := NewRootCommand(Dependencies{
		Stdin:           strings.NewReader(`{"id":"node-a","kind":"outbound","enabled":true}`),
		Stdout:          &bytes.Buffer{},
		Stderr:          &bytes.Buffer{},
		OpenApplication: application.Open,
	})
	command.SetArgs([]string{"--config", settingsPath, "node", "create", "--base-revision", first.Revision.ID})
	if err := command.ExecuteContext(context.Background()); err == nil || ExitCode(err) != 3 {
		t.Fatalf("missing file error=%v exit=%d", err, ExitCode(err))
	}

	created := runApplicationCommand(t, settingsPath,
		`{"id":"node-a","kind":"outbound","enabled":true}`,
		"--output", "json", "node", "create", "--file", "-", "--base-revision", first.Revision.ID,
	)
	var save application.CanonicalSave
	if err := json.Unmarshal(created, &save); err != nil {
		t.Fatal(err)
	}

	stale := NewRootCommand(Dependencies{
		Stdin:           strings.NewReader(""),
		Stdout:          &bytes.Buffer{},
		Stderr:          &bytes.Buffer{},
		OpenApplication: application.Open,
	})
	stale.SetArgs([]string{"--config", settingsPath, "node", "disable", "node-a", "--base-revision", first.Revision.ID})
	if err := stale.ExecuteContext(context.Background()); err == nil || ExitCode(err) != 4 {
		t.Fatalf("stale mutation error=%v exit=%d latest=%s", err, ExitCode(err), save.Revision.ID)
	}
}

func runApplicationCommand(t *testing.T, settingsPath, stdin string, args ...string) []byte {
	t.Helper()
	var stdout bytes.Buffer
	command := NewRootCommand(Dependencies{
		Stdin:           strings.NewReader(stdin),
		Stdout:          &stdout,
		Stderr:          &bytes.Buffer{},
		OpenApplication: application.Open,
	})
	command.SetArgs(append([]string{"--config", settingsPath}, args...))
	if err := command.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("command %v error=%v output=%s", args, err, stdout.String())
	}
	return stdout.Bytes()
}

func commandSettingsFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	dataDir := filepath.Join(root, "data")
	if err := os.Mkdir(dataDir, 0o700); err != nil {
		t.Fatal(err)
	}
	settingsPath := filepath.Join(root, "setting.json")
	settingsJSON := fmt.Sprintf(`{
      "server":{"host":"127.0.0.1","port":3000,"base_path":""},
      "data_dir":%q,
      "auth":{"token":"test-token","secure_cookie":false},
      "github":{"token":"","catalog_ttl_hours":12},
      "traffic":{"quota_gib":null,"period_months":1},
      "subscription":{"author":"a","provider":"p","private_source_cidrs":[]},
      "logs":{"retention_days":7}
    }`, dataDir)
	if err := os.WriteFile(settingsPath, []byte(settingsJSON), 0o600); err != nil {
		t.Fatal(err)
	}
	return settingsPath
}
