// SPDX-License-Identifier: GPL-3.0-or-later

package systemd

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/rehuony/sing-box-panel/internal/atomicfile"
)

func (manager *Manager[T]) Install(ctx context.Context, request InstallRequest) (result InstallResult[T], installErr error) {
	if err := manager.requireLinux(); err != nil {
		return InstallResult[T]{}, err
	}
	scope, err := manager.resolveScope(request.Scope)
	if err != nil {
		return InstallResult[T]{}, err
	}
	if err := manager.requireMutationPermission(scope); err != nil {
		return InstallResult[T]{}, err
	}
	request, err = manager.resolveInstall(scope, request)
	if err != nil {
		return InstallResult[T]{}, err
	}
	executablePath, statePath, dataDir, err := manager.validateInstallPaths(scope, request)
	if err != nil {
		return InstallResult[T]{}, err
	}

	unit, err := manager.Render(scope, executablePath, statePath, dataDir)
	if err != nil {
		return InstallResult[T]{}, err
	}
	files, err := manager.installFiles(scope, unit, dataDir)
	if err != nil {
		return InstallResult[T]{}, err
	}
	if err := preflightInstall(files, request.Force, manager.managedMark()); err != nil {
		return InstallResult[T]{}, err
	}
	for _, path := range append([]string{statePath}, manager.managedPaths(scope)...) {
		if err := writableParent(path); err != nil {
			return InstallResult[T]{}, err
		}
	}
	if err := writableDirectory(dataDir); err != nil {
		return InstallResult[T]{}, err
	}
	if err := manager.preflightCommands(ctx, scope, true); err != nil {
		return InstallResult[T]{}, err
	}
	if scope == ScopeSystem {
		if _, err := manager.inspectSystemAccount(ctx); err != nil {
			return InstallResult[T]{}, err
		}
	}
	if err := manager.definition.State.Relocate(ctx, statePath, dataDir); err != nil {
		return InstallResult[T]{}, err
	}
	initial, err := manager.prepareResources(ctx, request)
	defer func() { result.Initial, result.StateCreated = initial, initial != nil }()
	if err != nil {
		return InstallResult[T]{}, err
	}
	installed := make([]string, 0, len(files))
	for _, file := range files {
		changed, writeErr := installFile(file.path, file.data)
		if writeErr != nil {
			return InstallResult[T]{}, writeErr
		}
		if changed {
			installed = append(installed, file.path)
		}
	}

	if scope == ScopeSystem {
		if err := manager.prepareSystemOwnership(ctx, statePath, dataDir); err != nil {
			return InstallResult[T]{}, err
		}
	}
	retired, err := manager.removeRetired(ctx, scope)
	installed = append(installed, retired...)
	if err != nil {
		return InstallResult[T]{}, err
	}

	if err := manager.runSystemctl(ctx, scope, "daemon-reload"); err != nil {
		return InstallResult[T]{}, err
	}
	enableArgs := []string{"enable"}
	if request.Now {
		enableArgs = append(enableArgs, "--now")
	}
	enableArgs = append(enableArgs, manager.unitName)
	if err := manager.runSystemctl(ctx, scope, enableArgs...); err != nil {
		return InstallResult[T]{}, err
	}

	return InstallResult[T]{
		Scope: scope, Unit: manager.unitName, UnitPath: manager.unitPath(scope), ExecutablePath: executablePath,
		StatePath: statePath, DataDir: dataDir, InstalledPaths: installed,
		Enabled: true, Started: request.Now, PersistentState: true,
	}, nil
}

