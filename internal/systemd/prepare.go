// SPDX-License-Identifier: GPL-3.0-or-later

package systemd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/rehuony/sing-box-panel/internal/settings"
)

// Resolve paths without creating anything, so permission, command and file
// conflicts are reported before first-run initialization.
func (manager *Manager) resolveInstallSettings(scope Scope, request InstallRequest) (InstallRequest, error) {
	if _, err := cleanAbsolute(request.SettingsPath, "settings path"); err != nil {
		return request, err
	}
	dataDir, err := settings.ConfiguredDataDir(request.SettingsPath)
	if errors.Is(err, os.ErrNotExist) {
		if _, statErr := os.Lstat(request.SettingsPath); !errors.Is(statErr, os.ErrNotExist) {
			return request, fmt.Errorf("%w: settings must be a regular file", ErrInvalid)
		}
		dataDir = settings.Defaults().DataDir
		if scope == ScopeSystem {
			dataDir = manager.layout.SystemDataDir
		}
		if request.DataDir != "" {
			dataDir = request.DataDir
		}
		location, locationErr := settings.ReadDataLocation(request.SettingsPath)
		if locationErr == nil {
			if location.Move != nil {
				return request, errors.New("restore settings to finish the pending data migration")
			}
			dataDir = location.DataDir
		} else if !errors.Is(locationErr, os.ErrNotExist) {
			return request, locationErr
		}
	} else if err != nil {
		return request, fmt.Errorf("%w: %w", ErrInvalid, err)
	} else if request.Now {
		if _, err := settings.Load(request.SettingsPath); err != nil {
			return request, fmt.Errorf("%w: %w", ErrInvalid, err)
		}
	}
	if request.DataDir != "" && request.DataDir != dataDir {
		return request, fmt.Errorf("%w: configured data directory changed; inspect settings and retry", ErrInvalid)
	}
	request.DataDir = dataDir
	return request, nil
}

func (manager *Manager) preflightCommands(ctx context.Context, scope Scope, install bool) error {
	names := []string{"systemctl"}
	if install && scope == ScopeSystem {
		names = append(names, "systemd-sysusers", "systemd-tmpfiles", "chown")
	}
	for _, name := range names {
		if _, err := manager.lookPath(name); err != nil {
			return fmt.Errorf("required command %s is unavailable: %w", name, err)
		}
	}
	return manager.runSystemctl(ctx, scope, "show", "--property=Version", "--value")
}

func prepareResources(ctx context.Context, request InstallRequest) (bool, error) {
	created, err := settings.EnsureFile(ctx, request.SettingsPath, request.DataDir)
	if err != nil {
		return created, err
	}
	dataDir, err := settings.ConfiguredDataDir(request.SettingsPath)
	if err != nil || dataDir != request.DataDir {
		return created, errors.New("settings changed during service preparation; inspect and retry")
	}
	if request.Now {
		if _, err := settings.Load(request.SettingsPath); err != nil {
			return created, err
		}
	}
	if err := ctx.Err(); err != nil {
		return created, err
	}
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return created, fmt.Errorf("create service data directory: %w", err)
	}
	return created, requireDirectory(dataDir)
}

func (manager *Manager) prepareInstalledResources(ctx context.Context, scope Scope) error {
	files, err := manager.managedFiles(ctx, scope)
	if err != nil {
		return err
	}
	if files.SettingsPath == "" || len(files.Files) == 0 || !files.Files[0].Managed {
		return nil // systemctl handles absent and externally managed units.
	}
	request := InstallRequest{SettingsPath: files.SettingsPath, Now: true}
	if _, err := os.Lstat(files.SettingsPath); errors.Is(err, os.ErrNotExist) {
		// When both settings and .location were lost, the exact generated unit
		// still identifies its working directory. Verify the entire unit below
		// before trusting this hint to create anything.
		unit, err := os.ReadFile(manager.unitPath(scope))
		if err != nil {
			return err
		}
		for _, line := range strings.Split(string(unit), "\n") {
			if !strings.HasPrefix(line, "WorkingDirectory=") {
				continue
			}
			if request.DataDir != "" {
				return errors.New("ambiguous service working directory")
			}
			value := strings.TrimPrefix(line, "WorkingDirectory=")
			if strings.Contains(strings.ReplaceAll(value, "%%", ""), "%") {
				return errors.New("unresolved working-directory specifier")
			}
			request.DataDir = strings.ReplaceAll(value, "%%", "%")
		}
		if request.DataDir == "" {
			return errors.New("service does not identify its data directory")
		}
	}
	request, err = manager.resolveInstallSettings(scope, request)
	if err != nil {
		return err
	}
	_, settingsErr := os.Lstat(request.SettingsPath)
	_, dataErr := os.Lstat(request.DataDir)
	if settingsErr == nil && dataErr == nil {
		return nil
	}
	current, err := manager.queryStatus(ctx, scope)
	if err != nil {
		return err
	}
	if current.NeedDaemonReload || current.UnitFileSettingsPath != files.SettingsPath || current.UnitPath != manager.unitPath(scope) {
		return errors.New("resource preparation requires an unmodified, reloaded managed service; reinstall explicitly")
	}
	executable, _, _, err := manager.validateInstallPaths(scope, request)
	if err != nil {
		return err
	}
	unit, err := renderUnit(scope, executable, request.SettingsPath, request.DataDir)
	if err != nil {
		return err
	}
	if err := preflightInstall(manager.installFiles(scope, unit, request.DataDir), false); err != nil {
		return err
	}
	if err := manager.preflightCommands(ctx, scope, true); err != nil {
		return err
	}
	if _, err := prepareResources(ctx, request); err != nil {
		return err
	}
	if scope == ScopeSystem {
		return manager.prepareSystemOwnership(ctx, request.SettingsPath, request.DataDir)
	}
	return nil
}
