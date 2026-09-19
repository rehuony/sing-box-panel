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
		if err := manager.run(ctx, "systemd-sysusers", manager.layout.SystemSysusersPath); err != nil {
			return InstallResult{}, err
		}
		if err := manager.run(ctx, "systemd-tmpfiles", "--create", manager.layout.SystemTmpfilesPath); err != nil {
			return InstallResult{}, err
		}
		settingsDirectory := filepath.Dir(settingsPath)
		for _, path := range []string{settingsDirectory, settingsPath, settingsPath + ".lock", settingsPath + ".pending", settingsPath + ".location"} {
			info, err := os.Lstat(path)
			if errors.Is(err, os.ErrNotExist) && path != settingsPath && path != settingsDirectory {
				continue
			}
			if err != nil {
				return InstallResult{}, err
			}
			if info.Mode()&os.ModeSymlink != 0 || (path != settingsDirectory && !info.Mode().IsRegular()) {
				return InstallResult{}, fmt.Errorf("invalid settings path %q", path)
			}
			if err := manager.run(ctx, "chown", serviceUser+":"+serviceGroup, path); err != nil {
				return InstallResult{}, err
			}
			mode := os.FileMode(0600)
			if path == settingsDirectory {
				mode = 0700
			}
			if err := os.Chmod(path, mode); err != nil {
				return InstallResult{}, fmt.Errorf("set settings permissions: %w", err)
			}
		}
		if err := manager.run(ctx, "chown", "--recursive", "--no-dereference", serviceUser+":"+serviceGroup, dataDir); err != nil {
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
		Scope: scope, Unit: UnitName, UnitPath: manager.unitPath(scope), ExecutablePath: executablePath,
		SettingsPath: settingsPath, DataDir: dataDir, InstalledPaths: installed,
		Enabled: true, Started: request.Now, PersistentState: true,
	}, nil
}

func (manager *Manager) Uninstall(ctx context.Context, request UninstallRequest) (UninstallResult, error) {
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
	paths := manager.managedPaths(scope)
	existing, err := preflightUninstall(paths, request.Force)
	if err != nil {
		return UninstallResult{}, err
	}
	if len(existing) == 0 {
		return UninstallResult{}, ErrNotInstalled
	}
	unitPresent := fileExists(manager.unitPath(scope))
	if unitPresent {
		if err := manager.runSystemctl(ctx, scope, "disable", "--now", UnitName); err != nil {
			return UninstallResult{}, err
		}
	}
	removed := make([]string, 0, len(existing))
	for _, path := range existing {
		if err := os.Remove(path); err != nil {
			return UninstallResult{}, fmt.Errorf("remove managed systemd file %q: %w", path, err)
		}
		if err := syncDirectory(filepath.Dir(path)); err != nil {
			return UninstallResult{}, err
		}
		removed = append(removed, path)
	}
	if err := manager.runSystemctl(ctx, scope, "daemon-reload"); err != nil {
		return UninstallResult{}, err
	}
	return UninstallResult{
		Scope: scope, Unit: UnitName, RemovedPaths: removed, Stopped: unitPresent, Disabled: unitPresent,
		ConfigRetained: true, DataRetained: true, AccountRetained: scope == ScopeSystem,
	}, nil
}
