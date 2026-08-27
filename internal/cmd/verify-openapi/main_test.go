// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRun(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		arguments  []string
		document   string
		wantStatus int
		wantOutput string
	}{
		{name: "usage", wantStatus: 2, wantOutput: "usage:"},
		{
			name:       "valid",
			document:   "openapi: 3.1.0\npaths: {}\n",
			wantStatus: 0,
			wantOutput: "OpenAPI validation passed:",
		},
		{
			name:       "invalid",
			document:   "openapi: 3.1.0\npaths: []\n",
			wantStatus: 1,
			wantOutput: "must be a mapping",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			arguments := test.arguments
			if test.document != "" {
				path := filepath.Join(t.TempDir(), "openapi.yaml")
				if err := os.WriteFile(path, []byte(test.document), 0o644); err != nil {
					t.Fatalf("write OpenAPI fixture: %v", err)
				}
				arguments = []string{path}
			}
			var stdout bytes.Buffer
			var stderr bytes.Buffer
			status := run(arguments, &stdout, &stderr)
			if status != test.wantStatus {
				t.Fatalf("run() status = %d, want %d; stdout=%q stderr=%q", status, test.wantStatus, stdout.String(), stderr.String())
			}
			if combined := stdout.String() + stderr.String(); !strings.Contains(combined, test.wantOutput) {
				t.Fatalf("run() output = %q, want substring %q", combined, test.wantOutput)
			}
		})
	}
}
