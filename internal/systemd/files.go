// SPDX-License-Identifier: GPL-3.0-or-later

package systemd

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// Files inspects the installer's exact destinations without contacting systemd.
func (manager *Manager) Files(ctx context.Context, requested Scope) (FilesResult, error) {
	result, err := manager.managedFiles(ctx, requested)
	if err != nil {
		return result, err
	}
	if err := manager.relatedFiles(ctx, result.Scope, &result); err != nil {
		return result, err
	}
	return result, nil
}

// Service preparation needs only exact installer destinations. Keep the broader
// read-only inventory out of start/restart so unrelated directories cannot block
// control; loaded fragments and drop-ins are verified separately before writes.
func (manager *Manager) managedFiles(ctx context.Context, requested Scope) (FilesResult, error) {
	if err := ctx.Err(); err != nil {
		return FilesResult{}, err
	}
	if err := manager.requireLinux(); err != nil {
		return FilesResult{}, err
	}
	scope, err := manager.resolveScope(requested)
	if err != nil {
		return FilesResult{}, err
	}
	result := FilesResult{Scope: scope, Files: []FileStatus{}}
	for _, path := range manager.managedPaths(scope) {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		item := FileStatus{Path: path, State: "missing"}
		info, err := os.Lstat(path)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return result, err
		}
		if err == nil {
			item.State = "unmanaged"
			if info.Mode().IsRegular() {
				data, err := os.ReadFile(path)
				if err != nil {
					return result, err
				}
				item.Managed = bytes.Contains(data, []byte(managedMark))
				if item.Managed {
					item.State = "managed"
				}
				if path == manager.unitPath(scope) {
					result.SettingsPath, _ = parseUnitFileSettingsPath(data)
				}
			}
		}
		result.Files = append(result.Files, item)
	}
	if result.SettingsPath == "" && scope == ScopeSystem && result.Files[0].State == "missing" {
		// System scope has one fixed configuration path. Auxiliary files remain
		// discoverable after a failed installation or a manually removed unit.
		for _, item := range result.Files[1:] {
			if item.Managed {
				result.SettingsPath = manager.layout.SystemSettingsPath
			}
		}
	}
	return result, nil
}

// Related artifacts are visible but never acquire removal authority merely by
// appearing in the inventory. systemctl owns enablement links and runtime dirs.
func (manager *Manager) relatedFiles(ctx context.Context, scope Scope, result *FilesResult) error {
	unit := manager.unitPath(scope)
	bases := []string{filepath.Dir(unit)}
	if scope == ScopeSystem && unit == "/etc/systemd/system/"+UnitName {
		bases = append(bases, "/run/systemd/system", "/usr/local/lib/systemd/system", "/usr/lib/systemd/system")
	} else if scope == ScopeUser {
		configHome, _ := os.UserConfigDir()
		if unit == filepath.Join(configHome, "systemd", "user", UnitName) {
			bases = append(bases, "/etc/systemd/user", "/usr/local/lib/systemd/user", "/usr/lib/systemd/user")
			if runtimeHome := os.Getenv("XDG_RUNTIME_DIR"); filepath.IsAbs(runtimeHome) {
				bases = append(bases, filepath.Join(runtimeHome, "systemd", "user"))
			}
		}
	}
	slices.Sort(bases)
	for _, base := range slices.Compact(bases) {
		if err := relatedFilesIn(ctx, unit, base, result); err != nil {
			return err
		}
	}
	if scope == ScopeSystem && unit == "/etc/systemd/system/"+UnitName {
		path := "/run/sing-box-panel"
		if info, err := os.Lstat(path); err == nil {
			state := "runtime directory"
			if !info.IsDir() {
				state = "runtime path"
			}
			result.Files = append(result.Files, FileStatus{Path: path, State: state, Retained: true})
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return nil
}

func relatedFilesIn(ctx context.Context, unit, base string, result *FilesResult) error {
	children, err := os.ReadDir(base)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	for _, child := range children {
		if err := ctx.Err(); err != nil {
			return err
		}
		if child.Name() == UnitName+".d" && child.Type()&os.ModeSymlink != 0 {
			result.Files = append(result.Files, FileStatus{Path: filepath.Join(base, child.Name()), State: "linked drop-in directory", Retained: true})
			continue
		}
		if !child.IsDir() {
			continue
		}
		directory := filepath.Join(base, child.Name())
		if child.Name() == UnitName+".d" {
			entries, err := os.ReadDir(directory)
			if err != nil {
				return err
			}
			result.Files = append(result.Files, FileStatus{Path: directory, State: "directory", Retained: true})
			for _, entry := range entries {
				result.Files = append(result.Files, FileStatus{Path: filepath.Join(directory, entry.Name()), State: "custom", Retained: true})
			}
		} else if strings.HasSuffix(child.Name(), ".wants") || strings.HasSuffix(child.Name(), ".requires") {
			path := filepath.Join(directory, UnitName)
			target, err := os.Readlink(path)
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			if err != nil {
				if info, e := os.Lstat(path); e == nil && info.Mode()&os.ModeSymlink == 0 {
					continue
				}
				return err
			}
			if !filepath.IsAbs(target) {
				target = filepath.Join(directory, target)
			}
			if filepath.Clean(target) == unit {
				result.Files = append(result.Files, FileStatus{Path: path, State: "enablement link", Retained: true})
			}
		}
	}
	return nil
}

type managedFile struct {
	path string
	data []byte
}

func preflightInstall(files []managedFile, force bool) error {
	for _, file := range files {
		info, err := os.Lstat(file.path)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return fmt.Errorf("inspect managed destination %q: %w", file.path, err)
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("%w: destination %q is not a regular file", ErrConflict, file.path)
		}
		existing, err := os.ReadFile(file.path)
		if err != nil {
			return fmt.Errorf("read managed destination %q: %w", file.path, err)
		}
		if !bytes.Equal(existing, file.data) && !force {
			return fmt.Errorf("%w: %q; pass --force to replace it", ErrConflict, file.path)
		}
	}
	return nil
}

func preflightUninstall(paths []string, force bool) ([]string, error) {
	existing := make([]string, 0, len(paths))
	for _, path := range paths {
		info, err := os.Lstat(path)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("inspect managed destination %q: %w", path, err)
		}
		if !info.Mode().IsRegular() {
			return nil, fmt.Errorf("%w: destination %q is not a regular file", ErrConflict, path)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read managed destination %q: %w", path, err)
		}
		if !bytes.Contains(data, []byte(managedMark)) && !force {
			return nil, fmt.Errorf("%w: refusing to remove unmanaged file %q; pass --force to remove it", ErrConflict, path)
		}
		existing = append(existing, path)
	}
	return existing, nil
}

