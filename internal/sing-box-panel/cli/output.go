// SPDX-License-Identifier: GPL-3.0-or-later

package cli

import (
	"encoding/json"
	"fmt"
	"io"
)

type outputFormat string

const (
	outputText  outputFormat = "text"
	outputJSON  outputFormat = "json"
	outputJSONL outputFormat = "jsonl"
)

func writeResult(writer io.Writer, format outputFormat, value any, text string) error {
	switch format {
	case outputText:
		_, err := fmt.Fprintln(writer, text)
		return err
	case outputJSON, outputJSONL:
		encoder := json.NewEncoder(writer)
		encoder.SetEscapeHTML(false)
		return encoder.Encode(value)
	default:
		return &Error{Kind: ErrorUsage, Code: "invalid_output", Message: "output must be text, json, or jsonl"}
	}
}
