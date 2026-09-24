// SPDX-License-Identifier: GPL-3.0-or-later

// Package installation inspects and removes the persistent files of one panel instance.
package installation

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/rehuony/sing-box-panel/internal/panelprocess"
	"github.com/rehuony/sing-box-panel/internal/settings"
	"github.com/rehuony/sing-box-panel/internal/store"
)

// Entry reports a selected instance path without reading secrets into output.
type Entry struct {
	Path    string `json:"path"`
	Role    string `json:"role"`
	State   string `json:"state"`
	Bytes   int64  `json:"bytes"`
	Cleanup string `json:"cleanup"`
	Empty   *bool  `json:"empty,omitempty"`
}

type Report struct {
	SettingsPath string `json:"settings_path"`
	// DataDir is empty when the selected settings file is missing.
	DataDir          string          `json:"data_dir"`
	DatabaseIdentity string          `json:"database_identity"`
	Entries          []Entry         `json:"entries"`
	HistoryPath      string          `json:"history_path,omitempty"`
	History          *CleanupHistory `json:"history,omitempty"`
	Warnings         []string        `json:"warnings,omitempty"`
}

// Known names provide descriptive roles; every entry inside DataDir is in scope.
var knownDataPaths = []struct{ name, role string }{
	{"artifacts", "installed core files"},
	{"runtime", "runtime configuration and process files"},
	{"imports", "staged core uploads"},
	{"logs/core", "core logs"},
	{"panel-control.sock", "local process control"},
	{"panel.db-shm", "database shared memory"},
	{"panel.db-wal", "database write-ahead log"},
	{"panel.db", "database (configuration, panel logs and product state)"},
	{panelprocess.LeaseFileName, "runtime ownership lock"},
}

// Inspect never initializes settings or databases and never runs migrations.
func Inspect(ctx context.Context, settingsPath string) (Report, error) {
	if err := ctx.Err(); err != nil {
		return Report{}, err
	}
	abs, err := filepath.Abs(settingsPath)
	if err != nil {
		return Report{}, err
	}
	report := Report{SettingsPath: abs, Entries: []Entry{}, DatabaseIdentity: "unknown"}
	executable, err := os.Executable()
	if err != nil {
		return report, err
	}
	executableEntry, err := inspectEntry(executable, "panel executable", "retain")
	if err != nil {
		return report, err
	}
	report.Entries = append(report.Entries, executableEntry)
	settingsEntry, err := inspectEntry(abs, "panel settings", "remove")
	if err != nil {
		settingsEntry.State = "unreadable"
	}
	report.Entries = append(report.Entries, settingsEntry)
	if err != nil {
		return report, err
	}
	for _, sidecar := range []struct{ suffix, role string }{{".lock", "settings write lock"}, {".pending", "settings recovery journal"}, {".location", "data location and migration state"}} {
		entry, err := inspectEntry(abs+sidecar.suffix, sidecar.role, "remove")
		if err != nil {
			return report, err
		}
		if entry.State != "missing" {
			report.Entries = append(report.Entries, entry)
		}
	}
	report.DataDir, err = selectedDataDir(abs)
	if err != nil {
		return report, err
	}
	dataDir := report.DataDir
	if dataDir != "" {
		report.DatabaseIdentity = "missing"
	}

	if dataDir != "" {
		entry, err := inspectEntry(dataDir, "instance data directory", "remove")
		if err != nil {
			return report, err
		}
		report.Entries = append(report.Entries, entry)

	}
	settingsDir := filepath.Dir(abs)
	if filepath.Base(settingsDir) == "sing-box-panel" && (dataDir == "" || !pathWithin(dataDir, settingsDir)) {
		entry, err := inspectEntry(settingsDir, "settings directory", "remove_if_empty")
		if err != nil {
			return report, err
		}
		if entry.State != "directory" {
			entry.Cleanup = "retain"
		}
		report.Entries = append(report.Entries, entry)
		if entry.State == "directory" {
			children, err := os.ReadDir(settingsDir)
			if err != nil {
				return report, err
			}
			for _, child := range children {
				path := filepath.Join(settingsDir, child.Name())
				if path == abs || path == abs+".lock" || path == abs+".location" || path == abs+".pending" || path == dataDir {
					continue
				}
				item, err := inspectEntry(path, "other configuration directory content", "retain")
				if err != nil {
					return report, err
				}
				report.Entries = append(report.Entries, item)
			}
		}
	}
	// Without settings or a location record, do not guess a data directory.
	// Return the paths we can inspect without guessing or initializing storage.
	if dataDir == "" {
		return report, nil
	}
	dataInfo, err := os.Lstat(dataDir)
	if errors.Is(err, os.ErrNotExist) {
		return report, nil
	}
	if err != nil {
		return report, err
	}
	if !dataInfo.IsDir() || dataInfo.Mode()&os.ModeSymlink != 0 {
		return report, errors.New("data directory must be a physical directory")
	}
	root, err := os.OpenRoot(dataDir)
	if err != nil {
		return report, err
	}
	defer root.Close()
	report.DatabaseIdentity, err = databaseIdentity(ctx, root)
	if err != nil {
		return report, err
	}
	err = fs.WalkDir(root.FS(), ".", func(name string, child fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if name == "." {
			return nil
		}
		path := filepath.Join(dataDir, name)
		// The selected settings already have their own report entry.
		if path == abs {
			return nil
		}
		role := "instance data"
		for _, item := range knownDataPaths {
			if name == item.name || strings.HasPrefix(name, item.name+"/") {
				role = item.role
				break
			}
		}
		entry, err := inspectEntry(path, role, "remove")
		if err != nil {
			return err
		}
		report.Entries = append(report.Entries, entry)
		return nil
	})
	if err != nil {
		return report, err
	}
	return report, nil
}

