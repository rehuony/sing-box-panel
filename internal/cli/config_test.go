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

func TestConfigReplaceShowAndConflict(t *testing.T) {
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
	document := `{"schema_version":1,"global":{},"nodes":[],"rules":[],"subscription":{}}`

	var output bytes.Buffer
	command := NewRootCommand(Dependencies{
		Stdin:           strings.NewReader(document),
		Stdout:          &output,
		Stderr:          &bytes.Buffer{},
		OpenApplication: application.Open,
	})
	command.SetArgs([]string{"--config", settingsPath, "--output", "json", "config", "replace", "--file", "-", "--base-revision", "none"})
	if err := command.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("config replace error = %v", err)
	}
	var save struct {
		Revision struct {
			ID string `json:"id"`
		} `json:"revision"`
	}
	if err := json.Unmarshal(output.Bytes(), &save); err != nil {
		t.Fatalf("decode config replace output: %v; output=%s", err, output.String())
	}
	if save.Revision.ID == "" {
		t.Fatal("config replace returned an empty revision id")
	}

	output.Reset()
	show := NewRootCommand(Dependencies{Stdout: &output, Stderr: &bytes.Buffer{}, OpenApplication: application.Open})
	show.SetArgs([]string{"--config", settingsPath, "config", "show"})
	if err := show.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("config show error = %v", err)
	}
	if !strings.Contains(output.String(), `"schema_version": 1`) {
		t.Fatalf("config show output = %s", output.String())
	}

	stale := NewRootCommand(Dependencies{
		Stdin:           strings.NewReader(document),
		Stdout:          &bytes.Buffer{},
		Stderr:          &bytes.Buffer{},
		OpenApplication: application.Open,
	})
	stale.SetArgs([]string{"--config", settingsPath, "config", "replace", "--file", "-", "--base-revision", "none"})
	err := stale.ExecuteContext(context.Background())
	if err == nil || ExitCode(err) != 4 {
		t.Fatalf("stale config replace error = %v, exit = %d", err, ExitCode(err))
	}
}
