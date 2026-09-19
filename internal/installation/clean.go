// SPDX-License-Identifier: GPL-3.0-or-later

package installation

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"

	"github.com/rehuony/sing-box-panel/internal/panelprocess"
	"github.com/rehuony/sing-box-panel/internal/settings"
	"github.com/rehuony/sing-box-panel/internal/store"
)

type CleanupResult struct {
	Removed  []string `json:"removed"`
	Retained []string `json:"retained"`
}

// Clean removes reserved instance paths, never arbitrary paths supplied in a
// manifest. The caller must first stop/uninstall the selected running instance.
func Clean(ctx context.Context, expected Report) (result CleanupResult, cleanErr error) {
	result = CleanupResult{Removed: []string{}, Retained: []string{}}
	current, err := Inspect(ctx, expected.SettingsPath)
	if err != nil {
		return result, err
	}
	if current.DataDir != expected.DataDir {
		return result, errors.New("settings changed data directory during cleanup; inspect again")
	}
	if err := ValidateCleanup(current); err != nil {
		return result, err
	}
	settingsRoot, err := os.OpenRoot(filepath.Dir(current.SettingsPath))
	if err != nil {
		return result, err
	}
	defer settingsRoot.Close()
	settingsName := filepath.Base(current.SettingsPath)
	settingsInfo, err := settingsRoot.Lstat(settingsName)
	if err != nil || !settingsInfo.Mode().IsRegular() {
		return result, errors.New("settings path must remain a regular file")
	}
	root, err := os.OpenRoot(current.DataDir)
	if errors.Is(err, os.ErrNotExist) {
		if err := settingsRoot.Remove(settingsName); err != nil {
			return result, err
		}
		result.Removed = append(result.Removed, current.SettingsPath)
		return result, nil
	}
	if err != nil {
		return result, err
	}
	defer root.Close()
	lease, err := panelprocess.AcquireLease(current.DataDir)
	if err != nil {
		return result, err
	}
	defer func() { cleanErr = errors.Join(cleanErr, lease.Close()) }()
	dataLock, err := store.LockDirectoryForCleanup(current.DataDir)
	if err != nil {
		return result, err
	}
	defer func() { cleanErr = errors.Join(cleanErr, dataLock.Close()) }()
	// Revalidate under the same ownership lease as server startup. SQLite is
	// only inspected read-only, so validation cannot migrate or create it.
	identity, err := databaseIdentity(ctx, root)
	if err != nil {
		return result, err
	}
	if identity != current.DatabaseIdentity {
		return result, errors.New("database changed during cleanup; inspect again")
	}
	dataDir, err := settings.LoadDataDir(current.SettingsPath)
	if err != nil || dataDir != current.DataDir {
		return result, errors.New("settings changed during cleanup; inspect again")
	}
	latest, err := settingsRoot.Lstat(settingsName)
	if err != nil || !os.SameFile(settingsInfo, latest) {
		return result, errors.New("settings file was replaced during cleanup; inspect again")
	}
	rootInfo, err := root.Stat(".")
	if err != nil {
		return result, err
	}
	currentInfo, err := os.Lstat(current.DataDir)
	if err != nil || !os.SameFile(rootInfo, currentInfo) {
		return result, errors.New("data directory changed during cleanup; inspect again")
	}
	if err := ctx.Err(); err != nil {
		return result, err
	}
	for _, item := range managed {
		name := item.name
		if name == panelprocess.LeaseFileName {
			continue
		}
		if err := ctx.Err(); err != nil {
			return result, err
		}
		if _, err := root.Lstat(name); errors.Is(err, os.ErrNotExist) {
			continue
		} else if err != nil {
			return result, err
		}
		if err := root.RemoveAll(name); err != nil {
			return result, fmt.Errorf("remove instance path %s: %w", name, err)
		}
		result.Removed = append(result.Removed, filepath.Join(current.DataDir, name))
	}
	if err := root.Remove("logs"); err == nil {
		result.Removed = append(result.Removed, filepath.Join(current.DataDir, "logs"))
	} else if !errors.Is(err, os.ErrNotExist) && !errors.Is(err, syscall.ENOTEMPTY) && !errors.Is(err, syscall.EEXIST) {
		return result, err
	}
	// Keep bootstrap settings and database until generated directories have
	// been removed, so an earlier filesystem failure remains diagnosable.
	if err := settingsRoot.Remove(settingsName); err == nil {
		result.Removed = append(result.Removed, current.SettingsPath)
	} else if !errors.Is(err, os.ErrNotExist) {
		return result, err
	}
	if err := root.Remove(panelprocess.LeaseFileName); err == nil {
		result.Removed = append(result.Removed, filepath.Join(current.DataDir, panelprocess.LeaseFileName))
	} else if !errors.Is(err, os.ErrNotExist) {
		return result, err
	}
	for _, entry := range current.Entries {
		if entry.Cleanup == "retain" && entry.State != "missing" {
			result.Retained = append(result.Retained, entry.Path)
		}
	}
	parent, err := os.OpenRoot(filepath.Dir(current.DataDir))
	if err != nil {
		return result, err
	}
	defer parent.Close()
	if err := parent.Remove(filepath.Base(current.DataDir)); err == nil {
		result.Removed = append(result.Removed, current.DataDir)
	} else if errors.Is(err, syscall.ENOTEMPTY) || errors.Is(err, syscall.EEXIST) {
		result.Retained = append(result.Retained, current.DataDir)
	} else {
		return result, err
	}
	for _, entry := range current.Entries {
		if entry.Role != "settings directory" || entry.Cleanup != "remove_if_empty" {
			continue
		}
		parent, err := os.OpenRoot(filepath.Dir(entry.Path))
		if err != nil {
			return result, err
		}
		info, err := parent.Lstat(filepath.Base(entry.Path))
		if err == nil && info.IsDir() && info.Mode()&os.ModeSymlink == 0 {
			if err := parent.Remove(filepath.Base(entry.Path)); err == nil {
				result.Removed = append(result.Removed, entry.Path)
			}
		}
		_ = parent.Close()
	}
	return result, nil
}
