// SPDX-License-Identifier: GPL-3.0-or-later

package installation

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/rehuony/sing-box-panel/internal/panelprocess"
	"github.com/rehuony/sing-box-panel/internal/settings"
	"github.com/rehuony/sing-box-panel/internal/store"
)

type CleanupResult struct {
	Removed  []string `json:"removed"`
	Retained []string `json:"retained"`
}

// Clean removes the selected settings and every entry in its data directory,
// never arbitrary paths from a report. The caller must first stop the instance.
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
	settingsLock, err := settings.TryLock(ctx, current.SettingsPath)
	if err != nil {
		return result, err
	}
	defer settingsLock.Close()
	if location, err := settings.ReadDataLocation(current.SettingsPath); err == nil && location.Move != nil {
		return result, errors.New("finish data directory migration before cleanup")
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
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
		if err := settingsRoot.Remove(settingsName + ".lock"); err != nil {
			return result, err
		}
		result.Removed = append(result.Removed, current.SettingsPath+".lock")
		if err := settingsRoot.Remove(settingsName + ".location"); err == nil {
			result.Removed = append(result.Removed, current.SettingsPath+".location")
		} else if !errors.Is(err, os.ErrNotExist) {
			return result, err
		}
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
	// Revalidate selection under the same locks used by the server and CLI.
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
	lockInfo, err := dataLock.Stat()
	if err != nil || !os.SameFile(rootInfo, lockInfo) {
		return result, errors.New("data directory changed while acquiring cleanup locks")
	}
	if err := ctx.Err(); err != nil {
		return result, err
	}
	// Keep the settings (including a containing subdirectory) and database until
	// other entries are gone, so an earlier failure remains diagnosable.
	resolvedSettings, err := cleanupPath(current.SettingsPath)
	if err != nil {
		return result, err
	}
	resolvedData, err := cleanupPath(current.DataDir)
	if err != nil {
		return result, err
	}
	settingsEntry := ""
	settingsRelative := ""
	if pathWithin(resolvedData, resolvedSettings) {
		settingsRelative, _ = filepath.Rel(resolvedData, resolvedSettings)
		settingsEntry = strings.Split(settingsRelative, string(filepath.Separator))[0]
	}
	tree := cleanupTree{ctx: ctx, dataDir: current.DataDir, settings: settingsRelative, result: &result}
	defer func() { cleanErr = errors.Join(cleanErr, tree.Close()) }()
	children, err := fs.ReadDir(root.FS(), ".")
	if err != nil {
		return result, err
	}
	for _, child := range children {
		name := child.Name()
		if name != settingsEntry && (name == panelprocess.LeaseFileName || name == "panel.db" || name == "panel.db-wal" || name == "panel.db-shm") {
			continue
		}
		if err := tree.remove(root, name, name); err != nil {
			return result, err
		}
	}
	for _, name := range []string{"panel.db-shm", "panel.db-wal", "panel.db"} {
		if name != settingsEntry {
			if err := tree.remove(root, name, name); err != nil {
				return result, err
			}
		}
	}
	if err := ctx.Err(); err != nil {
		return result, err
	}
	latest, err = settingsRoot.Lstat(settingsName)
	if err != nil || !os.SameFile(settingsInfo, latest) {
		return result, errors.New("settings file was replaced before removal")
	}
	if err := settingsRoot.Remove(settingsName); err == nil {
		result.Removed = append(result.Removed, current.SettingsPath)
	} else if !errors.Is(err, os.ErrNotExist) {
		return result, err
	}
	if err := settingsRoot.Remove(settingsName + ".lock"); err != nil {
		return result, err
	}
	result.Removed = append(result.Removed, current.SettingsPath+".lock")
	if err := settingsRoot.Remove(settingsName + ".location"); err == nil {
		result.Removed = append(result.Removed, current.SettingsPath+".location")
	} else if !errors.Is(err, os.ErrNotExist) {
		return result, err
	}
	if err := tree.removeSettingsDirectories(); err != nil {
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
	latestDir, err := parent.Lstat(filepath.Base(current.DataDir))
	if err != nil || !os.SameFile(rootInfo, latestDir) {
		return result, errors.New("data directory changed before final removal")
	}
	if err := parent.Remove(filepath.Base(current.DataDir)); err == nil {
		result.Removed = append(result.Removed, current.DataDir)
	} else {
		result.Retained = append(result.Retained, current.DataDir)
		return result, fmt.Errorf("remove data directory (new or inaccessible entries may remain): %w", err)
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
