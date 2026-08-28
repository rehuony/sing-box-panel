// SPDX-License-Identifier: GPL-3.0-or-later

package systemd

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

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
