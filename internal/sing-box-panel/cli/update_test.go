// SPDX-License-Identifier: GPL-3.0-or-later

package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"strings"
	"testing"

	"github.com/rehuony/sing-box-panel/internal/buildinfo"
	"github.com/rehuony/sing-box-panel/internal/selfupdate"
)

func TestUpdateCommandPreservesStructuredOutput(t *testing.T) {
	t.Parallel()

	want := selfupdate.Result{
		PreviousVersion: "v1.2.3", Version: "v1.3.0", Updated: true,
		ExecutablePath: "/usr/local/bin/sing-box-panel",
	}
	for _, format := range []string{"json", "jsonl"} {
		t.Run(format, func(t *testing.T) {
			stdout, stderr, err := executeUpdateCommand(t, func(_ context.Context, current string, report selfupdate.ProgressFunc) (selfupdate.Result, error) {
				if current != "v1.2.3" || report != nil {
					t.Fatalf("version=%q progress enabled=%t", current, report != nil)
				}
				return want, nil
			}, "--output="+format, "update")
			if err != nil || stderr != "" {
				t.Fatalf("error=%v stderr=%q", err, stderr)
			}
			var result selfupdate.Result
			if err := json.Unmarshal([]byte(stdout), &result); err != nil || result != want {
				t.Fatalf("result=%+v error=%v stdout=%q", result, err, stdout)
			}
		})
	}
}

func TestUpdateCommandReportsAlreadyCurrent(t *testing.T) {
	t.Parallel()

	stdout, stderr, err := executeUpdateCommand(t, func(_ context.Context, _ string, report selfupdate.ProgressFunc) (selfupdate.Result, error) {
		report(selfupdate.Progress{Stage: selfupdate.StageCheckRelease})
		return selfupdate.Result{PreviousVersion: "v1.2.3", Version: "v1.2.3"}, nil
	}, "update")
	if err != nil || stderr != "Checking for updates...\n" || !strings.Contains(stdout, "already up to date") {
		t.Fatalf("stdout=%q stderr=%q error=%v", stdout, stderr, err)
	}
}

func TestUpdateCommandReportsProgressOnStderr(t *testing.T) {
	t.Parallel()
	stdout, stderr, err := executeUpdateCommand(t, func(_ context.Context, _ string, report selfupdate.ProgressFunc) (selfupdate.Result, error) {
		for _, event := range []selfupdate.Progress{
			{Stage: selfupdate.StageCheckRelease},
			{Stage: selfupdate.StageVerifyRelease, Version: "v1.3.0"},
			{Stage: selfupdate.StagePrepare},
			{Stage: selfupdate.StageDownload, Total: 2 << 20},
			{Stage: selfupdate.StageDownload, Downloaded: 1 << 20, Total: 2 << 20},
			{Stage: selfupdate.StageDownload, Downloaded: 2 << 20, Total: 2 << 20, Complete: true},
			{Stage: selfupdate.StageVerifyBinary},
			{Stage: selfupdate.StageInstall},
		} {
			report(event)
		}
		return selfupdate.Result{Updated: true, PreviousVersion: "v1.2.3", Version: "v1.3.0", ExecutablePath: "/bin/panel"}, nil
	}, "update")
	if err != nil || stdout != "updated sing-box-panel from v1.2.3 to v1.3.0 at /bin/panel\n" {
		t.Fatalf("stdout=%q error=%v", stdout, err)
	}
	last := -1
	for _, text := range []string{"Checking for updates", "New release v1.3.0", "Preparing update", "  0%", " 50%", "100%  2.0 / 2.0 MiB", "Verifying downloaded binary", "Installing update"} {
		index := strings.Index(stderr, text)
		if index <= last {
			t.Fatalf("missing or out-of-order %q: %q", text, stderr)
		}
		last = index
	}
	if strings.ContainsAny(stderr, "\r\x1b") {
		t.Fatalf("redirected output contains terminal controls: %q", stderr)
	}
}

func TestUpdateCommandDownloadFailureDoesNotPrintSuccess(t *testing.T) {
	t.Parallel()
	for _, failure := range []error{context.Canceled, selfupdate.ErrReleaseUnavailable, selfupdate.ErrChecksumInvalid} {
		stdout, stderr, err := executeUpdateCommand(t, func(_ context.Context, _ string, report selfupdate.ProgressFunc) (selfupdate.Result, error) {
			report(selfupdate.Progress{Stage: selfupdate.StageDownload, Downloaded: 1 << 20, Total: 2 << 20})
			return selfupdate.Result{}, failure
		}, "update")
		if !errors.Is(err, failure) || stdout != "" || !strings.Contains(stderr, "50%") || strings.Contains(stderr, "100%") || !strings.HasSuffix(stderr, "\n") {
			t.Fatalf("stdout=%q stderr=%q error=%v", stdout, stderr, err)
		}
	}
}

func TestUpdateCommandClassifiesFailures(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error
		code int
	}{
		{name: "permission", err: fs.ErrPermission, code: 5},
		{name: "platform", err: selfupdate.ErrUnsupportedPlatform, code: 6},
		{name: "development build", err: selfupdate.ErrInvalidVersion, code: 6},
		{name: "release unavailable", err: selfupdate.ErrReleaseUnavailable, code: 6},
		{name: "missing asset", err: selfupdate.ErrAssetMissing, code: 6},
		{name: "verification key", err: selfupdate.ErrVerificationKeyInvalid, code: 3},
		{name: "release invalid", err: selfupdate.ErrReleaseInvalid, code: 3},
		{name: "signature", err: selfupdate.ErrSignatureInvalid, code: 3},
		{name: "checksum", err: selfupdate.ErrChecksumInvalid, code: 3},
		{name: "executable", err: selfupdate.ErrExecutableInvalid, code: 3},
		{name: "executable changed", err: selfupdate.ErrExecutableChanged, code: 3},
		{name: "staged executable", err: selfupdate.ErrStagedExecutableInvalid, code: 3},
		{name: "canceled", err: context.Canceled, code: 130},
		{name: "other", err: errors.New("replace failed"), code: 1},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			_, _, err := executeUpdateCommand(t, func(context.Context, string, selfupdate.ProgressFunc) (selfupdate.Result, error) {
				return selfupdate.Result{}, testCase.err
			}, "update")
			if got := ExitCode(err); got != testCase.code {
				t.Fatalf("exit code = %d, error = %v", got, err)
			}
		})
	}
}

func executeUpdateCommand(
	t *testing.T,
	update func(context.Context, string, selfupdate.ProgressFunc) (selfupdate.Result, error),
	args ...string,
) (string, string, error) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	command := NewRootCommand(Dependencies{
		Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr,
		Build: buildinfo.Info{Version: "v1.2.3"}, Update: update,
	})
	command.SetArgs(args)
	err := command.ExecuteContext(context.Background())
	return stdout.String(), stderr.String(), err
}
