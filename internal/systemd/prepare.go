// SPDX-License-Identifier: GPL-3.0-or-later

package systemd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
)

// resolveInstall resolves the data directory without creating anything, so
// permission, command and file conflicts are reported before first-run
// initialization.
func (manager *Manager[T]) resolveInstall(scope Scope, request InstallRequest) (InstallRequest, error) {
	if _, err := cleanAbsolute(request.StatePath, manager.definition.StateLabel+" path"); err != nil {
		return request, err
	}
	fallback := ""
	if scope == ScopeSystem {
		fallback = manager.layout.SystemDataDir
	}
	if request.DataDir != "" {
		fallback = request.DataDir
	}
	dataDir, err := manager.definition.State.DataDir(request.StatePath, fallback, request.Now)
	if err != nil {
		return request, err
	}
	if request.DataDir != "" && request.DataDir != dataDir {
		return request, fmt.Errorf("%w: configured data directory changed; inspect %s and retry", ErrInvalid, manager.definition.StateLabel)
	}
	request.DataDir = dataDir
	return request, nil
}

func (manager *Manager[T]) preflightCommands(ctx context.Context, scope Scope, install bool) error {
	names := []string{"systemctl"}
	if install && scope == ScopeSystem {
		names = append(names, "systemd-sysusers", "systemd-tmpfiles", "chown", "getent")
	}
	for _, name := range names {
		if _, err := manager.lookPath(name); err != nil {
			return fmt.Errorf("required command %s is unavailable: %w", name, err)
		}
	}
	return manager.runSystemctl(ctx, scope, "show", "--property=Version", "--value")
}

// prepareResources creates the missing state and data directory of an
// installation.
func (manager *Manager[T]) prepareResources(ctx context.Context, request InstallRequest) (*T, error) {
	initial, err := manager.definition.State.Prepare(ctx, request.StatePath, request.DataDir, request.Now)
	if err != nil {
		return initial, err
	}
	if err := ctx.Err(); err != nil {
		return initial, err
	}
	if err := os.MkdirAll(request.DataDir, 0o700); err != nil {
		return initial, fmt.Errorf("create service data directory: %w", err)
	}
	return initial, requireDirectory(request.DataDir)
}

// prepareInstalledResources recreates the missing state and data directory
// of an installed managed unit before it starts.
func (manager *Manager[T]) prepareInstalledResources(ctx context.Context, scope Scope) (*T, error) {
	files, err := manager.managedFiles(ctx, scope)
	if err != nil {
		return nil, err
	}
	if files.StatePath == "" || len(files.Files) == 0 || !files.Files[0].Managed {
		return nil, nil // systemctl handles absent and externally managed units.
	}
	request := InstallRequest{StatePath: files.StatePath, Now: true}
	if _, err := os.Lstat(files.StatePath); errors.Is(err, os.ErrNotExist) {
		// When the state is lost, the exact generated unit still identifies its
		// working directory. Verify the entire unit below before trusting this
		// hint to create anything.
		unit, err := os.ReadFile(manager.unitPath(scope))
		if err != nil {
			return nil, err
		}
		for _, line := range strings.Split(string(unit), "\n") {
			if !strings.HasPrefix(line, "WorkingDirectory=") {
				continue
			}
			if request.DataDir != "" {
				return nil, errors.New("ambiguous service working directory")
			}
			value := strings.TrimPrefix(line, "WorkingDirectory=")
			if strings.Contains(strings.ReplaceAll(value, "%%", ""), "%") {
				return nil, errors.New("unresolved working-directory specifier")
			}
			request.DataDir = strings.ReplaceAll(value, "%%", "%")
		}
		if request.DataDir == "" {
			return nil, errors.New("service does not identify its data directory")
		}
	}
	request, err = manager.resolveInstall(scope, request)
	if err != nil {
		return nil, err
	}
	_, stateErr := os.Lstat(request.StatePath)
	_, dataErr := os.Lstat(request.DataDir)
	if stateErr == nil && dataErr == nil {
		return nil, nil
	}
	current, err := manager.queryStatus(ctx, scope)
	if err != nil {
		return nil, err
	}
	if current.NeedDaemonReload || current.UnitFileStatePath != files.StatePath || current.UnitPath != manager.unitPath(scope) {
		return nil, errors.New("resource preparation requires an unmodified, reloaded managed service; reinstall explicitly")
	}
	executable, _, _, err := manager.validateInstallPaths(scope, request)
	if err != nil {
		return nil, err
	}
	unit, err := manager.Render(scope, executable, request.StatePath, request.DataDir)
	if err != nil {
		return nil, err
	}
	if err := preflightInstall(manager.installFiles(scope, unit, request.DataDir), false); err != nil {
		return nil, err
	}
	if err := manager.preflightCommands(ctx, scope, true); err != nil {
		return nil, err
	}
	initial, err := manager.prepareResources(ctx, request)
	if err != nil {
		return initial, err
	}
	if scope == ScopeSystem {
		return initial, manager.prepareSystemOwnership(ctx, request.StatePath, request.DataDir)
	}
	return initial, nil
}

// prepareDataMove moves the data directory of a start or restart that its
// state relocated, outside the service sandbox. Only byte-for-byte
// generated units may be rewritten automatically; custom units and drop-ins
// keep their explicit install and force workflow.
func (manager *Manager[T]) prepareDataMove(ctx context.Context, scope Scope, action Action) error {
	if action == ActionStop {
		return nil
	}
	files, err := manager.managedFiles(ctx, scope)
	if err != nil {
		return err
	}
	if files.StatePath == "" {
		return nil
	}
	current, target, move, err := manager.definition.State.Location(files.StatePath)
	if err != nil {
		return err
	}
	if current == "" || (current == target && !move) {
		return nil
	}
	status, err := manager.Status(ctx, scope)
	if err != nil {
		return err
	}
	if status.UnitPath != manager.unitPath(scope) || status.UnitFileStatePath != files.StatePath || status.NeedDaemonReload {
		return errors.New("data migration requires an unmodified, reloaded managed service unit")
	}
	if action == ActionStart && status.ActiveState == "active" {
		return errors.New("the service is running; restart it explicitly to move its data directory")
	}
	executable, err := manager.executable()
	if err != nil {
		return err
	}
	oldUnit, err := manager.Render(scope, executable, files.StatePath, current)
	if err != nil {
		return err
	}
	if err := preflightInstall(manager.installFiles(scope, oldUnit, current), false); err != nil {
		return fmt.Errorf("data migration cannot rewrite customized service files; stop the service and reinstall it explicitly: %w", err)
	}
	// Validate before stopping; migration itself still proves all processes and
	// database owners have released the source before touching data.
	if _, err := manager.definition.State.DataDir(files.StatePath, "", true); err != nil {
		return err
	}
	if _, _, _, err := manager.validateInstallPaths(scope, InstallRequest{StatePath: files.StatePath, DataDir: target}); err != nil {
		return err
	}
	if action == ActionRestart {
		if err := manager.runSystemctl(ctx, scope, "stop", manager.unitName); err != nil {
			return err
		}
	}
	_, err = manager.Install(ctx, InstallRequest{Scope: scope, StatePath: files.StatePath, DataDir: target, Force: true})
	return err
}
