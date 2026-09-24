// SPDX-License-Identifier: GPL-3.0-or-later

package systemd

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/rehuony/sing-box-panel/internal/installation"
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
	result, err := replaceDirective(template, "ExecStart=", "ExecStart="+executable+" server start --config "+settings)
	if err != nil {
		return nil, err
	}
	workingDirectory, err := workingDirectoryValue(dataDir)
	if err != nil {
		return nil, err
	}
	result, err = replaceDirective(result, "WorkingDirectory=", "WorkingDirectory="+workingDirectory)
	if err != nil {
		return nil, err
	}
	settingsDirectory, err := quotePathDirective(filepath.Dir(settingsPath))
	if err != nil {
		return nil, err
	}
	result, err = replaceDirective(result, "ReadWritePaths=", "ReadWritePaths="+data+" "+settingsDirectory)
	if err != nil {
		return nil, err
	}
	if scope == ScopeSystem && dataDir != "/var/lib/sing-box-panel" {
		// Custom roots are created by the installer; do not recreate the former
		// default directory on each subsequent start.
		result, err = replaceDirective(result, "StateDirectory=", "StateDirectory=")
		if err != nil {
			return nil, err
		}
	}
	// PrivateTmp must not hide explicitly selected state or configuration.
	// Bind only these dedicated directories, leaving the remaining sandbox intact.
	var temporaryPaths []string
	for _, path := range []string{dataDir, filepath.Dir(settingsPath)} {
		for _, root := range []string{"/tmp", "/var/tmp"} {
			inside, err := installation.DataDirectoryContainsPath(root, path)
			if err != nil {
				return nil, err
			}
			if !inside {
				continue
			}
			if strings.Contains(path, ":") {
				return nil, fmt.Errorf("%w: temporary service paths must not contain colons", ErrInvalid)
			}
			quoted, err := quotePathDirective(path)
			if err != nil {
				return nil, err
			}
			temporaryPaths = append(temporaryPaths, quoted)
			break
		}
	}
	if len(temporaryPaths) != 0 {
		result, err = replaceDirective(result, "PrivateTmp=", "PrivateTmp=true\nBindPaths="+strings.Join(temporaryPaths, " "))
		if err != nil {
			return nil, err
		}
	}
	return result, nil
}

func quoteExecArgument(value string) (string, error) {
	return quoteUnitValue(value, true)
}

// WorkingDirectory is a scalar path, not an unquoted word list. systemd keeps
// quotes and backslashes literally here, but expands percent specifiers.
func workingDirectoryValue(value string) (string, error) {
	if !filepath.IsAbs(value) || hasControl(value) || strings.TrimSpace(value) != value || strings.HasSuffix(value, "\\") {
		return "", fmt.Errorf("%w: working directory cannot be represented losslessly", ErrInvalid)
	}
	return strings.ReplaceAll(value, "%", "%%"), nil
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
