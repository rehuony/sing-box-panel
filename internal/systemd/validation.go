// SPDX-License-Identifier: GPL-3.0-or-later

package systemd

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/rehuony/sing-box-panel/internal/pathguard"
)

// ExecRunner runs commands with a two-minute bound; it is the default Runner.
type ExecRunner struct{}

func (ExecRunner) Run(ctx context.Context, name string, args ...string) (CommandResult, error) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	command := exec.CommandContext(ctx, name, args...)
	command.WaitDelay = 5 * time.Second
	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	err := command.Run()
	if err != nil && ctx.Err() != nil {
		err = ctx.Err()
	}
	return CommandResult{Stdout: stdout.Bytes(), Stderr: stderr.Bytes()}, err
}

func (manager *Manager[T]) requireLinux() error {
	if manager.goos != "linux" {
		return fmt.Errorf("%w: running on %s", ErrUnsupportedOS, manager.goos)
	}
	return nil
}

func (manager *Manager[T]) resolveScope(scope Scope) (Scope, error) {
	parsed, err := ParseScope(string(scope))
	if err != nil {
		return "", err
	}
	if parsed == ScopeAuto {
		parsed = ScopeUser
		if manager.euid() == 0 {
			parsed = ScopeSystem
		}
	}
	if parsed == ScopeUser && manager.definition.UserUnit == nil {
		return "", fmt.Errorf("%w: %s has no user service", ErrInvalid, manager.definition.Name)
	}
	if parsed == ScopeUser && manager.layout.UserUnitPath == "" {
		return "", fmt.Errorf("%w: user scope needs $XDG_CONFIG_HOME or $HOME", ErrInvalid)
	}
	return parsed, nil
}

func (manager *Manager[T]) requireMutationPermission(scope Scope) error {
	if scope == ScopeSystem && manager.euid() != 0 {
		return ErrPermission
	}
	return nil
}

