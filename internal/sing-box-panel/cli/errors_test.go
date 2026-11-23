// SPDX-License-Identifier: GPL-3.0-or-later

package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"regexp"
	"strings"
	"testing"
)

func TestExitCodeClassifiesCobraUsage(t *testing.T) {
	if got := ExitCode(errors.New(`unknown command "wrong" for "sing-box-panel"`)); got != 2 {
		t.Fatalf("ExitCode() = %d, want 2", got)
	}
}

func TestWriteErrorUsesMachineOutput(t *testing.T) {
	root := NewRootCommand(Dependencies{Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{}})
	if err := root.PersistentFlags().Set("output", "json"); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	err := &Error{Kind: ErrorConflict, Code: "revision_conflict", Message: "revision changed"}
	if writeErr := WriteError(&output, root, err); writeErr != nil {
		t.Fatal(writeErr)
	}
	var value map[string]any
	if decodeErr := json.Unmarshal(output.Bytes(), &value); decodeErr != nil {
		t.Fatalf("decode output: %v; output=%s", decodeErr, output.String())
	}
	if value["code"] != "revision_conflict" || value["exit_code"] != float64(4) {
		t.Fatalf("WriteError() = %#v", value)
	}
}

func TestTextDiagnosticsPreserveAndDeduplicateMessages(t *testing.T) {
	message := "systemd reports NeedDaemonReload=yes"
	message += "\nInspect: systemctl cat sing-box-panel.service"
	message += "\nNext: after reviewing the unit, run `systemctl daemon-reload` and retry"
	err := &Error{Kind: ErrorConflict, Code: "instance_cleanup_failed", Message: message + "\n" + message}
	var output bytes.Buffer
	if writeErr := WriteError(&output, nil, err); writeErr != nil {
		t.Fatal(writeErr)
	}
	if strings.Count(output.String(), "NeedDaemonReload=yes") != 1 ||
		!strings.HasPrefix(output.String(), "\nError [instance_cleanup_failed]\n  - ") ||
		!strings.Contains(output.String(), "\n  - Next:") || strings.Contains(output.String(), "\x1b") {
		t.Fatalf("unreadable or repeated error: %q", output.String())
	}
	plain := diagnosticText("Error", diagnosticLines([]string{message}), fileTreeStyle{})
	colored := diagnosticText("\x1b[1;31mError\x1b[0m", diagnosticLines([]string{message}), fileTreeStyle{color: true})
	if regexp.MustCompile(`\x1b\[[0-9;]*m`).ReplaceAllString(colored, "") != plain {
		t.Fatal("styling changed diagnostic content")
	}
	long := "An operation failed while inspecting a service configuration with a long description and multiple details that must stay legible on a normal terminal."
	wrapped := diagnosticText("Error", []string{long}, fileTreeStyle{})
	if !strings.Contains(wrapped, "\n    ") || !strings.Contains(wrapped, "normal terminal.") {
		t.Fatalf("lost continuation indentation or content: %q", wrapped)
	}
}
