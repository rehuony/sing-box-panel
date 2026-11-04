// SPDX-License-Identifier: GPL-3.0-or-later

package systemd

import (
	"os"
	"path/filepath"
	"strings"
)

// unitFileSettingsPath reports the --config argument written in the unit file
// currently on disk. It describes that file only: systemd may still run a
// previously loaded command line, and the process may have started with other
// settings. The second result is false whenever the file does not state one
// unambiguous absolute path: unreadable unit, no or several effective
// [Service] ExecStart lines, unsupported quoting, unresolved specifiers or
// variables, or a relative path. Callers must then report unknown, not guess.
func unitFileSettingsPath(unitPath string) (string, bool) {
	data, err := os.ReadFile(unitPath)
	if err != nil {
		return "", false
	}
	return parseUnitFileSettingsPath(data)
}

func parseUnitFileSettingsPath(unit []byte) (string, bool) {
	execStart, ok := effectiveExecStart(string(unit))
	if !ok {
		return "", false
	}
	words, ok := splitExecLine(execStart)
	if !ok || len(words) < 2 {
		return "", false
	}
	return settingsArgument(words[1:])
}

// settingsArgument applies the server's pflag semantics: the last --config
// value wins, "--" ends flag parsing, and both separated and attached forms
// are accepted. Every occurrence must resolve, so a value that only systemd
// could expand makes the whole result unknown.
func settingsArgument(args []string) (string, bool) {
	var raw string
	found := false
	for index := 0; index < len(args); index++ {
		word := args[index]
		var value string
		switch {
		case word == "--":
			index = len(args)
			continue
		case word == "--config" || word == "-c":
			if index+1 >= len(args) {
				return "", false
			}
			index++
			value = args[index]
		case strings.HasPrefix(word, "--config="):
			value = strings.TrimPrefix(word, "--config=")
		case strings.HasPrefix(word, "-c="):
			value = strings.TrimPrefix(word, "-c=")
		case strings.HasPrefix(word, "-c") && !strings.HasPrefix(word, "--"):
			value = strings.TrimPrefix(word, "-c")
		default:
			continue
		}
		resolved, ok := resolveExecWord(value)
		if !ok || !filepath.IsAbs(resolved) || hasControl(resolved) {
			return "", false
		}
		raw, found = filepath.Clean(resolved), true
	}
	return raw, found
}

// effectiveExecStart returns the single ExecStart command line of the
// [Service] section after applying empty-assignment resets. Comments and
// continuation lines follow systemd's unit syntax. More than one effective
// line (legal for Type=oneshot) is reported as not determinable rather than
// choosing one arbitrarily.
func effectiveExecStart(unit string) (string, bool) {
	var commands []string
	section := ""
	for _, line := range logicalUnitLines(unit) {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
			section = strings.TrimSpace(trimmed[1 : len(trimmed)-1])
			continue
		}
		if section != "Service" {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok || strings.TrimSpace(key) != "ExecStart" {
			continue
		}
		value = strings.TrimSpace(value)
		if value == "" {
			commands = nil
			continue
		}
		commands = append(commands, value)
	}
	if len(commands) != 1 {
		return "", false
	}
	return commands[0], true
}

// logicalUnitLines joins backslash continuations and drops blank and comment
// lines the way systemd reads a unit file.
func logicalUnitLines(unit string) []string {
	var logical []string
	var pending strings.Builder
	for _, line := range strings.Split(unit, "\n") {
		line = strings.TrimRight(line, "\r")
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, ";") {
			continue
		}
		if strings.HasSuffix(line, "\\") {
			pending.WriteString(strings.TrimSuffix(line, "\\"))
			pending.WriteByte(' ')
			continue
		}
		pending.WriteString(line)
		logical = append(logical, pending.String())
		pending.Reset()
	}
	if pending.Len() > 0 {
		logical = append(logical, pending.String())
	}
	return logical
}

// splitExecLine tokenizes a systemd command line: whitespace separates
// words, single or double quotes group, and inside quotes a backslash
// escapes the next quote or backslash. Other escapes are rejected because the
// panel never writes them and their expansion cannot be verified here.
func splitExecLine(line string) ([]string, bool) {
	var words []string
	var current strings.Builder
	inWord := false
	var quote rune
	runes := []rune(line)
	for index := 0; index < len(runes); index++ {
		character := runes[index]
		switch {
		case quote != 0:
			switch character {
			case quote:
				quote = 0
			case '\\':
				if index+1 >= len(runes) {
					return nil, false
				}
				next := runes[index+1]
				if next != '\\' && next != '"' && next != '\'' {
					return nil, false
				}
				current.WriteRune(next)
				index++
			default:
				current.WriteRune(character)
			}
		case character == '"' || character == '\'':
			quote = character
			inWord = true
		case character == ' ' || character == '\t':
			if inWord {
				words = append(words, current.String())
				current.Reset()
				inWord = false
			}
		case character == '\\':
			return nil, false
		default:
			current.WriteRune(character)
			inWord = true
		}
	}
	if quote != 0 {
		return nil, false
	}
	if inWord {
		words = append(words, current.String())
	}
	return words, true
}

// resolveExecWord undoes the literal specifier and variable escapes the
// installer writes. Any other % specifier or $ variable would be expanded by
// systemd at start time, so the resulting path cannot be verified statically.
func resolveExecWord(word string) (string, bool) {
	var output strings.Builder
	runes := []rune(word)
	for index := 0; index < len(runes); index++ {
		character := runes[index]
		if character != '%' && character != '$' {
			output.WriteRune(character)
			continue
		}
		if index+1 >= len(runes) || runes[index+1] != character {
			return "", false
		}
		output.WriteRune(character)
		index++
	}
	return output.String(), true
}
