// SPDX-License-Identifier: GPL-3.0-or-later

package systemd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/rehuony/sing-box-panel/internal/installation"
	"github.com/rehuony/sing-box-panel/internal/settings"
)

func (manager *Manager) Install(ctx context.Context, request InstallRequest) (InstallResult, error) {
	if err := manager.requireLinux(); err != nil {
		return InstallResult{}, err
	}
	scope, err := manager.resolveScope(request.Scope)
	if err != nil {
		return InstallResult{}, err
	}
	if err := manager.requireMutationPermission(scope); err != nil {
		return InstallResult{}, err
	}
	request, err = manager.resolveInstallSettings(scope, request)
	if err != nil {
		return InstallResult{}, err
	}
	executablePath, settingsPath, dataDir, err := manager.validateInstallPaths(scope, request)
	if err != nil {
		return InstallResult{}, err
	}

	unit, err := renderUnit(scope, executablePath, settingsPath, dataDir)
	if err != nil {
		return InstallResult{}, err
	}
	files := manager.installFiles(scope, unit, dataDir)
	if err := preflightInstall(files, request.Force); err != nil {
		return InstallResult{}, err
	}
	for _, path := range append([]string{settingsPath}, manager.managedPaths(scope)...) {
		if err := writableParent(path); err != nil {
			return InstallResult{}, err
		}
	}
	if err := writableDirectory(dataDir); err != nil {
		return InstallResult{}, err
	}
	if err := manager.preflightCommands(ctx, scope, true); err != nil {
		return InstallResult{}, err
	}
	if scope == ScopeSystem {
		if _, err := manager.inspectSystemAccount(ctx); err != nil {
			return InstallResult{}, err
		}
	}
	if location, err := settings.ReadDataLocation(settingsPath); err == nil && (location.DataDir != dataDir || location.Move != nil) {
		prepared, err := installation.PrepareDataLocation(ctx, settingsPath)
		if err != nil {
			return InstallResult{}, err
		}
		if prepared.DataDir != dataDir {
			return InstallResult{}, errors.New("data directory changed during service installation; retry with the current settings")
		}
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return InstallResult{}, err
	}
	created, err := prepareResources(ctx, request)
	if err != nil {
		return InstallResult{}, err
	}
	installed := make([]string, 0, len(files))
	for _, file := range files {
		changed, writeErr := installFile(file.path, file.data)
		if writeErr != nil {
			return InstallResult{}, writeErr
		}
		if changed {
			installed = append(installed, file.path)
		}
	}

	if scope == ScopeSystem {
		if err := manager.prepareSystemOwnership(ctx, settingsPath, dataDir); err != nil {
			return InstallResult{}, err
		}
	}

	if err := manager.runSystemctl(ctx, scope, "daemon-reload"); err != nil {
		return InstallResult{}, err
	}
	enableArgs := []string{"enable"}
	if request.Now {
		enableArgs = append(enableArgs, "--now")
	}
	enableArgs = append(enableArgs, UnitName)
	if err := manager.runSystemctl(ctx, scope, enableArgs...); err != nil {
		return InstallResult{}, err
	}

	return InstallResult{
		SettingsCreated: created,
		Scope:           scope, Unit: UnitName, UnitPath: manager.unitPath(scope), ExecutablePath: executablePath,
		SettingsPath: settingsPath, DataDir: dataDir, InstalledPaths: installed,
		Enabled: true, Started: request.Now, PersistentState: true,
	}, nil
}

