// SPDX-License-Identifier: GPL-3.0-or-later

package cliui

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/rehuony/sing-box-panel/internal/systemd"
)

func TestExitCodeClassifiesCobraUsage(t *testing.T) {
	if got := ExitCode(errors.New(`unknown command "wrong" for "sing-box-panel"`)); got != 2 {
		t.Fatalf("ExitCode() = %d, want 2", got)
	}
}

// A service that stops for systemd to start it again, such as to install a
// staged update, exits with the status its system unit restarts on, silently.
func TestServiceRestartExitsWithTheUnitsRestartStatus(t *testing.T) {
	err := fmt.Errorf("serve: %w", systemd.ErrRestart)
	if got := ExitCode(err); got != systemd.RestartExitStatus {
		t.Fatalf("ExitCode() = %d, want %d", got, systemd.RestartExitStatus)
	}
	var output bytes.Buffer
	if writeErr := WriteError(&output, &cobra.Command{Use: "test"}, err); writeErr != nil || output.Len() != 0 {
		t.Fatalf("WriteError() wrote %q, %v; want nothing", output.String(), writeErr)
	}
}

func TestWriteErrorUsesMachineOutput(t *testing.T) {
	root := &cobra.Command{Use: "test"}
	var format Format
	AddFormatFlag(root.PersistentFlags(), &format)
	if err := root.PersistentFlags().Set("format", "json"); err != nil {
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
	plain := (Style{}).Diagnostic("Error", DiagnosticLines([]string{message}))
	colored := (Style{Color: true}).Diagnostic("\x1b[1;31mError\x1b[0m", DiagnosticLines([]string{message}))
	if regexp.MustCompile(`\x1b\[[0-9;]*m`).ReplaceAllString(colored, "") != plain {
		t.Fatal("styling changed diagnostic content")
	}
	long := "An operation failed while inspecting a service configuration with a long description and multiple details that must stay legible on a normal terminal."
	wrapped := (Style{}).Diagnostic("Error", []string{long})
	if !strings.Contains(wrapped, "\n    ") || !strings.Contains(wrapped, "normal terminal.") {
		t.Fatalf("lost continuation indentation or content: %q", wrapped)
	}
}
