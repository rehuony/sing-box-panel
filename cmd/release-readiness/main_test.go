// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestValidateFormalOptions(t *testing.T) {
	t.Parallel()
	commit40 := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	commit64 := "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	tests := []struct {
		name              string
		releaseVersionSet bool
		releaseVersion    string
		sourceCommitSet   bool
		sourceCommit      string
		valid             bool
	}{
		{name: "diagnostic", valid: true},
		{
			name: "formal SHA-1", releaseVersionSet: true, releaseVersion: "v1.2.3",
			sourceCommitSet: true, sourceCommit: commit40, valid: true,
		},
		{
			name: "formal SHA-256", releaseVersionSet: true, releaseVersion: "v1.2.3",
			sourceCommitSet: true, sourceCommit: commit64, valid: true,
		},
		{name: "release only", releaseVersionSet: true, releaseVersion: "v1.2.3"},
		{name: "commit only", sourceCommitSet: true, sourceCommit: commit40},
		{
			name: "empty release", releaseVersionSet: true,
			sourceCommitSet: true, sourceCommit: commit40,
		},
		{
			name: "empty commit", releaseVersionSet: true, releaseVersion: "v1.2.3",
			sourceCommitSet: true,
		},
		{
			name: "development mode", releaseVersionSet: true, releaseVersion: "dev",
			sourceCommitSet: true, sourceCommit: commit40,
		},
		{
			name: "CI mode", releaseVersionSet: true, releaseVersion: "ci",
			sourceCommitSet: true, sourceCommit: commit40,
		},
		{
			name: "non SemVer release", releaseVersionSet: true, releaseVersion: "1.2.3",
			sourceCommitSet: true, sourceCommit: commit40,
		},
		{
			name: "invalid commit", releaseVersionSet: true, releaseVersion: "v1.2.3",
			sourceCommitSet: true, sourceCommit: "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			err := validateFormalOptions(
				test.releaseVersionSet,
				test.releaseVersion,
				test.sourceCommitSet,
				test.sourceCommit,
			)
			if test.valid && err != nil {
				t.Fatalf("validateFormalOptions(): %v", err)
			}
			if !test.valid && err == nil {
				t.Fatal("validateFormalOptions() succeeded, want error")
			}
		})
	}
}

func TestWriteReadyOutput(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		ready bool
		want  string
	}{
		{name: "false", want: "false\n"},
		{name: "true", ready: true, want: "true\n"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			path := filepath.Join(t.TempDir(), "ready")
			if err := writeReadyOutput(path, test.ready); err != nil {
				t.Fatalf("writeReadyOutput(): %v", err)
			}
			content, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read readiness output: %v", err)
			}
			if string(content) != test.want {
				t.Fatalf("readiness output = %q, want %q", content, test.want)
			}
		})
	}
}
