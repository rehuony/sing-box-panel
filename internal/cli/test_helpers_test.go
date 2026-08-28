// SPDX-License-Identifier: GPL-3.0-or-later

package cli

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rehuony/sing-box-panel/internal/application"
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
