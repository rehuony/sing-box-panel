// SPDX-License-Identifier: GPL-3.0-or-later

package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/rehuony/sing-box-panel/internal/buildinfo"
	"github.com/rehuony/sing-box-panel/internal/settings"
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

func TestHelpSectionOrder(t *testing.T) {
	for _, test := range []struct {
		name        string
		args        []string
		usage       string
		inherited   bool
		subcommands bool
		examples    bool
	}{
		{name: "root", usage: "[flags] [command]", subcommands: true},
		{name: "root flag", args: []string{"--help"}, usage: "[flags] [command]", subcommands: true},
		{name: "root short flag", args: []string{"-h"}, usage: "[flags] [command]", subcommands: true},
		{name: "root help command", args: []string{"help"}, usage: "[flags] [command]", subcommands: true},
		{name: "group", args: []string{"core"}, usage: "core [flags] [command]", inherited: true, subcommands: true},
		{name: "group flag", args: []string{"core", "--help"}, usage: "core [flags] [command]", inherited: true, subcommands: true},
		{name: "group help command", args: []string{"help", "core"}, usage: "core [flags] [command]", inherited: true, subcommands: true},
		{name: "completion group", args: []string{"completion", "--help"}, usage: "completion [flags] [command]", inherited: true, subcommands: true},
		{name: "system group", args: []string{"system", "--help"}, usage: "system [flags] [command]", inherited: true, subcommands: true},
		{name: "systemd group", args: []string{"systemd", "--help"}, usage: "systemd [flags] [command]", inherited: true, subcommands: true},
		{name: "config group", args: []string{"config", "--help"}, usage: "config [flags] [command]", inherited: true, subcommands: true},
		{name: "config set", args: []string{"config", "set", "--help"}, usage: "config set [flags]", inherited: true, examples: true},
		{name: "config check", args: []string{"config", "check", "--help"}, usage: "config check [flags]", inherited: true, examples: true},
		{name: "leaf flag", args: []string{"core", "install", "--help"}, usage: "core install ASSET_ID [flags]", inherited: true},
		{name: "leaf help command", args: []string{"help", "core", "install"}, usage: "core install ASSET_ID [flags]", inherited: true},
		{name: "period argument", args: []string{"metrics", "period", "--help"}, usage: "metrics period PERIOD_ID [flags]", inherited: true},
		{name: "optional help argument", args: []string{"help", "--help"}, usage: "help [command] [flags]", inherited: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			stdout, stderr, err := execute(t, test.args...)
			if err != nil || stderr != "" {
				t.Fatalf("help error=%v stderr=%q", err, stderr)
			}
			_, after, found := strings.Cut(stdout, "\nUsage:\n  ")
			usage, _, _ := strings.Cut(after, "\n\n")
			if want := "sing-box-panel " + test.usage; !found || usage != want {
				t.Fatalf("usage = %q, want one line %q", usage, want)
			}
			_, flags, _ := strings.Cut(stdout, "\nFlags:\n")
			if !strings.HasPrefix(flags, "  -h, --help ") {
				t.Fatalf("help must be the first local flag:\n%s", flags)
			}
			if !strings.Contains(stdout, "-o, --output format") {
				t.Fatalf("help is missing the lowercase output shorthand:\n%s", stdout)
			}
			previous := -1
			for _, section := range []struct {
				heading string
				present bool
			}{
				{"\nUsage:\n", true},
				{"\nExamples:\n", test.examples},
				{"\nFlags:\n", true},
				{"\nGlobal Flags:\n", test.inherited},
				{"\nAvailable Commands:\n", test.subcommands},
				{"\nUse \"", test.subcommands},
			} {
				index := strings.Index(stdout, section.heading)
				if !section.present {
					if index != -1 {
						t.Errorf("unexpected section %q in help:\n%s", section.heading, stdout)
					}
					continue
				}
				if index <= previous || strings.Count(stdout, section.heading) != 1 {
					t.Fatalf("section %q is missing, duplicated, or out of order:\n%s", section.heading, stdout)
				}
				previous = index
			}
		})
	}
}

