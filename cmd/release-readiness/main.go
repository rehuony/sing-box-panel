// SPDX-License-Identifier: GPL-3.0-or-later

// Command release-readiness is an internal build tool. It is not included in
// release archives; the production artifact remains sing-box-panel alone.
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"strconv"

	"github.com/rehuony/sing-box-panel/internal/releasegate"
)

func main() {
	var releaseVersion string
	var releaseVersionSet bool
	var sourceCommit string
	var sourceCommitSet bool
	var readyOutput string
	flag.Func("release-version", "validate a formal v-prefixed SemVer release", func(value string) error {
		releaseVersion = value
		releaseVersionSet = true
		return nil
	})
	flag.Func("source-commit", "bind a formal release to its lowercase full source commit", func(value string) error {
		sourceCommit = value
		sourceCommitSet = true
		return nil
	})
	flag.StringVar(&readyOutput, "ready-output", "", "write the readiness boolean to a file")
	flag.Parse()
	if flag.NArg() != 0 {
		_, _ = fmt.Fprintln(os.Stderr, "release-readiness accepts no positional arguments")
		os.Exit(2)
	}
	if err := validateFormalOptions(releaseVersionSet, releaseVersion, sourceCommitSet, sourceCommit); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	status := releasegate.Readiness()
	if err := writeReadyOutput(readyOutput, status.Ready); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	_ = json.NewEncoder(os.Stdout).Encode(status)
	var err error
	if releaseVersionSet {
		err = releasegate.RequireGAForSourceCommit(sourceCommit)
	} else {
		err = releasegate.RequireGA()
	}
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func writeReadyOutput(path string, ready bool) error {
	if path == "" {
		return nil
	}
	content := []byte(strconv.FormatBool(ready) + "\n")
	if err := os.WriteFile(path, content, 0o600); err != nil {
		return fmt.Errorf("write readiness output: %w", err)
	}
	return nil
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
