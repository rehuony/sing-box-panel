// SPDX-License-Identifier: GPL-3.0-or-later

// Package cliui is what both binaries' command lines share: the output
// formats, classified errors, colors, aligned fields, file trees,
// diagnostics, update progress, foreground server output and help layout,
// and the version, completion, server stop|status and systemd commands.
package cliui

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/pflag"
)

// Format selects human or machine-readable output.
type Format string

const (
	Text  Format = "text"
	JSON  Format = "json"
	JSONL Format = "jsonl"
)

// AddFormatFlag registers -f/--format, which selects target.
func AddFormatFlag(flags *pflag.FlagSet, target *Format) {
	*target = Text
	flags.VarP(&formatValue{target: target}, "format", "f", "output format: text, json, or jsonl")
}

type formatValue struct{ target *Format }

func (value *formatValue) String() string {
	if value == nil || value.target == nil {
		return string(Text)
	}
	return string(*value.target)
}

func (value *formatValue) Set(raw string) error {
	format := Format(strings.ToLower(strings.TrimSpace(raw)))
	switch format {
	case Text, JSON, JSONL:
		*value.target = format
		return nil
	default:
		return &Error{Kind: ErrorUsage, Code: "invalid_format", Message: "format must be text, json, or jsonl"}
	}
}

func (value *formatValue) Type() string { return "format" }

// Write prints text, or value as one JSON document in a structured format.
func Write(writer io.Writer, format Format, value any, text string) error {
	switch format {
	case Text:
		_, err := fmt.Fprintln(writer, text)
		return err
	case JSON, JSONL:
		encoder := json.NewEncoder(writer)
		encoder.SetEscapeHTML(false)
		return encoder.Encode(value)
	default:
		return &Error{Kind: ErrorUsage, Code: "invalid_format", Message: "format must be text, json, or jsonl"}
	}
}
