// SPDX-License-Identifier: GPL-3.0-or-later

package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"
)

type ErrorKind int

const (
	ErrorDomain ErrorKind = iota + 1
	ErrorUsage
	ErrorValidation
	ErrorConflict
	ErrorPermission
	ErrorUnavailable
)

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

func ExitCode(err error) int {
	if err == nil {
		return 0
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
	if isCobraUsageError(err) {
		return 2
	}
	return 1
}

type errorOutput struct {
	Code     string `json:"code"`
	Message  string `json:"message"`
	ExitCode int    `json:"exit_code"`
}

// WriteError renders one terminal error to stderr according to the parsed
// root output flag. Causes are deliberately not serialized because they may
// contain filesystem or upstream details unsuitable for machine consumers.
func WriteError(writer io.Writer, root *cobra.Command, err error) error {
	if err == nil {
		return nil
	}
	output := errorOutput{Code: "internal_error", Message: err.Error(), ExitCode: ExitCode(err)}
	var classified *Error
	if errors.As(err, &classified) && classified.Code != "" {
		output.Code = classified.Code
	} else if errors.Is(err, context.Canceled) {
		output.Code = "canceled"
	} else if isCobraUsageError(err) {
		output.Code = "usage_error"
	}
	format := outputText
	if root != nil {
		if flag := root.Flag("output"); flag != nil {
			format = outputFormat(flag.Value.String())
		}
	}
	if format == outputJSON || format == outputJSONL {
		encoder := json.NewEncoder(writer)
		encoder.SetEscapeHTML(false)
		return encoder.Encode(output)
	}
	_, writeErr := fmt.Fprintln(writer, output.Message)
	return writeErr
}

func isCobraUsageError(err error) bool {
	if err == nil {
		return false
	}
	message := err.Error()
	for _, prefix := range []string{
		"unknown command ",
		"unknown flag: ",
		"flag needs an argument: ",
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