func (manager *Manager[T]) validateInstallPaths(scope Scope, request InstallRequest) (string, string, string, error) {
	executablePath, err := manager.executable()
	if err != nil {
		return "", "", "", fmt.Errorf("resolve current executable: %w", err)
	}
	executablePath, err = cleanAbsolute(executablePath, "executable path")
	if err != nil {
		return "", "", "", err
	}
	statePath, err := cleanAbsolute(request.StatePath, manager.definition.StateLabel+" path")
	if err != nil {
		return "", "", "", err
	}
	dataDir, err := cleanAbsolute(request.DataDir, "data directory")
	if err != nil {
		return "", "", "", err
	}
	if scope == ScopeSystem {
		if executablePath != manager.layout.SystemExecutablePath || statePath != manager.layout.SystemStatePath {
			return "", "", "", fmt.Errorf(
				"%w: system scope requires executable=%q %s=%q",
				ErrInvalid, manager.layout.SystemExecutablePath, manager.definition.StateLabel, manager.layout.SystemStatePath,
			)
		}
		for _, protected := range []string{"/home", "/root", "/run/user"} {
			if dataDir == protected || strings.HasPrefix(dataDir, protected+"/") {
				return "", "", "", fmt.Errorf("%w: system service data must be outside protected home directories", ErrInvalid)
			}
		}
		for _, protected := range []string{"/home", "/root", "/run/user"} {
			inside, err := pathguard.Contains(protected, dataDir)
			if err != nil {
				return "", "", "", err
			}
			if inside {
				return "", "", "", fmt.Errorf("%w: system service data must be outside protected home directories", ErrInvalid)
			}
		}
	}
	if err := requireRegularExecutable(executablePath); err != nil {
		return "", "", "", err
	}
	if err := validateServiceDirectory(executablePath, dataDir); err != nil {
		return "", "", "", fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	if err := manager.definition.State.Validate(executablePath, statePath, dataDir); err != nil {
		return "", "", "", fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	if err := requireDirectory(dataDir); err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", "", "", fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	return executablePath, statePath, dataDir, nil
}

// validateServiceDirectory refuses a data directory that the service account
// must not own: a shared system directory, or one holding the executable.
func validateServiceDirectory(executablePath, dataDir string) error {
	if err := pathguard.RejectShared(dataDir); err != nil {
		return fmt.Errorf("data directory %q: %w", dataDir, err)
	}
	resolvedData, err := pathguard.Resolve(dataDir)
	if err != nil {
		return err
	}
	resolvedExecutable, err := pathguard.Resolve(executablePath)
	if err != nil {
		return err
	}
	if pathguard.Within(resolvedData, resolvedExecutable) {
		return errors.New("the executable is inside the data directory; move it outside")
	}
	return nil
}

func (manager *Manager[T]) installFiles(scope Scope, unit []byte, dataDir string) ([]managedFile, error) {
	files := []managedFile{{path: manager.unitPath(scope), data: unit}}
	if scope == ScopeSystem {
		path, err := quotePathDirective(dataDir)
		if err != nil {
			return nil, err
		}
		tmpfiles := strings.Replace(string(manager.definition.Tmpfiles), "/var/lib/"+manager.definition.Name, path, 1)
		files = append(files,
			managedFile{path: manager.layout.SystemSysusersPath, data: manager.definition.Sysusers},
			managedFile{path: manager.layout.SystemTmpfilesPath, data: []byte(tmpfiles)},
		)
	}
	return files, nil
}

// ManagedPaths lists the files the installer writes for scope.
func (manager *Manager[T]) ManagedPaths(scope Scope) []string { return manager.managedPaths(scope) }

func (manager *Manager[T]) managedPaths(scope Scope) []string {
	paths := []string{manager.unitPath(scope)}
	if scope == ScopeSystem {
		paths = append(paths, manager.layout.SystemSysusersPath, manager.layout.SystemTmpfilesPath)
	}
	return paths
}

func (manager *Manager[T]) unitPath(scope Scope) string {
	if scope == ScopeSystem {
		return manager.layout.SystemUnitPath
	}
	return manager.layout.UserUnitPath
}

func (manager *Manager[T]) systemctlPrefix(scope Scope) []string {
	args := []string{"--no-ask-password"}
	if scope == ScopeUser {
		args = append(args, "--user")
	}
	return args
}

func (manager *Manager[T]) runSystemctl(ctx context.Context, scope Scope, args ...string) error {
	all := append(manager.systemctlPrefix(scope), args...)
	return manager.run(ctx, "systemctl", all...)
}

func (manager *Manager[T]) run(ctx context.Context, name string, args ...string) error {
	_, err := manager.runResult(ctx, name, args...)
	return err
}

func (manager *Manager[T]) runResult(ctx context.Context, name string, args ...string) (CommandResult, error) {
	result, err := manager.runner.Run(ctx, name, args...)
	if err != nil {
		return CommandResult{}, &CommandError{
			Name: name, Args: append([]string(nil), args...),
			Stderr: strings.TrimSpace(string(result.Stderr)), Cause: err,
		}
	}
	return result, nil
}

func resolvedLayout[T any](definition Definition[T], layout Layout) (Layout, error) {
	// A process without a home, such as a root unit, has no user scope; its
	// user unit path stays empty and user scope is refused.
	userUnitPath := ""
	if configDir, err := os.UserConfigDir(); err == nil {
		userUnitPath = filepath.Join(configDir, "systemd", "user", definition.Name+".service")
	}
	name := definition.Name
	defaults := Layout{
		SystemUnitPath:       "/etc/systemd/system/" + name + ".service",
		SystemSysusersPath:   "/etc/sysusers.d/" + name + ".conf",
		SystemTmpfilesPath:   "/etc/tmpfiles.d/" + name + ".conf",
		SystemExecutablePath: "/usr/local/bin/" + name,
		SystemStatePath:      definition.SystemStatePath,
		SystemDataDir:        "/var/lib/" + name,
		UserUnitPath:         userUnitPath,
	}
	for _, override := range []struct{ value, target *string }{
		{&layout.SystemUnitPath, &defaults.SystemUnitPath}, {&layout.SystemSysusersPath, &defaults.SystemSysusersPath},
		{&layout.SystemTmpfilesPath, &defaults.SystemTmpfilesPath}, {&layout.SystemExecutablePath, &defaults.SystemExecutablePath},
		{&layout.SystemStatePath, &defaults.SystemStatePath}, {&layout.SystemDataDir, &defaults.SystemDataDir},
		{&layout.UserUnitPath, &defaults.UserUnitPath},
	} {
		if *override.value != "" {
			*override.target = *override.value
		}
	}
	for name, value := range map[string]string{
		"system unit path": defaults.SystemUnitPath, "system sysusers path": defaults.SystemSysusersPath,
		"system tmpfiles path": defaults.SystemTmpfilesPath, "system executable path": defaults.SystemExecutablePath,
		"system state path": defaults.SystemStatePath, "system data directory": defaults.SystemDataDir,
		"user unit path": defaults.UserUnitPath,
	} {
		if value == "" && name == "user unit path" {
			continue
		}
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
