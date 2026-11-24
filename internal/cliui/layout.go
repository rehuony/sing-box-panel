// SPDX-License-Identifier: GPL-3.0-or-later

package cliui

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// Unavailable stands in for a value that is not known.
const Unavailable = "—"

// Field is one labeled value of a result.
type Field struct {
	Label string
	Value string
}

// Fields renders one row per field, values aligned two spaces past the
// longest label. A field without a value shows Unavailable.
func Fields(fields ...Field) string {
	width := 0
	for _, field := range fields {
		width = max(width, utf8.RuneCountInString(field.Label))
	}
	var output strings.Builder
	for i, field := range fields {
		if i > 0 {
			output.WriteByte('\n')
		}
		value := field.Value
		if value == "" {
			value = Unavailable
		}
		output.WriteString(field.Label + strings.Repeat(" ", width-utf8.RuneCountInString(field.Label)+2) + value)
	}
	return output.String()
}

// Section renders a result: the title line, then each non-empty part
// indented by two spaces after a blank line.
func Section(title string, parts ...string) string {
	var output strings.Builder
	output.WriteString(title)
	for _, part := range parts {
		if part == "" {
			continue
		}
		output.WriteString("\n\n")
		output.WriteString(Indent(part))
	}
	return output.String()
}

// Notice separates a section written into a stream, such as the server's
// log, with a blank line before and after it.
func Notice(section string) string { return "\n" + section + "\n" }

// Indent prefixes every non-empty line of text with two spaces.
func Indent(text string) string {
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		if line != "" {
			lines[i] = "  " + line
		}
	}
	return strings.Join(lines, "\n")
}

// Lines joins notes into one part of a section.
func Lines(lines ...string) string {
	kept := lines[:0:0]
	for _, line := range lines {
		if line != "" {
			kept = append(kept, line)
		}
	}
	return strings.Join(kept, "\n")
}

// FormatBytes shows a size in binary units with one decimal.
func FormatBytes(bytes int64) string {
	if bytes < 1024 {
		return fmt.Sprintf("%d B", bytes)
	}
	value := float64(bytes)
	for _, suffix := range []string{"KiB", "MiB", "GiB"} {
		value /= 1024
		if value < 1024 {
			return fmt.Sprintf("%.1f %s", value, suffix)
		}
	}
	return fmt.Sprintf("%.1f TiB", value/1024)
}
