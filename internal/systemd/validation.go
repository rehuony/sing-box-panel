// SPDX-License-Identifier: GPL-3.0-or-later

package systemd

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	systemdassets "github.com/rehuony/sing-box-panel/systemd"
)

type execRunner struct{}

func (execRunner) Run(ctx context.Context, name string, args ...string) (CommandResult, error) {
	command := exec.CommandContext(ctx, name, args...)
	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	err := command.Run()
	if err != nil && ctx.Err() != nil {
		err = ctx.Err()
	}
	return CommandResult{Stdout: stdout.Bytes(), Stderr: stderr.Bytes()}, err
}

func (manager *Manager) requireLinux() error {
	if manager.goos != "linux" {
		return fmt.Errorf("%w: running on %s", ErrUnsupportedOS, manager.goos)
	}
	return nil
}

func (manager *Manager) resolveScope(scope Scope) (Scope, error) {
	parsed, err := ParseScope(string(scope))
	if err != nil {
		return "", err
	}
	if parsed == ScopeAuto {
		if manager.euid() == 0 {
			return ScopeSystem, nil
		}
		return ScopeUser, nil
	}
	return parsed, nil
}

func (manager *Manager) requireMutationPermission(scope Scope) error {
	if scope == ScopeSystem && manager.euid() != 0 {
		return ErrPermission
	}
	return nil
}

func (manager *Manager) validateInstallPaths(scope Scope, request InstallRequest) (string, string, string, error) {
	executablePath, err := manager.executable()
	if err != nil {
		return "", "", "", fmt.Errorf("resolve current executable: %w", err)
	}
	executablePath, err = cleanAbsolute(executablePath, "executable path")
	if err != nil {
		return "", "", "", err
	}
	settingsPath, err := cleanAbsolute(request.SettingsPath, "settings path")
	if err != nil {
		return "", "", "", err
	}
	dataDir, err := cleanAbsolute(request.DataDir, "data directory")
	if err != nil {
		return "", "", "", err
	}
	if scope == ScopeSystem {
		if executablePath != manager.layout.SystemExecutablePath || settingsPath != manager.layout.SystemSettingsPath || dataDir != manager.layout.SystemDataDir {
			return "", "", "", fmt.Errorf(
				"%w: system scope requires executable=%q settings=%q data=%q",
				ErrInvalid, manager.layout.SystemExecutablePath, manager.layout.SystemSettingsPath, manager.layout.SystemDataDir,
			)
		}
	}
	if err := requireRegularExecutable(executablePath); err != nil {
		return "", "", "", err
	}
	if err := requireRegularFile(settingsPath, "settings"); err != nil {
		return "", "", "", err
	}
	if err := requireDirectory(dataDir); err != nil {
		return "", "", "", err
	}
	return executablePath, settingsPath, dataDir, nil
}

func (manager *Manager) installFiles(scope Scope, unit []byte) []managedFile {
	files := []managedFile{{path: manager.unitPath(scope), data: unit}}
	if scope == ScopeSystem {
		files = append(files,
			managedFile{path: manager.layout.SystemSysusersPath, data: systemdassets.Sysusers},
			managedFile{path: manager.layout.SystemTmpfilesPath, data: systemdassets.Tmpfiles},
		)
	}
	return files
}

func (manager *Manager) managedPaths(scope Scope) []string {
	paths := []string{manager.unitPath(scope)}
	if scope == ScopeSystem {
		paths = append(paths, manager.layout.SystemSysusersPath, manager.layout.SystemTmpfilesPath)
	}
	return paths
}

func (manager *Manager) unitPath(scope Scope) string {
	if scope == ScopeSystem {
		return manager.layout.SystemUnitPath
	}
	return manager.layout.UserUnitPath
}

func (manager *Manager) systemctlPrefix(scope Scope) []string {
	args := []string{"--no-ask-password"}
	if scope == ScopeUser {
		args = append(args, "--user")
	}
	return args
}

func (manager *Manager) runSystemctl(ctx context.Context, scope Scope, args ...string) error {
	all := append(manager.systemctlPrefix(scope), args...)
	return manager.run(ctx, "systemctl", all...)
}