func TestHelpFlagsKeepStableOrder(t *testing.T) {
	commands := [][]string{{"--help"}}
	for _, path := range visibleLeafCapabilities {
		commands = append(commands, append(strings.Fields(path), "--help"))
	}
	for _, args := range commands {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			stdout, stderr, err := execute(t, args...)
			if err != nil || stderr != "" {
				t.Fatalf("help error=%v stderr=%q", err, stderr)
			}
			_, after, found := strings.Cut(stdout, "\nFlags:\n")
			flags, _, _ := strings.Cut(after, "\n\n")
			if !found || !strings.HasPrefix(flags, "  -h, --help ") {
				t.Fatalf("help is not first:\n%s", flags)
			}
			var names []string
			for _, line := range strings.Split(flags, "\n")[1:] {
				_, name, found := strings.Cut(line, "--")
				if !found {
					t.Fatalf("invalid flag usage: %q", line)
				}
				names = append(names, strings.Fields(name)[0])
			}
			if !slices.IsSorted(names) || slices.Contains(names, "help") {
				t.Fatalf("remaining flags are unordered or help is repeated: %v", names)
			}
		})
	}
}

func TestGlobalFlagsCombineWithSubcommands(t *testing.T) {
	for _, args := range [][]string{
		{"--output=json", "config", "check"},
		{"config", "--output=json", "check"},
		{"config", "check", "--output=json"},
		{"-o", "json", "config", "check"},
		{"config", "-o=json", "check"},
		{"config", "check", "-o", "json"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			root := NewRootCommand(Dependencies{Stdin: strings.NewReader("{}"), Stdout: &stdout, Stderr: &stderr})
			root.SetArgs(append(args, "--config", commandSettingsFixture(t)))
			if err := root.ExecuteContext(t.Context()); err != nil || stderr.Len() != 0 {
				t.Fatalf("command error=%v stderr=%q", err, stderr.String())
			}
			var result struct {
				Valid bool `json:"valid"`
			}
			if err := json.Unmarshal(stdout.Bytes(), &result); err != nil || !result.Valid {
				t.Fatalf("global output flag was not applied: stdout=%q error=%v", stdout.String(), err)
			}
		})
	}
}

func TestOutputShorthand(t *testing.T) {
	for _, format := range []string{"text", "json", "jsonl"} {
		want, stderr, err := execute(t, "version", "--output="+format)
		if err != nil || stderr != "" {
			t.Fatalf("long flag error=%v stderr=%q", err, stderr)
		}
		for _, args := range [][]string{{"-o", format, "version"}, {"version", "-o=" + format}} {
			t.Run(strings.Join(args, " "), func(t *testing.T) {
				stdout, stderr, err := execute(t, args...)
				if err != nil || stderr != "" || stdout != want {
					t.Fatalf("shorthand stdout=%q stderr=%q error=%v, want %q", stdout, stderr, err, want)
				}
			})
		}
	}
}

func TestOutputShorthandRejectsInvalidFormat(t *testing.T) {
	for _, args := range [][]string{
		{"-o", "yaml", "version"}, {"version", "-o=yaml"}, {"--output=yaml", "version"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			stdout, _, err := execute(t, args...)
			if ExitCode(err) != 2 || stdout != "" || !strings.Contains(err.Error(), "output must be text, json, or jsonl") {
				t.Fatalf("invalid output stdout=%q error=%v", stdout, err)
			}
		})
	}
}

func TestOutputShorthandRejectsUppercaseAlias(t *testing.T) {
	stdout, _, err := execute(t, "version", "-O=json")
	if err == nil || stdout != "" || !strings.Contains(err.Error(), "unknown shorthand flag: 'O'") {
		t.Fatalf("uppercase shorthand stdout=%q error=%v", stdout, err)
	}
}

func TestVersionOutput(t *testing.T) {
	for _, test := range []struct {
		version string
		text    string
	}{
		{"v1.2.3", "sing-box-panel v1.2.3\n"},
		{"v1.2.3-rc.1+build.5", "sing-box-panel v1.2.3-rc.1+build.5\n"},
		{"", "sing-box-panel unknown\n"},
		{"dev", "sing-box-panel unknown\n"},
		{"(devel)", "sing-box-panel unknown\n"},
		{"unknown", "sing-box-panel unknown\n"},
		{"v0.0.2-0.20260919073707-361e5be561c1", "sing-box-panel v0.0.2-0.20260919073707-361e5be561c1\n"},
		{"v0.0.2-0.20260919073707-361e5be561c1+dirty", "sing-box-panel v0.0.2-0.20260919073707-361e5be561c1+dirty\n"},
	} {
		for _, format := range []string{"", "text", "json", "jsonl"} {
			t.Run(test.version+"/"+format, func(t *testing.T) {
				var stdout, stderr bytes.Buffer
				info := buildinfo.Info{Version: test.version, Commit: "abc", Date: "2026-08-26"}
				root := NewRootCommand(Dependencies{Stdout: &stdout, Stderr: &stderr, Build: info})
				args := []string{"version"}
				if format != "" {
					args = append(args, "--output="+format)
				}
				root.SetArgs(args)
				if err := root.ExecuteContext(t.Context()); err != nil {
					t.Fatal(err)
				}
				if stderr.Len() != 0 {
					t.Fatalf("stderr = %q", stderr.String())
				}
				want := test.text
				if format == "json" || format == "jsonl" {
					encoded, err := json.Marshal(info)
					if err != nil {
						t.Fatal(err)
					}
					want = string(encoded) + "\n"
				}
				if stdout.String() != want {
					t.Fatalf("stdout = %q, want %q", stdout.String(), want)
				}
			})
		}
	}
}