func installFile(path string, data []byte) (bool, error) {
	if info, err := os.Lstat(path); err == nil {
		if !info.Mode().IsRegular() {
			return false, fmt.Errorf("%w: destination %q is not a regular file", ErrConflict, path)
		}
		existing, err := os.ReadFile(path)
		if err != nil {
			return false, fmt.Errorf("read managed destination %q: %w", path, err)
		}
		if bytes.Equal(existing, data) {
			if info.Mode().Perm() == 0o644 {
				return false, nil
			}
			if err := os.Chmod(path, 0o644); err != nil {
				return false, fmt.Errorf("set managed-file permissions %q: %w", path, err)
			}
			if err := syncDirectory(filepath.Dir(path)); err != nil {
				return false, err
			}
			return true, nil
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return false, fmt.Errorf("inspect managed destination %q: %w", path, err)
	}
	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return false, fmt.Errorf("create systemd directory %q: %w", directory, err)
	}
	temporary, err := os.CreateTemp(directory, ".sing-box-panel-*.tmp")
	if err != nil {
		return false, fmt.Errorf("create temporary managed file in %q: %w", directory, err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o644); err != nil {
		temporary.Close()
		return false, fmt.Errorf("set temporary managed-file permissions: %w", err)
	}
	if _, err := temporary.Write(data); err != nil {
		temporary.Close()
		return false, fmt.Errorf("write temporary managed file: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return false, fmt.Errorf("sync temporary managed file: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return false, fmt.Errorf("close temporary managed file: %w", err)
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return false, fmt.Errorf("replace managed file %q: %w", path, err)
	}
	if err := syncDirectory(directory); err != nil {
		return false, err
	}
	return true, nil
}

func syncDirectory(path string) error {
	directory, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open managed directory %q: %w", path, err)
	}
	defer directory.Close()
	if err := directory.Sync(); err != nil {
		return fmt.Errorf("sync managed directory %q: %w", path, err)
	}
	return nil
}

func parseProperties(data []byte) (map[string]string, error) {
	properties := make(map[string]string)
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		key, value, found := strings.Cut(line, "=")
		if !found || key == "" {
			return nil, fmt.Errorf("%w: malformed systemctl property %q", ErrInvalid, line)
		}
		properties[key] = value
	}
	for _, required := range []string{"LoadState", "ActiveState", "SubState", "UnitFileState", "MainPID", "FragmentPath"} {
		if _, found := properties[required]; !found {
			return nil, fmt.Errorf("%w: systemctl omitted %s", ErrInvalid, required)
		}
	}
	return properties, nil
}

func fileExists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}
