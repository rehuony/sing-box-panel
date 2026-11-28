// SPDX-License-Identifier: GPL-3.0-or-later

package cliui

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/rehuony/sing-box-panel/internal/systemd"
)

// ErrorKind selects a failure's exit code.
type ErrorKind int

const (
	ErrorDomain ErrorKind = iota + 1
	ErrorUsage
	ErrorValidation
	ErrorConflict
	ErrorPermission
	ErrorUnavailable
)

// Error is a classified command failure: Code names it in output and Kind
// selects the exit code.
type Error struct {
	Kind    ErrorKind
	Code    string
	Message string
	Cause   error
}

func (err *Error) Error() string {
	if err.Message != "" {
		return err.Message
	}
	if err.Cause != nil {
		return err.Cause.Error()
	}
	return err.Code
}

func (err *Error) Unwrap() error { return err.Cause }

// Fail classifies cause as kind with code, keeping its message.
func Fail(kind ErrorKind, code string, cause error) error {
	if cause == nil {
		return nil
	}
	return &Error{Kind: kind, Code: code, Message: cause.Error(), Cause: cause}
}

// ExitCode is the process exit status for a command's outcome. A service
// that stops for systemd to start it again exits with
// systemd.RestartExitStatus.
func ExitCode(err error) int {
	if err == nil {
		return 0
	}
	if errors.Is(err, systemd.ErrRestart) {
		return systemd.RestartExitStatus
	}
	if errors.Is(err, context.Canceled) {
		return 130
	}
	var classified *Error
	if errors.As(err, &classified) {
		switch classified.Kind {
		case ErrorUsage:
			return 2
		case ErrorValidation:
			return 3
		case ErrorConflict:
			return 4
		case ErrorPermission:
			return 5
		case ErrorUnavailable:
			return 6
		}
	}
	if isUsageError(err) {
		return 2
	}
	return 1
}

type errorOutput struct {
	Code     string `json:"code"`
	Message  string `json:"message"`
	ExitCode int    `json:"exit_code"`
}

// WriteError renders one terminal error to writer in the format root's
// --format flag selected. Causes are deliberately not serialized because they
// may contain filesystem or upstream details unsuitable for machine consumers.
func WriteError(writer io.Writer, root *cobra.Command, err error) error {
	// A service that stops to be started again has not failed.
	if err == nil || errors.Is(err, systemd.ErrRestart) {
		return nil
	}
	output := errorOutput{Code: "internal_error", Message: err.Error(), ExitCode: ExitCode(err)}
	var classified *Error
	if errors.As(err, &classified) && classified.Code != "" {
		output.Code = classified.Code
	} else if errors.Is(err, context.Canceled) {
		output.Code = "canceled"
	} else if isUsageError(err) {
		output.Code = "usage_error"
	}
	format := Text
	if root != nil {
		if flag := root.Flag("format"); flag != nil {
			format = Format(flag.Value.String())
		}
	}
	if format == JSON || format == JSONL {
		encoder := json.NewEncoder(writer)
		encoder.SetEscapeHTML(false)
		return encoder.Encode(output)
	}
	style := NewStyle(writer, Text)
	heading := style.Failure("Error [" + output.Code + "]")
	_, writeErr := fmt.Fprintln(writer, "\n"+style.Diagnostic(heading, DiagnosticLines([]string{output.Message})))
	return writeErr
}

func isUsageError(err error) bool {
	if err == nil {
		return false
	}
	message := err.Error()
	for _, prefix := range []string{
		"unknown command ",
		"unknown flag: ",
		"unknown shorthand flag: ",
		"flag needs an argument: ",
		"invalid argument ",
		"requires at least ",
		"requires at most ",
		"requires exactly ",
		"accepts ",
		"required flag(s) ",
	} {
		if strings.HasPrefix(message, prefix) {
			return true
		}
	}
	return false
}
