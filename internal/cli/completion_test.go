// SPDX-License-Identifier: GPL-3.0-or-later

package cli

import (
	"bytes"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func completionTestRoot() *cobra.Command {
	return NewRootCommand(Dependencies{
		Stdin:  strings.NewReader(""),
		Stdout: io.Discard,
		Stderr: io.Discard,
	})
}

func TestCompletionCommandSupportsOnlyLinuxShells(t *testing.T) {
	root := completionTestRoot()
	completion, _, err := root.Find([]string{"completion"})
	if err != nil {
		t.Fatal(err)
	}

	want := map[string]bool{"bash": false, "zsh": false, "fish": false}
	commands := completion.Commands()
	if len(commands) != len(want) {
		t.Fatalf("completion commands = %d, want %d", len(commands), len(want))
	}
	for _, command := range commands {
		if _, ok := want[command.Name()]; !ok {
			t.Errorf("unexpected completion command %q", command.Name())
			continue
		}
		want[command.Name()] = true
	}
	for name, found := range want {
		if !found {
			t.Errorf("missing completion command %q", name)
		}
	}
}

func TestPublicCommandsHaveShortDescriptions(t *testing.T) {
	var walk func(*cobra.Command)
	walk = func(command *cobra.Command) {
		if !command.Hidden && strings.TrimSpace(command.Short) == "" {
			t.Errorf("public command %q has no Short description", command.CommandPath())
		}
		for _, child := range command.Commands() {
			walk(child)
		}
	}
	walk(completionTestRoot())
}

func TestBashCompletionIncludesDescriptions(t *testing.T) {
	stdout, stderr, err := execute(t, "completion", "bash")
	if err != nil {
		t.Fatal(err)
	}
	if stderr != "" {
		t.Fatalf("stderr = %q", stderr)
	}
	if stdout == "" {
		t.Fatal("stdout is empty")
	}
	if strings.Contains(stdout, cobra.ShellCompNoDescRequestCmd) {
		t.Fatalf("completion script requests %q", cobra.ShellCompNoDescRequestCmd)
	}
	if !strings.Contains(stdout, " "+cobra.ShellCompRequestCmd+" ") {
		t.Fatalf("completion script does not request %q", cobra.ShellCompRequestCmd)
	}
}

func TestCompletionCandidatesIncludeDescriptions(t *testing.T) {
	t.Setenv("SING_BOX_PANEL_COMPLETION_DESCRIPTIONS", "true")
	root := completionTestRoot()
	completion, _, err := root.Find([]string{"completion"})
	if err != nil {
		t.Fatal(err)
	}

	stdout, _, err := execute(t, cobra.ShellCompRequestCmd, "completion", "")
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSuffix(stdout, "\n"), "\n")
	wantDirective := fmt.Sprintf(":%d", cobra.ShellCompDirectiveNoFileComp)
	if lines[len(lines)-1] != wantDirective {
		t.Fatalf("completion directive = %q, want %q", lines[len(lines)-1], wantDirective)
	}
	want := make(map[string]string, len(completion.Commands()))
	for _, command := range completion.Commands() {
		want[command.Name()] = command.Short
	}
	got := make(map[string]string, len(want))
	for _, line := range lines[:len(lines)-1] {
		name, description, ok := strings.Cut(line, "\t")
		if !ok {
			t.Fatalf("completion candidate has no description: %q", line)
		}
		got[name] = description
	}
	if len(got) != len(want) {
		t.Fatalf("completion candidates = %#v, want %#v", got, want)
	}
	for name, description := range want {
		if got[name] != description {
			t.Errorf("completion candidate %q description = %q, want %q", name, got[name], description)
		}
	}
}

func TestBashCompletionScriptSyntax(t *testing.T) {
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash is not installed")
	}
	stdout, stderr, err := execute(t, "completion", "bash")
	if err != nil {
		t.Fatal(err)
	}
	if stderr != "" {
		t.Fatalf("stderr = %q", stderr)
	}

	command := exec.CommandContext(t.Context(), bash, "-n")
	command.Stdin = strings.NewReader(stdout)
	var output bytes.Buffer
	command.Stdout = &output
	command.Stderr = &output
	if err := command.Run(); err != nil {
		t.Fatalf("bash -n: %v; output = %q", err, output.String())
	}
}
