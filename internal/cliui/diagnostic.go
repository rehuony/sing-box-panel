// SPDX-License-Identifier: GPL-3.0-or-later

package cliui

import "strings"

// diagnosticWidth is where diagnostic lines wrap.
const diagnosticWidth = 84

// DiagnosticLines splits messages into lines and drops blank and repeated
// ones, so joined errors stay readable without repeating causes an earlier
// layer reported. Machine output keeps the original message.
func DiagnosticLines(messages []string) []string {
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

// Diagnostic renders heading and one wrapped bullet per line, highlighting
// the Inspect:, Next:, Expected: and Unit file: labels of recovery guidance.
func (style Style) Diagnostic(heading string, lines []string) string {
	var output strings.Builder
	output.WriteString(heading + "\n")
	for _, line := range lines {
		prefix := "  - "
		for {
			runes := []rune(line)
			end := len(runes)
			if end > diagnosticWidth {
				for index := diagnosticWidth; index > 0; index-- {
					if runes[index] == ' ' {
						end = index
						break
					}
				}
			}
			part := string(runes[:end])
			for _, label := range []string{"Inspect:", "Next:", "Expected:", "Unit file:"} {
				if strings.HasPrefix(part, label) {
					part = style.paint(codeHint, label) + strings.TrimPrefix(part, label)
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
