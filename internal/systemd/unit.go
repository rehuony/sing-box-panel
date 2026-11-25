// SPDX-License-Identifier: GPL-3.0-or-later

package systemd

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/rehuony/sing-box-panel/internal/pathguard"
)

// Render returns the unit for scope that runs executable with the state at
// statePath and its data in dataDir.
func (manager *Manager[T]) Render(scope Scope, executablePath, statePath, dataDir string) ([]byte, error) {
	definition := manager.definition
	template := definition.SystemUnit
	if scope == ScopeUser {
		template = definition.UserUnit
	}
	if template == nil {
		return nil, fmt.Errorf("%w: %s has no %s unit", ErrInvalid, definition.Name, scope)
	}
	command, err := manager.execStart(executablePath, statePath)
	if err != nil {
		return nil, err
	}
	result, err := replaceDirective(template, "ExecStart=", "ExecStart="+command)
	if err != nil {
		return nil, err
	}
	if hasDirective(result, "ExecStartPre=") {
		preStart, err := manager.preStart(executablePath, dataDir)
		if err != nil {
			return nil, err
		}
		if result, err = replaceDirective(result, "ExecStartPre=", "ExecStartPre="+preStart); err != nil {
			return nil, err
		}
	}
	workingDirectory, err := workingDirectoryValue(dataDir)
	if err != nil {
		return nil, err
	}
	result, err = replaceDirective(result, "WorkingDirectory=", "WorkingDirectory="+workingDirectory)
	if err != nil {
		return nil, err
	}
	writable := []string{dataDir}
	if definition.StateFile {
		writable = append(writable, filepath.Dir(statePath))
	}
	quotedWritable := make([]string, 0, len(writable))
	for _, path := range writable {
		quoted, err := quotePathDirective(path)
		if err != nil {
			return nil, err
		}
		quotedWritable = append(quotedWritable, quoted)
	}
	// A template that confines the file system names the writable paths
	// exactly once; one without ProtectSystem may leave them out.
	if hasDirective(result, "ProtectSystem=") || hasDirective(result, "ReadWritePaths=") {
		result, err = replaceDirective(result, "ReadWritePaths=", "ReadWritePaths="+strings.Join(quotedWritable, " "))
		if err != nil {
			return nil, err
		}
	}
	if scope == ScopeSystem && dataDir != "/var/lib/"+definition.Name {
		// Custom roots are created by the installer; do not recreate the former
		// default directory on each subsequent start.
		result, err = replaceDirective(result, "StateDirectory=", "StateDirectory=")
		if err != nil {
			return nil, err
		}
	}
	if hasDirective(result, "PrivateTmp=") {
		return bindTemporaryPaths(result, writable)
	}
	return result, nil
}

// execStart is the command line the unit runs: executable with the state at
// statePath.
func (manager *Manager[T]) execStart(executablePath, statePath string) (string, error) {
	executable, err := quoteExecArgument(executablePath)
	if err != nil {
		return "", err
	}
	state, err := quoteExecArgument(statePath)
	if err != nil {
		return "", err
	}
	return executable + " " + manager.definition.Arguments(state), nil
}

// installStepPrefix makes the install step run with full privileges, as
// root and outside the unit's sandbox, and lets the service start whatever
// the step's outcome; the step records a failure for the service to report.
const installStepPrefix = "-+"

// preStart is the install step's command line: executable with the data
// directory, run before the service.
func (manager *Manager[T]) preStart(executablePath, dataDir string) (string, error) {
	if manager.definition.PreStartArguments == nil {
		return "", fmt.Errorf("%w: %s has no install step arguments", ErrInvalid, manager.definition.Name)
	}
	executable, err := quoteExecArgument(executablePath)
	if err != nil {
		return "", err
	}
	data, err := quoteExecArgument(dataDir)
	if err != nil {
		return "", err
	}
	return installStepPrefix + executable + " " + manager.definition.PreStartArguments(data), nil
}

// unitOutdated reports whether the managed unit in scope runs a command
// line this release does not write for the same executable and state, as
// a unit installed by an earlier release with other arguments does: the
// executable it starts may not accept them. A system unit also needs this
// release's install step, without which a staged update is never
// installed. A missing or foreign unit, or one whose command line cannot be
// read, is not reported.
func (manager *Manager[T]) unitOutdated(scope Scope) bool {
	unit, err := os.ReadFile(manager.unitPath(scope))
	if err != nil || !bytes.Contains(unit, []byte(manager.managedMark())) {
		return false
	}
	command, ok := effectiveExecStart(string(unit))
	if !ok {
		return false
	}
	words, ok := splitExecLine(command)
	if !ok || len(words) == 0 {
		return false
	}
	executable, ok := resolveExecWord(words[0])
	if !ok || !filepath.IsAbs(executable) {
		return false
	}
	statePath, ok := manager.parseUnitFileStatePath(unit)
	if !ok {
		return true
	}
	expected, err := manager.execStart(executable, statePath)
	if err != nil {
		return false
	}
	if expected != command {
		return true
	}
	if scope != ScopeSystem || manager.definition.PreStartArguments == nil {
		return false
	}
	preStart, ok := effectiveDirective(string(unit), "ExecStartPre")
	if !ok {
		return true
	}
	preStartWords, ok := splitExecLine(strings.TrimPrefix(preStart, installStepPrefix))
	if !ok || len(preStartWords) < 2 {
		return true
	}
	dataDir, ok := flagArgument(preStartWords[1:], "--data-dir", "")
	if !ok {
		return true
	}
	expectedPreStart, err := manager.preStart(executable, dataDir)
	return err != nil || expectedPreStart != preStart
}

// refreshCommand is how an operator rewrites the unit in scope with the
// installed release.
func (manager *Manager[T]) refreshCommand(scope Scope) string {
	if scope == ScopeSystem {
		return "sudo " + manager.definition.Name + " systemd install --scope=system"
	}
	return manager.definition.Name + " systemd install --scope=user"
}

func hasDirective(source []byte, prefix string) bool {
	for _, line := range strings.Split(string(source), "\n") {
		if strings.HasPrefix(line, prefix) {
			return true
		}
	}
	return false
}

// bindTemporaryPaths keeps PrivateTmp from hiding explicitly selected state
// or configuration: it binds only these dedicated directories, leaving the
// remaining sandbox intact.
func bindTemporaryPaths(result []byte, writable []string) ([]byte, error) {
	var err error
	var temporaryPaths []string
	for _, path := range writable {
		for _, root := range []string{"/tmp", "/var/tmp"} {
			inside, err := pathguard.Contains(root, path)
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
