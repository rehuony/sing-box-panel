// SPDX-License-Identifier: GPL-3.0-or-later

package systemd

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/rehuony/sing-box-panel/internal/settings"
)

// A requested start/restart prepares a data move outside the service sandbox.
// Only byte-for-byte generated units may be rewritten automatically; custom
// units and drop-ins retain their existing explicit install/force workflow.
func (manager *Manager) prepareDataMove(ctx context.Context, scope Scope, action Action) error {
	if action == ActionStop {
		return nil
	}
	files, err := manager.Files(ctx, scope)
	if err != nil {
		return err
	}
	if files.SettingsPath == "" {
		return nil
	}
	location, err := settings.ReadDataLocation(files.SettingsPath)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	target, err := settings.ConfiguredDataDir(files.SettingsPath)
	if err != nil {
		return err
	}
	if location.DataDir == target && location.Move == nil {
		return nil
	}
	current, err := manager.Status(ctx, scope)
	if err != nil {
		return err
	}
	if current.UnitPath != manager.unitPath(scope) || current.UnitFileSettingsPath != files.SettingsPath || current.NeedDaemonReload {
		return errors.New("data migration requires an unmodified, reloaded managed service unit")
	}
	if action == ActionStart && current.ActiveState == "active" {
		return errors.New("the service is running; restart it explicitly to move its data directory")
	}
	executable, err := manager.executable()
	if err != nil {
		return err
	}
	oldUnit, err := renderUnit(scope, executable, files.SettingsPath, location.DataDir)
	if err != nil {
		return err
	}
	if err := preflightInstall(manager.installFiles(scope, oldUnit, location.DataDir), false); err != nil {
		return fmt.Errorf("data migration cannot rewrite customized service files; stop the service and reinstall it explicitly: %w", err)
	}
	// Validate before stopping; migration itself still proves all processes and
	// database owners have released the source before touching data.
	if _, err := settings.Load(files.SettingsPath); err != nil {
		return err
	}
	if _, _, _, err := manager.validateInstallPaths(scope, InstallRequest{SettingsPath: files.SettingsPath, DataDir: target}); err != nil {
		return err
	}
	if action == ActionRestart {
		if err := manager.runSystemctl(ctx, scope, "stop", UnitName); err != nil {
			return err
		}
	}
	_, err = manager.Install(ctx, InstallRequest{Scope: scope, SettingsPath: files.SettingsPath, DataDir: target, Force: true})
	return err
}
