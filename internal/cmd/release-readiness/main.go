// SPDX-License-Identifier: GPL-3.0-or-later

// Command release-readiness is an internal build tool. It is not included in
// release archives; the production artifact remains sing-box-panel alone.
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/rehuony/sing-box-panel/internal/releasegate"
)

const (
	exitSuccess  = 0
	exitFailure  = 1
	exitUsage    = 2
	exitNotReady = 3
)

type readinessOptions struct {
	releaseVersion    string
	releaseVersionSet bool
	sourceCommit      string
	sourceCommitSet   bool
}

type readinessEvaluator func(readinessOptions) (releasegate.ReadinessStatus, error)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(arguments []string, stdout, stderr io.Writer) int {
	return runWithEvaluator(arguments, stdout, stderr, evaluateReadiness)
}

func runWithEvaluator(
	arguments []string,
	stdout io.Writer,
	stderr io.Writer,
	evaluate readinessEvaluator,
) int {
	options, ok := parseOptions(arguments, stderr)
	if !ok {
		return exitUsage
	}

	status, readinessErr := evaluate(options)
	if err := json.NewEncoder(stdout).Encode(status); err != nil {
		_, _ = fmt.Fprintf(stderr, "encode release readiness status: %v\n", err)
		return exitFailure
	}
	if readinessErr == nil {
		return exitSuccess
	}

	_, _ = fmt.Fprintln(stderr, readinessErr)
	if errors.Is(readinessErr, releasegate.ErrGANotReady) {
		return exitNotReady
	}
	return exitFailure
}

func parseOptions(arguments []string, stderr io.Writer) (readinessOptions, bool) {
	var options readinessOptions
	flags := flag.NewFlagSet("release-readiness", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.Func("release-version", "validate a formal v-prefixed SemVer release", func(value string) error {
		options.releaseVersion = value
		options.releaseVersionSet = true
		return nil
	})
	flags.Func("source-commit", "bind a formal release to its lowercase full source commit", func(value string) error {
		options.sourceCommit = value
		options.sourceCommitSet = true
		return nil
	})
	if err := flags.Parse(arguments); err != nil {
		return readinessOptions{}, false
	}
	if flags.NArg() != 0 {
		_, _ = fmt.Fprintln(stderr, "release-readiness accepts no positional arguments")
		return readinessOptions{}, false
	}
	if err := validateFormalOptions(
		options.releaseVersionSet,
		options.releaseVersion,
		options.sourceCommitSet,
		options.sourceCommit,
	); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return readinessOptions{}, false
	}
	return options, true
}

func evaluateReadiness(options readinessOptions) (releasegate.ReadinessStatus, error) {
	status := releasegate.Readiness()
	if options.releaseVersionSet {
		return status, releasegate.RequireGAForSourceCommit(options.sourceCommit)
	}
	return status, releasegate.RequireGA()
}

func validateFormalOptions(releaseVersionSet bool, releaseVersion string, sourceCommitSet bool, sourceCommit string) error {
	if releaseVersionSet {
		if err := releasegate.ValidateReleaseVersion(releaseVersion); err != nil {
			return err
		}
	}
	if sourceCommitSet {
		if err := releasegate.ValidateSourceCommit(sourceCommit); err != nil {
			return err
		}
	}
	if releaseVersionSet != sourceCommitSet {
		return errors.New("--release-version and --source-commit must be provided together for a formal release")
	}
	return nil
}
