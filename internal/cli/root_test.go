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
	if !strings.Contains(stdout, "server") || !strings.Contains(stdout, "update") || !strings.Contains(stdout, "completion") {
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

// visibleLeafCapabilities is the complete public command inventory. Every
// entry is a runnable leaf at most two words deep; hierarchy changes must
// keep this list's size and either keep or deliberately rename its entries.
var visibleLeafCapabilities = []string{
	"init", "verify", "version", "update",
	"server start", "server stop", "server status",
	"core catalog", "core refresh",
	"core list", "core show", "core install", "core import", "core remove", "core quarantine", "core revoke",
	"core enable", "core status", "core start", "core stop", "core restart", "core rollback",
	"config show", "config export", "config import", "config validate",
	"config get", "config set", "config unset",
	"config check", "config apply",
	"channel list", "channel show", "channel create", "channel update", "channel delete", "channel render",
	"source list", "source show", "source create", "source update", "source refresh", "source delete",
	"token list", "token create", "token rotate", "token revoke",
	"task list", "task show", "task wait", "task cancel",
	"log list", "log show", "log tail", "log clear", "log delete",
	"metrics show", "metrics watch", "metrics history", "metrics period",
	"system files", "system clean", "system install", "system uninstall", "system status", "system start", "system stop", "system restart", "system logs",
	"completion bash", "completion zsh", "completion fish",
}

func TestCommandTreeIsAtMostTwoWordsDeepAndKeepsEveryCapability(t *testing.T) {
	if len(visibleLeafCapabilities) != 72 {
		t.Fatalf("inventory lists %d capabilities, want 72", len(visibleLeafCapabilities))
	}
	var stdout, stderr bytes.Buffer
	root := NewRootCommand(Dependencies{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr})
	leaves := map[string]bool{}
	var walk func(*cobra.Command, int)
	walk = func(command *cobra.Command, depth int) {
		path := strings.TrimPrefix(command.CommandPath(), "sing-box-panel ")
		if command.Hidden {
			t.Errorf("command %q is hidden; the tree has no hidden entry points", path)
		}
		if depth > 2 {
			t.Errorf("command %q is %d words deep", path, depth)
		}
		children := command.Commands()
		for _, child := range children {
			walk(child, depth+1)
		}
		if depth > 0 && len(children) == 0 && command.Runnable() {
			leaves[path] = true
		}
	}
	walk(root, 0)
	for _, path := range visibleLeafCapabilities {
		if !leaves[path] {
			t.Errorf("missing capability %q", path)
		}
		delete(leaves, path)
	}
	for path := range leaves {
		t.Errorf("unexpected leaf %q", path)
	}
	for _, path := range []string{
		"server run",
		"config history", "config revision", "config diff", "config restore", "config compile", "config replace", "config revision list", "config revision show", "config revision diff", "config revision restore",
		"core check", "core activate", "core catalog list", "core catalog refresh",
		"subscription", "subscription channel", "subscription source", "subscription token",
		"traffic", "traffic status", "traffic list", "traffic show", "traffic period", "traffic period list", "traffic period show",
	} {
		if command, _, err := root.Find(strings.Fields(path)); err == nil && command.CommandPath() == "sing-box-panel "+path {
			t.Errorf("superseded command path %q is still registered", path)
		}
	}
}

func TestServerStartIsForegroundAndGroupDoesNotStart(t *testing.T) {
	var started []string
	newRoot := func(stdout, stderr *bytes.Buffer) *cobra.Command {
		return NewRootCommand(Dependencies{
			Stdin: strings.NewReader(""), Stdout: stdout, Stderr: stderr,
			RunServer: func(_ context.Context, settingsPath string) error {
				started = append(started, settingsPath)
				return nil
			},
		})
	}
	var stdout, stderr bytes.Buffer
	root := newRoot(&stdout, &stderr)
	root.SetArgs([]string{"server", "start", "--config", "/tmp/settings.json"})
	if err := root.ExecuteContext(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(started) != 1 || started[0] != "/tmp/settings.json" {
		t.Fatalf("started %v", started)
	}
	for _, args := range [][]string{{"server"}, {"server", "--help"}} {
		started = nil
		stdout.Reset()
		root = newRoot(&stdout, &stderr)
		root.SetArgs(args)
		if err := root.ExecuteContext(context.Background()); err != nil {
			t.Fatal(err)
		}
		if len(started) != 0 || !strings.Contains(stdout.String(), "Available Commands") {
			t.Fatalf("group started %v or printed unexpected help: %q", started, stdout.String())
		}
	}
	root = newRoot(&stdout, &stderr)
	root.SetArgs([]string{"server", "run"})
	if err := root.ExecuteContext(context.Background()); err == nil || len(started) != 0 {
		t.Fatalf("obsolete command accepted: err=%v started=%v", err, started)
	}
}

func TestConfigCheckAndApplyExposeOnlyCoreSelection(t *testing.T) {
	var stdout, stderr bytes.Buffer
	root := NewRootCommand(Dependencies{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr})
	for _, path := range []string{"sing-box-panel config check", "sing-box-panel config apply"} {
		command, _, err := root.Find(strings.Fields(path)[1:])
		if err != nil {
			t.Fatal(err)
		}
		for _, removed := range []string{"artifact", "monitoring"} {
			if command.Flags().Lookup(removed) != nil {
				t.Errorf("%s still exposes --%s", path, removed)
			}
		}
		if command.Flags().Lookup("core") == nil {
			t.Errorf("%s does not expose --core", path)
		}
	}
}

func TestUnavailableExitClass(t *testing.T) {
	_, _, err := execute(t, "core", "list")
	if ExitCode(err) != 6 {
		t.Fatalf("ExitCode() = %d, error = %v", ExitCode(err), err)
	}
}
