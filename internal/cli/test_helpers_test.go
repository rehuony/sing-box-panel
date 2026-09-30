// SPDX-License-Identifier: GPL-3.0-or-later

package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/rehuony/sing-box-panel/internal/application"
	"github.com/rehuony/sing-box-panel/internal/auth"
	"github.com/rehuony/sing-box-panel/internal/testutil"
)

func runApplicationCommand(t *testing.T, settingsPath, stdin string, args ...string) []byte {
	t.Helper()
	var stdout bytes.Buffer
	command := NewRootCommand(Dependencies{
		Stdin: strings.NewReader(stdin), Stdout: &stdout, Stderr: &bytes.Buffer{},
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
      "auth":{"email":"admin@example.com","password_hash":%q},
      "github":{"token":"","catalog_refresh_interval_hours":12},
      "traffic":{"quota_gib":null,"period_months":1,"sample_retention_days":90},
      "subscription":{"private_source_cidrs":[]},
      "logs":{"core_retention_days":7}
    }`, dataDir, testutil.PasswordHash)
	if err := os.WriteFile(settingsPath, []byte(settingsJSON), 0o600); err != nil {
		t.Fatal(err)
	}
	return settingsPath
}

func saveConfigurationFixture(t *testing.T, path, content string) application.ConfigurationFile {
	t.Helper()
	app, err := application.Open(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer app.Close()
	file, err := app.SaveConfigurationFile(t.Context(), application.ConfigurationFileWrite{Revision: 0, Content: content})
	if err != nil {
		t.Fatal(err)
	}
	return file
}

func initialPasswordFromOutput(t *testing.T, output, hash string) string {
	t.Helper()
	var event map[string]any
	password := ""
	if json.Unmarshal([]byte(output), &event) == nil {
		password, _ = event["login_password"].(string)
	} else {
		match := regexp.MustCompile(`(?m)^  (?:Initial )?Password +([^\r\n]+)$`).FindStringSubmatch(output)
		if len(match) == 2 {
			password = match[1]
		}
	}
	matches, err := auth.VerifyPassword(t.Context(), password, hash)
	if err != nil || !matches || strings.Count(output, password) != 1 || strings.Contains(output, hash) {
		t.Fatal("initial password was not printed exactly once or does not match stored hash", err)
	}
	return password
}