func inspectEntry(path, role, cleanup string) (Entry, error) {
	entry := Entry{Path: path, Role: role, Cleanup: cleanup}
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		entry.State = "missing"
		return entry, nil
	}
	if err != nil {
		return entry, fmt.Errorf("inspect %s: %w", path, err)
	}
	entry.State = fileState(info)
	entry.Bytes = info.Size()
	if info.IsDir() {
		directory, err := os.Open(path)
		if err != nil {
			return entry, err
		}
		_, readErr := directory.ReadDir(1)
		_ = directory.Close()
		if readErr != nil && !errors.Is(readErr, io.EOF) {
			return entry, readErr
		}
		empty := errors.Is(readErr, io.EOF)
		entry.Empty = &empty
	}
	if entry.State == "symlink" && cleanup != "retain" {
		entry.Cleanup = "remove_link"
	}
	return entry, nil
}

func fileState(info fs.FileInfo) string {
	switch {
	case info.Mode().IsRegular():
		return "file"
	case info.IsDir():
		return "directory"
	case info.Mode()&os.ModeSymlink != 0:
		return "symlink"
	case info.Mode()&os.ModeSocket != 0:
		return "socket"
	default:
		return "special"
	}
}

func databaseIdentity(ctx context.Context, root *os.Root) (string, error) {
	info, err := root.Lstat("panel.db")
	if errors.Is(err, os.ErrNotExist) {
		return "missing", nil
	}
	if err != nil {
		return "unknown", err
	}
	if !info.Mode().IsRegular() {
		return "unrecognized", nil
	}
	// Include committed WAL identity without initializing a database. Incomplete
	// recovery state remains unknown; identity is diagnostic, not cleanup permission.
	id, err := store.ReadApplicationID(ctx, filepath.Join(root.Name(), "panel.db"))
	if err != nil {
		if ctx.Err() != nil {
			return "unknown", ctx.Err()
		}
		return "unknown", nil
	}
	if id == store.ApplicationID {
		return "sing-box-panel", nil
	}

	return "unrecognized", nil
}

