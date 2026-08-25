// SPDX-License-Identifier: GPL-3.0-or-later

package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rehuony/sing-box-panel/internal/buildinfo"
	"github.com/spf13/cobra"
)

func execute(t *testing.T, args ...string) (string, string, error) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	root := NewRootCommand(Dependencies{
		Stdin:  strings.NewReader(""),
		Stdout: &stdout,
		Stderr: &stderr,
		Build:  buildinfo.Info{Version: "v1.2.3", Commit: "abc", Date: "2026-08-26"},
		RunServer: func(context.Context, string) error {
			return nil
		},
	})
	root.SetArgs(args)
	err := root.ExecuteContext(context.Background())
	return stdout.String(), stderr.String(), err
}

func TestRootShowsHelp(t *testing.T) {
	stdout, stderr, err := execute(t)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout, "server") || !strings.Contains(stdout, "completion") {
		t.Fatalf("stdout = %q", stdout)
	}
	if stderr != "" {
		t.Fatalf("stderr = %q", stderr)
	}
}

func TestVersionJSON(t *testing.T) {
	stdout, stderr, err := execute(t, "version", "--output=json")
	if err != nil {
		t.Fatal(err)
	}
	if stderr != "" {
		t.Fatalf("stderr = %q", stderr)
	}
	var value buildinfo.Info
	if err := json.Unmarshal([]byte(stdout), &value); err != nil {
		t.Fatalf("stdout is not JSON: %v; %q", err, stdout)
	}
	if value.Version != "v1.2.3" || value.Commit != "abc" {
		t.Fatalf("version = %#v", value)
	}
}

func TestInitAndVerify(t *testing.T) {
	root := t.TempDir()
	t.Setenv("XDG_DATA_HOME", filepath.Join(root, "data-home"))
	path := filepath.Join(root, "setting.json")
	stdout, stderr, err := execute(t, "init", "--config", path)
	if err != nil {
		t.Fatal(err)
	}
	if stderr != "" || !strings.Contains(stdout, "initialized") {
		t.Fatalf("stdout=%q stderr=%q", stdout, stderr)
	}
	stdout, stderr, err = execute(t, "verify", "--config", path, "--output=json")
	if err != nil {
		t.Fatal(err)
	}
	if stderr != "" || !strings.Contains(stdout, `"valid":true`) {
		t.Fatalf("stdout=%q stderr=%q", stdout, stderr)
	}
}

func TestCommandTreeIncludesContract(t *testing.T) {
	paths := map[string]bool{}
	var walk func(*cobra.Command)
	walk = func(command *cobra.Command) {
		paths[command.CommandPath()] = true
		for _, child := range command.Commands() {
			walk(child)
		}
	}
	var stdout, stderr bytes.Buffer
	root := NewRootCommand(Dependencies{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr})
	walk(root)
	for _, path := range []string{
		"sing-box-panel server run",
		"sing-box-panel core capability pack",
		"sing-box-panel core capability upgrade",
		"sing-box-panel core capability quarantine",
		"sing-box-panel core quarantine",
		"sing-box-panel core revoke",
		"sing-box-panel config manual reattach preview",
		"sing-box-panel subscription source refresh",
		"sing-box-panel task cancel",
		"sing-box-panel completion fish",
	} {
		if !paths[path] {
			t.Errorf("missing command path %q", path)
		}
	}
}

func TestUnavailableExitClass(t *testing.T) {
	_, _, err := execute(t, "core", "list")
	if ExitCode(err) != 6 {
		t.Fatalf("ExitCode() = %d, error = %v", ExitCode(err), err)
	}
}
