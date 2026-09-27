// SPDX-License-Identifier: GPL-3.0-or-later

package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/rehuony/sing-box-panel/internal/installation"
)

// Keep joined errors readable without repeating causes already reported by an
// earlier layer. Machine output retains the original message and result fields.
func diagnosticLines(messages []string) []string {
	var lines []string
	seen := make(map[string]bool)
	for _, message := range messages {
		for _, line := range strings.Split(message, "\n") {
			line = strings.TrimSpace(line)
			if line != "" && !seen[line] {
				seen[line] = true
				lines = append(lines, line)
			}
		}
	}
	return lines
}

func diagnosticText(heading string, lines []string, style fileTreeStyle) string {
	var output strings.Builder
	output.WriteString("\n" + heading + "\n")
	for _, line := range lines {
		prefix := "  - "
		for {
			runes := []rune(line)
			end := len(runes)
			if end > 84 {
				for index := 84; index > 0; index-- {
					if runes[index] == ' ' {
						end = index
						break
					}
				}
			}
			part := string(runes[:end])
			for _, label := range []string{"Inspect:", "Next:", "Expected:", "Unit file:"} {
				if strings.HasPrefix(part, label) {
					part = style.paint("1;36", label) + strings.TrimPrefix(part, label)
					break
				}
			}
			output.WriteString(prefix + part + "\n")
			if end == len(runes) {
				break
			}
			line, prefix = string(runes[end+1:]), "    "
		}
	}
	return strings.TrimSuffix(output.String(), "\n")
}

func writeCleanupWarnings(writer io.Writer, format outputFormat, result installation.CleanupResult, cleanupErr error) error {
	if format != outputText {
		return nil
	}
	excluded := make(map[string]bool)
	if cleanupErr != nil {
		for _, line := range diagnosticLines([]string{cleanupErr.Error()}) {
			excluded[line] = true
		}
	}
	if result.ServiceAccount != nil {
		excluded[accountCleanupSummary(*result.ServiceAccount)] = true
	}
	var warnings []string
	for _, line := range diagnosticLines(result.Warnings) {
		if !excluded[line] {
			warnings = append(warnings, line)
		}
	}
	if len(warnings) == 0 {
		return nil
	}
	style := newFileTreeStyle(writer, format)
	_, err := fmt.Fprintln(writer, diagnosticText(style.paint("1;33", "Warnings"), warnings, style))
	return err
}