func (manager *Manager[T]) Uninstall(ctx context.Context, request UninstallRequest) (result UninstallResult, uninstallErr error) {
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
	result = UninstallResult{Scope: scope, Unit: manager.unitName, RemovedPaths: []string{},
		ConfigRetained: true, DataRetained: true}
	paths := manager.managedPaths(scope)
	existing, err := preflightUninstall(paths, manager.managedMark(), request.Force)
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
	contents := make(map[string][]byte, len(existing))
	for _, path := range existing {
		info, err := os.Lstat(path)
		if err != nil {
			return result, err
		}
		identities[path] = info
		contents[path], err = os.ReadFile(path)
		if err != nil {
			return result, err
		}
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
	retired, err := manager.removeRetired(ctx, scope)
	result.RemovedPaths = append(result.RemovedPaths, retired...)
	if err != nil {
		return result, err
	}
	if !unitStopped(status) {
		if status.UnitPath != manager.unitPath(scope) {
			return result, fmt.Errorf("%w: cannot establish running service ownership", ErrConflict)
		}
		if err := manager.runSystemctl(ctx, scope, "stop", manager.unitName); err != nil {
			return result, err
		}
		status, err = manager.queryStatus(ctx, scope)
		if err != nil {
			return result, err
		}
		if !unitStopped(status) {
			return result, errors.New("service has not stopped; retained managed files")
		}
		if account != (systemAccount{}) {
			if err := manager.validateAccountRemovalStatus(status); err != nil {
				return result, err
			}
		}
	}
	result.Stopped = true
	if fileExists(manager.unitPath(scope)) || len(links) > 0 {
		args := []string{"disable", "--no-reload", manager.unitName}
		if account != (systemAccount{}) {
			// Account removal rechecks NeedDaemonReload. Let systemctl refresh
			// its own enablement changes before that check; stale configuration
			// present before disable is still rejected above.
			args = []string{"disable", manager.unitName}
		}
		disableErr := manager.runSystemctl(ctx, scope, args...)
		// Disable can remove links before its implicit reload fails.
		for _, path := range links {
			if _, err := os.Lstat(path); errors.Is(err, os.ErrNotExist) {
				result.RemovedPaths = append(result.RemovedPaths, path)
			}
		}
		if disableErr != nil {
			return result, disableErr
		}
		result.Disabled = true
	}
	if scope == ScopeSystem {
		// The reload must not hide a concurrent replacement or in-place edit.
		// Revalidate both identity and content before changing account ownership.
		for _, path := range existing {
			info, err := os.Lstat(path)
			if err != nil {
				return result, err
			}
			if !os.SameFile(identities[path], info) {
				return result, fmt.Errorf("service file changed during uninstall: %s", path)
			}
			content, err := os.ReadFile(path)
			if err != nil {
				return result, err
			}
			if !bytes.Equal(contents[path], content) {
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
		if err := atomicfile.SyncDir(filepath.Dir(path)); err != nil {
			return result, err
		}
	}
	if err := manager.runSystemctl(ctx, scope, "daemon-reload"); err != nil {
		return result, err
	}
	return result, nil
}

// removeRetired stops and removes the retired units beside the system unit
// that carry the installation's mark, in the order RetiredUnits lists them,
// and returns the paths it removed.
func (manager *Manager[T]) removeRetired(ctx context.Context, scope Scope) ([]string, error) {
	if scope != ScopeSystem {
		return nil, nil
	}
	var removed []string
	for _, name := range manager.definition.RetiredUnits {
		path := filepath.Join(filepath.Dir(manager.layout.SystemUnitPath), name)
		info, err := os.Lstat(path)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return removed, err
		}
		if !info.Mode().IsRegular() {
			continue
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return removed, err
		}
		if !bytes.Contains(data, []byte(manager.managedMark())) {
			continue
		}
		// A path unit is disabled so that it starts nothing more.
		args := []string{"stop", name}
		if strings.HasSuffix(name, ".path") {
			args = []string{"disable", "--now", name}
		}
		if err := manager.runSystemctl(ctx, scope, args...); err != nil {
			return removed, err
		}
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return removed, fmt.Errorf("remove retired unit %q: %w", path, err)
		}
		removed = append(removed, path)
	}
	if len(removed) != 0 {
		return removed, atomicfile.SyncDir(filepath.Dir(manager.layout.SystemUnitPath))
	}
	return removed, nil
}

func unitStopped(status Status) bool {
	return status.MainPID == 0 && (status.ActiveState == "inactive" || status.ActiveState == "failed")
}

func (manager *Manager[T]) prepareSystemOwnership(ctx context.Context, statePath, dataDir string) error {
	if _, err := manager.inspectSystemAccount(ctx); err != nil {
		return err
	}
	if err := manager.run(ctx, "systemd-sysusers", manager.layout.SystemSysusersPath); err != nil {
		return err
	}
	if err := manager.run(ctx, "systemd-tmpfiles", "--create", manager.layout.SystemTmpfilesPath); err != nil {
		return err
	}
	account := manager.definition.Name + ":" + manager.definition.Name
	for _, owned := range manager.definition.State.Owned(statePath) {
		info, err := os.Lstat(owned.Path)
		if errors.Is(err, os.ErrNotExist) && owned.Optional {
			continue
		}
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 || (owned.Directory && !info.IsDir()) || (!owned.Directory && !info.Mode().IsRegular()) {
			return fmt.Errorf("invalid %s path %q", manager.definition.StateLabel, owned.Path)
		}
		if err := manager.run(ctx, "chown", account, owned.Path); err != nil {
			return err
		}
		if err := os.Chmod(owned.Path, owned.Mode); err != nil {
			return fmt.Errorf("set %s permissions: %w", manager.definition.StateLabel, err)
		}
	}
	return manager.run(ctx, "chown", "--recursive", "--no-dereference", account, dataDir)
}