func TestInitAndConfigCheck(t *testing.T) {
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
	stdout, stderr, err = execute(t, "config", "check", "--config", path, "--output=json")
	if err != nil {
		t.Fatal(err)
	}
	if stderr != "" || !strings.Contains(stdout, `"valid":true`) {
		t.Fatalf("stdout=%q stderr=%q", stdout, stderr)
	}
}

func TestForcedInitMigratesExistingDataBeforeOpeningStorage(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "setting.json")
	t.Setenv("XDG_DATA_HOME", filepath.Join(root, "old-home"))
	if _, _, err := execute(t, "init", "--config", path); err != nil {
		t.Fatal(err)
	}
	old, err := settings.LoadDataDir(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(old, "retained.txt"), []byte("original"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_DATA_HOME", filepath.Join(root, "new-home"))
	if _, _, err := execute(t, "init", "--force", "--config", path); err != nil {
		t.Fatal(err)
	}
	next, err := settings.LoadDataDir(path)
	if err != nil || next == old {
		t.Fatalf("location did not move: %s %v", next, err)
	}
	content, err := os.ReadFile(filepath.Join(next, "retained.txt"))
	if err != nil || string(content) != "original" {
		t.Fatalf("original data was not migrated: %s %v", content, err)
	}
}

// visibleLeafCapabilities is the complete public command inventory. Every
// entry is a runnable leaf at most two words deep; hierarchy changes must
// deliberately update this inventory when changing the public surface.
var visibleLeafCapabilities = []string{
	"init", "version", "update",
	"server start", "server stop", "server status",
	"core catalog", "core refresh",
	"core list", "core show", "core install", "core import", "core remove", "core quarantine", "core revoke",
	"core enable", "core status", "core start", "core stop", "core restart", "core rollback",
	"config show", "config set", "config check",
	"channel list", "channel show", "channel create", "channel update", "channel delete", "channel render",
	"source list", "source show", "source create", "source update", "source refresh", "source delete",
	"token list", "token create", "token rotate", "token revoke",
	"task list", "task show", "task wait", "task cancel",
	"log list", "log show", "log tail", "log clear", "log delete",
	"metrics show", "metrics watch", "metrics history", "metrics period",
	"system df", "system prune",
	"systemd install", "systemd uninstall", "systemd status", "systemd start", "systemd stop", "systemd restart", "systemd logs",
	"completion bash", "completion zsh", "completion fish",
}

func TestCommandTreeIsAtMostTwoWordsDeepAndKeepsEveryCapability(t *testing.T) {
	if len(visibleLeafCapabilities) != 65 {
		t.Fatalf("inventory lists %d capabilities, want 65", len(visibleLeafCapabilities))
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
		"system clean", "system prn", "system file", "system files",
		"system install", "system uninstall", "system status", "system start", "system stop", "system restart", "system logs",
		"server run", "verify", "config export", "config import", "config validate", "config get", "config unset", "config apply",
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
	settingsPath := commandSettingsFixture(t)
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
	root.SetArgs([]string{"server", "start", "--config", settingsPath})
	if err := root.ExecuteContext(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(started) != 1 || started[0] != settingsPath {
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

func TestConfigCommandsOnlyExposePanelSettingsFlags(t *testing.T) {
	root := NewRootCommand(Dependencies{})
	for _, name := range []string{"show", "set", "check"} {
		command, _, err := root.Find([]string{"config", name})
		if err != nil {
			t.Fatal(err)
		}
		for _, removed := range []string{"core", "detach", "revision", "base-revision", "artifact", "monitoring"} {
			if command.Flags().Lookup(removed) != nil {
				t.Errorf("config %s still exposes --%s", name, removed)
			}
		}
	}
}

func TestUnavailableExitClass(t *testing.T) {
	_, _, err := execute(t, "core", "list")
	if ExitCode(err) != 6 {
		t.Fatalf("ExitCode() = %d, error = %v", ExitCode(err), err)
	}
}
