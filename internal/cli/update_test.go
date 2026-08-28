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

	var receivedVersion string
	want := selfupdate.Result{
		PreviousVersion: "v1.2.3", Version: "v1.3.0", Updated: true,
		ExecutablePath: "/usr/local/bin/sing-box-panel",
	}
	stdout, stderr, err := executeUpdateCommand(t, func(_ context.Context, current string) (selfupdate.Result, error) {
		receivedVersion = current
		return want, nil
	}, "--output=json", "update")
	if err != nil {
		t.Fatal(err)
	}
	if stderr != "" {
		t.Fatalf("stderr = %q", stderr)
	}
	var result selfupdate.Result
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("decode stdout: %v; %q", err, stdout)
	}
	if result != want || receivedVersion != "v1.2.3" {
		t.Fatalf("result=%+v version=%q", result, receivedVersion)
	}
}

func TestUpdateCommandReportsAlreadyCurrent(t *testing.T) {
	t.Parallel()

	stdout, stderr, err := executeUpdateCommand(t, func(context.Context, string) (selfupdate.Result, error) {
		return selfupdate.Result{PreviousVersion: "v1.2.3", Version: "v1.2.3"}, nil
	}, "update")
	if err != nil || stderr != "" || !strings.Contains(stdout, "already up to date") {
		t.Fatalf("stdout=%q stderr=%q error=%v", stdout, stderr, err)
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
			_, _, err := executeUpdateCommand(t, func(context.Context, string) (selfupdate.Result, error) {
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
	update func(context.Context, string) (selfupdate.Result, error),
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