func (manager *Manager) run(ctx context.Context, name string, args ...string) error {
	_, err := manager.runResult(ctx, name, args...)
	return err
}

func (manager *Manager) runResult(ctx context.Context, name string, args ...string) (CommandResult, error) {
	result, err := manager.runner.Run(ctx, name, args...)
	if err != nil {
		return CommandResult{}, &CommandError{
			Name: name, Args: append([]string(nil), args...),
			Stderr: strings.TrimSpace(string(result.Stderr)), Cause: err,
		}
	}
	return result, nil
}

func resolvedLayout(layout Layout) (Layout, error) {
	configDir, err := os.UserConfigDir()
	if err != nil && layout.UserUnitPath == "" {
		return Layout{}, fmt.Errorf("resolve user config directory: %w", err)
	}
	defaults := Layout{
		SystemUnitPath:       "/etc/systemd/system/" + UnitName,
		SystemSysusersPath:   "/etc/sysusers.d/sing-box-panel.conf",
		SystemTmpfilesPath:   "/etc/tmpfiles.d/sing-box-panel.conf",
		SystemExecutablePath: "/usr/local/bin/sing-box-panel",
		SystemSettingsPath:   "/etc/sing-box-panel/setting.json",
		SystemDataDir:        "/var/lib/sing-box-panel",
		UserUnitPath:         filepath.Join(configDir, "systemd", "user", UnitName),
	}
	if layout.SystemUnitPath != "" {
		defaults.SystemUnitPath = layout.SystemUnitPath
	}
	if layout.SystemSysusersPath != "" {
		defaults.SystemSysusersPath = layout.SystemSysusersPath
	}
	if layout.SystemTmpfilesPath != "" {
		defaults.SystemTmpfilesPath = layout.SystemTmpfilesPath
	}
	if layout.SystemExecutablePath != "" {
		defaults.SystemExecutablePath = layout.SystemExecutablePath
	}
	if layout.SystemSettingsPath != "" {
		defaults.SystemSettingsPath = layout.SystemSettingsPath
	}
	if layout.SystemDataDir != "" {
		defaults.SystemDataDir = layout.SystemDataDir
	}
	if layout.UserUnitPath != "" {
		defaults.UserUnitPath = layout.UserUnitPath
	}
	for name, value := range map[string]string{
		"system unit path": defaults.SystemUnitPath, "system sysusers path": defaults.SystemSysusersPath,
		"system tmpfiles path": defaults.SystemTmpfilesPath, "system executable path": defaults.SystemExecutablePath,
		"system settings path": defaults.SystemSettingsPath, "system data directory": defaults.SystemDataDir,
		"user unit path": defaults.UserUnitPath,
	} {
		if _, err := cleanAbsolute(value, name); err != nil {
			return Layout{}, err
		}
	}
	return defaults, nil
}

func cleanAbsolute(value, name string) (string, error) {
	if strings.TrimSpace(value) == "" || !filepath.IsAbs(value) || hasControl(value) {
		return "", fmt.Errorf("%w: %s must be an absolute path without control characters", ErrInvalid, name)
	}
	clean := filepath.Clean(value)
	if clean != value {
		return "", fmt.Errorf("%w: %s must be normalized", ErrInvalid, name)
	}
	return clean, nil
}

func hasControl(value string) bool {
	for _, character := range value {
		if character < 0x20 || character == 0x7f {
			return true
		}
	}
	return false
}

func requireRegularExecutable(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("inspect executable %q: %w", path, err)
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0o111 == 0 {
		return fmt.Errorf("%w: executable path %q must be a regular executable file", ErrInvalid, path)
	}
	return nil
}

func requireRegularFile(path, label string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("inspect %s %q: %w", label, path, err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("%w: %s %q must be a regular file", ErrInvalid, label, path)
	}
	return nil
}

func requireDirectory(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("inspect data directory %q: %w", path, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("%w: data directory %q must be a directory", ErrInvalid, path)
	}
	return nil
}
