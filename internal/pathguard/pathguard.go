// SPDX-License-Identifier: GPL-3.0-or-later

// Package pathguard decides whether a service directory is safe to own,
// clean, move or bind into a sandbox. It resolves symbolic links, including
// those of directories that do not exist yet, so aliases cannot evade a check.
package pathguard

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// Resolve returns path with its existing ancestors' symbolic links resolved;
// the missing remainder is joined unchanged.
func Resolve(path string) (string, error) {
	path = filepath.Clean(path)
	resolved, err := filepath.EvalSymlinks(path)
	if err == nil || !errors.Is(err, os.ErrNotExist) {
		return resolved, err
	}
	parent := filepath.Dir(path)
	if parent == path {
		return path, nil
	}
	resolved, err = Resolve(parent)
	if err != nil {
		return "", err
	}
	return filepath.Join(resolved, filepath.Base(path)), nil
}

// Within reports whether path is directory or inside it, lexically.
func Within(directory, path string) bool {
	relative, err := filepath.Rel(directory, path)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

// Overlap reports whether two directories are the same or nested, including
// aliases and directories that have not been created yet.
func Overlap(first, second string) (bool, error) {
	first, err := Resolve(first)
	if err != nil {
		return false, err
	}
	second, err = Resolve(second)
	if err != nil {
		return false, err
	}
	return Within(first, second) || Within(second, first), nil
}

// Contains reports whether path, or one of its parents, lies inside
// directory. Parent aliases count: deleting a symbolic link inside directory
// can break a path whose target is outside it.
func Contains(directory, path string) (bool, error) {
	directory, err := Resolve(directory)
	if err != nil {
		return false, err
	}
	path, err = filepath.Abs(path)
	if err != nil {
		return false, err
	}
	for {
		resolved, err := Resolve(path)
		if err != nil {
			return false, err
		}
		if Within(directory, path) || Within(directory, resolved) {
			return true, nil
		}
		parent := filepath.Dir(path)
		if parent == path {
			return false, nil
		}
		path = parent
	}
}

// ErrShared marks a directory that is, or contains, a filesystem root, a
// home or a shared system directory.
var ErrShared = errors.New("a filesystem root, home or shared system directory")

// RejectShared returns ErrShared for a service directory that is, or
// contains, a filesystem root, a home or a shared system directory.
func RejectShared(directory string) error {
	resolved, err := Resolve(directory)
	if err != nil {
		return err
	}
	home, _ := os.UserHomeDir()
	configHome, _ := os.UserConfigDir()
	cacheHome, _ := os.UserCacheDir()
	dataHome := os.Getenv("XDG_DATA_HOME")
	if !filepath.IsAbs(dataHome) && home != "" {
		dataHome = filepath.Join(home, ".local", "share")
	}
	for _, shared := range []string{"/", "/etc", "/usr", "/usr/local", "/var", "/var/lib", "/opt", "/srv", "/home", "/Users", "/run", home, configHome, cacheHome, dataHome, os.TempDir()} {
		if shared == "" {
			continue
		}
		sharedPath, err := Resolve(shared)
		if err != nil {
			return err
		}
		if Within(resolved, sharedPath) {
			return ErrShared
		}
	}
	return nil
}