// ValidateCleanup limits complete removal to the selected instance directory.
// Content identity is irrelevant; shared paths, linked roots and the executable
// remain protected even when the user confirms removal of all instance data.
func ValidateCleanup(report Report) error {
	if report.DataDir == "" {
		for _, entry := range report.Entries {
			if entry.Role == "panel settings" && entry.State != "missing" {
				return errors.New("cannot determine the data directory for cleanup")
			}
		}
		return nil
	}
	if !filepath.IsAbs(report.DataDir) || strings.TrimSpace(report.SettingsPath) == "" {
		return errors.New("absolute data directory and settings path are required")
	}
	if len(report.Entries) == 0 {
		return errors.New("instance inventory is required")
	}
	dataDir, err := cleanupPath(report.DataDir)
	if err != nil {
		return err
	}
	// Migration also uses this boundary, without an inventory from df. Protect
	// the fixed history location even when the caller has no history to report.
	historyPath := report.HistoryPath
	if historyPath == "" {
		historyPath = DefaultHistoryPath()
	}
	contains, err := DataDirectoryContainsPath(dataDir, filepath.Dir(historyPath))
	if err != nil {
		return err
	}
	if contains {
		return errors.New("data directory contains the retained cleanup history directory; keep history outside the data directory before cleanup or migration")
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
		resolved, err := cleanupPath(shared)
		if err != nil {
			return err
		}
		if pathWithin(dataDir, resolved) {
			return errors.New("refusing to clean a filesystem root, home or shared system directory")
		}
	}
	executable, err := cleanupPath(report.Entries[0].Path)
	if err != nil {
		return err
	}
	if pathWithin(dataDir, executable) {
		return errors.New("panel executable is inside the data directory; move it outside before cleanup")
	}
	for _, entry := range report.Entries {
		switch entry.Role {
		case "panel settings":
			if entry.State != "file" && entry.State != "missing" {
				return errors.New("settings path must be a regular file")
			}
		case "instance data directory":
			if entry.State != "directory" && entry.State != "missing" {
				return errors.New("data directory must be a physical directory")
			}
		}
	}
	return nil
}

func selectedDataDir(path string) (string, error) {
	if info, err := os.Lstat(path); err == nil {
		if !info.Mode().IsRegular() {
			return "", errors.New("settings path must be a regular file")
		}
		return settings.LoadDataDir(path)
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	if err := settings.CheckPending(path); err != nil {
		return "", err
	}
	location, err := settings.ReadDataLocation(path)
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	if location.Move != nil {
		return "", errors.New("restore settings to finish the data directory migration before cleanup")
	}
	return location.DataDir, nil
}

func pathWithin(directory, path string) bool {
	relative, err := filepath.Rel(directory, path)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

// DataDirectoriesOverlap identifies shared or nested service storage, including
// aliases and directories that have not been created yet.
func DataDirectoriesOverlap(first, second string) (bool, error) {
	first, err := cleanupPath(first)
	if err != nil {
		return false, err
	}
	second, err = cleanupPath(second)
	if err != nil {
		return false, err
	}
	return pathWithin(first, second) || pathWithin(second, first), nil
}

// DataDirectoryContainsPath also checks parent aliases: deleting a symlink in
// the data directory can break an external service path whose target is outside.
func DataDirectoryContainsPath(directory, path string) (bool, error) {
	directory, err := cleanupPath(directory)
	if err != nil {
		return false, err
	}
	path, err = filepath.Abs(path)
	if err != nil {
		return false, err
	}
	for {
		resolved, err := cleanupPath(path)
		if err != nil {
			return false, err
		}
		if pathWithin(directory, path) || pathWithin(directory, resolved) {
			return true, nil
		}
		parent := filepath.Dir(path)
		if parent == path {
			return false, nil
		}
		path = parent
	}
}

// Resolve existing ancestors too, so aliases cannot evade shared-path checks
// when the selected directory has not been created yet.
func cleanupPath(path string) (string, error) {
	path = filepath.Clean(path)
	resolved, err := filepath.EvalSymlinks(path)
	if err == nil || !errors.Is(err, os.ErrNotExist) {
		return resolved, err
	}
	parent := filepath.Dir(path)
	if parent == path {
		return path, nil
	}
	resolved, err = cleanupPath(parent)
	if err != nil {
		return "", err
	}
	return filepath.Join(resolved, filepath.Base(path)), nil
}
