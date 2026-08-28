// SPDX-License-Identifier: GPL-3.0-or-later

package systemd

import (
	"fmt"
	"strings"

	systemdassets "github.com/rehuony/sing-box-panel/systemd"
)

func renderUnit(scope Scope, executablePath, settingsPath, dataDir string) ([]byte, error) {
	template := systemdassets.SystemUnit
	if scope == ScopeUser {
		template = systemdassets.UserUnit
	}
	executable, err := quoteExecArgument(executablePath)
	if err != nil {
		return nil, err
	}
	settings, err := quoteExecArgument(settingsPath)
	if err != nil {
		return nil, err
	}
	data, err := quotePathDirective(dataDir)
	if err != nil {
		return nil, err
	}
	result, err := replaceDirective(template, "ExecStart=", "ExecStart="+executable+" server run --config "+settings)
	if err != nil {
		return nil, err
	}
	result, err = replaceDirective(result, "WorkingDirectory=", "WorkingDirectory="+data)
	if err != nil {
		return nil, err
	}
	if scope == ScopeUser {
		result, err = replaceDirective(result, "ReadWritePaths=", "ReadWritePaths="+data)
		if err != nil {
			return nil, err
		}
	}
	return result, nil
}

func quoteExecArgument(value string) (string, error) {
	return quoteUnitValue(value, true)
}

func quotePathDirective(value string) (string, error) {
	return quoteUnitValue(value, false)
}

func quoteUnitValue(value string, escapeDollar bool) (string, error) {
	if value == "" || hasControl(value) {
		return "", fmt.Errorf("%w: systemd arguments must not be empty or contain control characters", ErrInvalid)
	}
	var output strings.Builder
	output.WriteByte('"')
	for _, character := range value {
		switch character {
		case '\\', '"':
			output.WriteByte('\\')
			output.WriteRune(character)
		case '%':
			output.WriteString("%%")
		case '$':
			if escapeDollar {
				output.WriteString("$$")
			} else {
				output.WriteRune(character)
			}
		default:
			output.WriteRune(character)
		}
	}
	output.WriteByte('"')
	return output.String(), nil
}

func replaceDirective(source []byte, prefix, replacement string) ([]byte, error) {
	lines := strings.Split(string(source), "\n")
	replaced := 0
	for index, line := range lines {
		if strings.HasPrefix(line, prefix) {
			lines[index] = replacement
			replaced++
		}
	}
	if replaced != 1 {
		return nil, fmt.Errorf("%w: template has %d %s directives", ErrInvalid, replaced, prefix)
	}
	return []byte(strings.Join(lines, "\n")), nil
}
