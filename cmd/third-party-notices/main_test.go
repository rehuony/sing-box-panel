// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"bytes"
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunWithDependencies(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		arguments   []string
		current     []byte
		generateErr error
		wantExit    int
		wantStdout  string
		wantStderr  string
		wantWrite   bool
	}{
		{name: "unknown flag", arguments: []string{"--unknown"}, wantExit: 2, wantStderr: "flag provided but not defined"},
		{name: "positional argument", arguments: []string{"unexpected"}, wantExit: 2, wantStderr: "unexpected positional arguments"},
		{name: "generation failure", generateErr: errors.New("fixture failure"), wantExit: 1, wantStderr: "generate third-party notices: fixture failure"},
		{name: "current", arguments: []string{"--check"}, current: []byte("generated"), wantStdout: "third-party notices are current:"},
		{name: "stale", arguments: []string{"--check"}, current: []byte("old"), wantExit: 1, wantStderr: "is stale; run make notices"},
		{name: "generate", arguments: []string{"--output", "custom-notices"}, wantStdout: "generated ", wantWrite: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			var stdout bytes.Buffer
			var stderr bytes.Buffer
			wrote := false
			dependencies := commandDependencies{
				findRoot: func() (string, error) { return "/repository", nil },
				generate: func(context.Context, string) ([]byte, error) {
					return []byte("generated"), test.generateErr
				},
				readFile: func(string) ([]byte, error) { return test.current, nil },
				write: func(path string, content []byte) error {
					wrote = true
					if path != filepath.Join("/repository", "custom-notices") || !bytes.Equal(content, []byte("generated")) {
						t.Fatalf("write(%q, %q)", path, content)
					}
					return nil
				},
			}

			gotExit := runWithDependencies(context.Background(), test.arguments, &stdout, &stderr, dependencies)
			if gotExit != test.wantExit {
				t.Fatalf("exit = %d, want %d; stdout=%q stderr=%q", gotExit, test.wantExit, stdout.String(), stderr.String())
			}
			if test.wantStdout != "" && !strings.Contains(stdout.String(), test.wantStdout) {
				t.Fatalf("stdout = %q, want substring %q", stdout.String(), test.wantStdout)
			}
			if test.wantStderr != "" && !strings.Contains(stderr.String(), test.wantStderr) {
				t.Fatalf("stderr = %q, want substring %q", stderr.String(), test.wantStderr)
			}
			if (test.wantStdout == "") != (stdout.Len() == 0) {
				t.Fatalf("stdout = %q", stdout.String())
			}
			if (test.wantStderr == "") != (stderr.Len() == 0) {
				t.Fatalf("stderr = %q", stderr.String())
			}
			if wrote != test.wantWrite {
				t.Fatalf("write called = %t, want %t", wrote, test.wantWrite)
			}
		})
	}
}
