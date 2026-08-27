// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/rehuony/sing-box-panel/internal/releasegate"
)

func TestRunWithEvaluator(t *testing.T) {
	t.Parallel()

	commit := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	tests := []struct {
		name           string
		arguments      []string
		status         releasegate.ReadinessStatus
		readinessErr   error
		wantExit       int
		wantStderr     string
		wantEvaluation bool
		wantFormal     bool
	}{
		{
			name: "ready diagnostic",
			status: releasegate.ReadinessStatus{
				Ready: true,
			},
			wantExit:       exitSuccess,
			wantEvaluation: true,
		},
		{
			name: "not ready",
			status: releasegate.ReadinessStatus{
				Ready: false,
			},
			readinessErr:   fmt.Errorf("%w: fixture gate", releasegate.ErrGANotReady),
			wantExit:       exitNotReady,
			wantStderr:     "fixture gate",
			wantEvaluation: true,
		},
		{
			name:           "operational failure",
			status:         releasegate.ReadinessStatus{},
			readinessErr:   errors.New("fixture failure"),
			wantExit:       exitFailure,
			wantStderr:     "fixture failure",
			wantEvaluation: true,
		},
		{
			name: "formal",
			arguments: []string{
				"--release-version", "v1.2.3",
				"--source-commit", commit,
			},
			status: releasegate.ReadinessStatus{
				Ready: true,
			},
			wantExit:       exitSuccess,
			wantEvaluation: true,
			wantFormal:     true,
		},
		{
			name:       "positional argument",
			arguments:  []string{"unexpected"},
			wantExit:   exitUsage,
			wantStderr: "accepts no positional arguments",
		},
		{
			name:       "unpaired formal option",
			arguments:  []string{"--release-version", "v1.2.3"},
			wantExit:   exitUsage,
			wantStderr: "must be provided together",
		},
		{
			name:       "removed ready output option",
			arguments:  []string{"--ready-output", "ready"},
			wantExit:   exitUsage,
			wantStderr: "flag provided but not defined",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			var stdout bytes.Buffer
			var stderr bytes.Buffer
			evaluated := false
			evaluator := func(options readinessOptions) (releasegate.ReadinessStatus, error) {
				evaluated = true
				if options.releaseVersionSet != test.wantFormal || options.sourceCommitSet != test.wantFormal {
					t.Fatalf("formal option presence = %t/%t, want %t", options.releaseVersionSet, options.sourceCommitSet, test.wantFormal)
				}
				if test.wantFormal && (options.releaseVersion != "v1.2.3" || options.sourceCommit != commit) {
					t.Fatalf("formal options = %q/%q", options.releaseVersion, options.sourceCommit)
				}
				return test.status, test.readinessErr
			}

			exitCode := runWithEvaluator(test.arguments, &stdout, &stderr, evaluator)
			if exitCode != test.wantExit {
				t.Fatalf("runWithEvaluator() exit = %d, want %d; stdout=%q stderr=%q", exitCode, test.wantExit, stdout.String(), stderr.String())
			}
			if evaluated != test.wantEvaluation {
				t.Fatalf("evaluator called = %t, want %t", evaluated, test.wantEvaluation)
			}
			if test.wantStderr == "" && stderr.Len() != 0 {
				t.Fatalf("stderr = %q, want empty", stderr.String())
			}
			if test.wantStderr != "" && !strings.Contains(stderr.String(), test.wantStderr) {
				t.Fatalf("stderr = %q, want substring %q", stderr.String(), test.wantStderr)
			}
			if !test.wantEvaluation {
				if stdout.Len() != 0 {
					t.Fatalf("stdout = %q, want empty", stdout.String())
				}
				return
			}
			var decoded releasegate.ReadinessStatus
			if err := json.Unmarshal(stdout.Bytes(), &decoded); err != nil {
				t.Fatalf("stdout is not readiness JSON: %v; output=%q", err, stdout.String())
			}
			if decoded.Ready != test.status.Ready {
				t.Fatalf("decoded ready = %t, want %t", decoded.Ready, test.status.Ready)
			}
		})
	}
}

func TestRunReportsJSONEncodingFailure(t *testing.T) {
	t.Parallel()
	var stderr bytes.Buffer
	exitCode := runWithEvaluator(nil, errorWriter{}, &stderr, func(readinessOptions) (releasegate.ReadinessStatus, error) {
		return releasegate.ReadinessStatus{Ready: true}, nil
	})
	if exitCode != exitFailure {
		t.Fatalf("runWithEvaluator() exit = %d, want %d", exitCode, exitFailure)
	}
	if !strings.Contains(stderr.String(), "encode release readiness status") {
		t.Fatalf("stderr = %q, want encoding diagnostic", stderr.String())
	}
}

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

type errorWriter struct{}

func (errorWriter) Write([]byte) (int, error) {
	return 0, io.ErrClosedPipe
}
