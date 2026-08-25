// SPDX-License-Identifier: GPL-3.0-or-later

package cli

import (
	"bytes"
	"encoding/json"
	"errors"
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