func (manager *Manager) Uninstall(ctx context.Context, request UninstallRequest) (result UninstallResult, uninstallErr error) {
	if err := manager.requireLinux(); err != nil {
		return UninstallResult{}, err
	}
	scope, err := manager.resolveScope(request.Scope)
	if err != nil {
		return UninstallResult{}, err
	}
	if err := manager.requireMutationPermission(scope); err != nil {
		return UninstallResult{}, err
	}
	result = UninstallResult{Scope: scope, Unit: UnitName, RemovedPaths: []string{},
		ConfigRetained: true, DataRetained: true}
	paths := manager.managedPaths(scope)
	existing, err := preflightUninstall(paths, request.Force)
	if err != nil {
		return result, err
	}
	files, err := manager.Files(ctx, scope)
	if err != nil {
		return result, err
	}
	var links []string
	for _, file := range files.Files {
		if file.State == "enablement link" {
			links = append(links, file.Path)
		}
	}
	identities := make(map[string]os.FileInfo, len(existing))
	for _, path := range existing {
		info, err := os.Lstat(path)
		if err != nil {
			return result, err
		}
		identities[path] = info
	}
	status, err := manager.queryStatus(ctx, scope)
	if err != nil {
		return result, err
	}
	if status.UnitPath != "" && status.UnitPath != manager.unitPath(scope) {
		return result, fmt.Errorf("%w: loaded service belongs to another installation", ErrConflict)
	}
	var account systemAccount
	if scope == ScopeSystem {
		account, err = manager.preflightAccountRemoval(ctx, request.KeepUser, &result)
		if err != nil {
			return result, err
		}
		if account != (systemAccount{}) {
			if err := manager.validateAccountRemovalStatus(status); err != nil {
				return result, err
			}
			if _, err := manager.retainedAccountPaths(); err != nil {
				return result, err
			}
		}
	}
	if len(existing) == 0 && len(links) == 0 && status.LoadState == "not-found" && unitStopped(status) {
		result.Stopped = true
		return result, nil
	}
	if !unitStopped(status) {
		if status.UnitPath != manager.unitPath(scope) {
			return result, fmt.Errorf("%w: cannot establish running service ownership", ErrConflict)
		}
		if err := manager.runSystemctl(ctx, scope, "stop", UnitName); err != nil {
			return result, err
		}
		status, err = manager.queryStatus(ctx, scope)
		if err != nil {
			return result, err
		}
		if !unitStopped(status) {
			return result, errors.New("service has not stopped; retained managed files")
		}
	}
	result.Stopped = true
	if fileExists(manager.unitPath(scope)) || len(links) > 0 {
		if err := manager.runSystemctl(ctx, scope, "disable", "--no-reload", UnitName); err != nil {
			return result, err
		}
		result.Disabled = true
	}
	for _, path := range links {
		if _, err := os.Lstat(path); errors.Is(err, os.ErrNotExist) {
			result.RemovedPaths = append(result.RemovedPaths, path)
		}
	}
	if scope == ScopeSystem {
		// Revalidate file identity before account removal as well as before
		// unlinking. A concurrent replacement must not lose its service user.
		for _, path := range existing {
			info, err := os.Lstat(path)
			if err != nil {
				return result, err
			}
			if !os.SameFile(identities[path], info) {
				return result, fmt.Errorf("service file changed during uninstall: %s", path)
			}
		}
		if err := manager.removeSystemAccount(ctx, account, &result); err != nil {
			return result, err
		}
	}
	for _, path := range existing {
		info, err := os.Lstat(path)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return result, err
		}
		if !os.SameFile(identities[path], info) {
			return result, fmt.Errorf("service file changed during uninstall: %s", path)
		}
		if err := os.Remove(path); err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return result, fmt.Errorf("remove managed systemd file %q: %w", path, err)
		}
		result.RemovedPaths = append(result.RemovedPaths, path)
		if err := syncDirectory(filepath.Dir(path)); err != nil {
			return result, err
		}
	}
	if err := manager.runSystemctl(ctx, scope, "daemon-reload"); err != nil {
		return result, err
	}
	return result, nil
}

func unitStopped(status Status) bool {
	return status.MainPID == 0 && (status.ActiveState == "inactive" || status.ActiveState == "failed")
}

func (manager *Manager) prepareSystemOwnership(ctx context.Context, settingsPath, dataDir string) error {
	if _, err := manager.inspectSystemAccount(ctx); err != nil {
		return err
	}
	if err := manager.run(ctx, "systemd-sysusers", manager.layout.SystemSysusersPath); err != nil {
		return err
	}
	if err := manager.run(ctx, "systemd-tmpfiles", "--create", manager.layout.SystemTmpfilesPath); err != nil {
		return err
	}
	settingsDirectory := filepath.Dir(settingsPath)
	for _, path := range []string{settingsDirectory, settingsPath, settingsPath + ".lock", settingsPath + ".pending", settingsPath + ".location"} {
		info, err := os.Lstat(path)
		if errors.Is(err, os.ErrNotExist) && path != settingsPath && path != settingsDirectory {
			continue
		}
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 || (path != settingsDirectory && !info.Mode().IsRegular()) {
			return fmt.Errorf("invalid settings path %q", path)
		}
		if err := manager.run(ctx, "chown", serviceUser+":"+serviceGroup, path); err != nil {
			return err
		}
		mode := os.FileMode(0600)
		if path == settingsDirectory {
			mode = 0700
		}
		if err := os.Chmod(path, mode); err != nil {
			return fmt.Errorf("set settings permissions: %w", err)
		}
	}
	if err := manager.run(ctx, "chown", "--recursive", "--no-dereference", serviceUser+":"+serviceGroup, dataDir); err != nil {
		return err
	}

	return nil
}
