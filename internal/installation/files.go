// SPDX-License-Identifier: GPL-3.0-or-later

// Package installation inspects and removes the persistent files of one panel instance.
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

// Entry reports a selected instance path without reading secrets into output.
type Entry struct {
	Path    string `json:"path"`
	Role    string `json:"role"`
	State   string `json:"state"`
	Bytes   int64  `json:"bytes"`
	Cleanup string `json:"cleanup"`
}

type Report struct {
	SettingsPath     string  `json:"settings_path"`
	DataDir          string  `json:"data_dir"`
	DatabaseIdentity string  `json:"database_identity"`
	Entries          []Entry `json:"entries"`
}

// Order also defines deletion: generated directories first, durable state last.
var managed = []struct{ name, role string }{
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
	configuration, err := settings.Load(abs)
	if err != nil {
		return Report{}, err
	}
	report := Report{SettingsPath: abs, DataDir: configuration.DataDir, Entries: []Entry{}, DatabaseIdentity: "missing"}
	executable, err := os.Executable()
	if err != nil {
		return report, err
	}
	for _, item := range []struct{ path, role, cleanup string }{
		{executable, "panel executable", "retain"},
		{abs, "panel bootstrap settings", "remove"},
		{configuration.DataDir, "instance data directory", "remove_if_empty"},
	} {
		entry, err := inspectEntry(item.path, item.role, item.cleanup)
		if err != nil {
			return report, err
		}
		report.Entries = append(report.Entries, entry)
	}
	settingsDir := filepath.Dir(abs)
	if filepath.Base(settingsDir) == "sing-box-panel" && settingsDir != configuration.DataDir {
		entry, err := inspectEntry(settingsDir, "settings directory", "remove_if_empty")
		if err != nil {
			return report, err
		}
		if entry.State != "directory" {
			entry.Cleanup = "retain"
		}
		report.Entries = append(report.Entries, entry)
	}
	dataInfo, err := os.Lstat(configuration.DataDir)
	if errors.Is(err, os.ErrNotExist) {
		return report, nil
	}
	if err != nil {
		return report, err
	}
	if !dataInfo.IsDir() || dataInfo.Mode()&os.ModeSymlink != 0 {
		return report, errors.New("data directory must be a physical directory")
	}
	root, err := os.OpenRoot(configuration.DataDir)
	if err != nil {
		return report, err
	}
	defer root.Close()
	logs, err := inspectEntry(filepath.Join(configuration.DataDir, "logs"), "log directory", "remove_if_empty")
	if err != nil {
		return report, err
	}
	report.Entries = append(report.Entries, logs)
	report.DatabaseIdentity, err = databaseIdentity(ctx, root)
	if err != nil {
		return report, err
	}
	for _, item := range managed {
		if item.name == "logs/core" && logs.State == "symlink" {
			continue
		}
		path := filepath.Join(configuration.DataDir, item.name)
		entry, err := inspectEntry(path, item.role, "remove")
		if err != nil {
			return report, err
		}
		want := "file"
		switch item.name {
		case "artifacts", "runtime", "imports", "logs/core":
			want = "directory"
		case "panel-control.sock":
			want = "socket"
		}
		if entry.State != "missing" && entry.State != want {
			entry.Cleanup = "blocked"
		}
		if report.DatabaseIdentity != "sing-box-panel" && entry.State != "missing" && item.name != panelprocess.LeaseFileName && item.name != "panel-control.sock" {
			entry.Cleanup = "blocked"
		}
		report.Entries = append(report.Entries, entry)
		if entry.State != "directory" {
			continue
		}
		if err := fs.WalkDir(root.FS(), item.name, func(name string, child fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if err := ctx.Err(); err != nil {
				return err
			}
			if name == item.name {
				return nil
			}
			info, err := child.Info()
			if err != nil {
				return err
			}
			state := fileState(info)
			cleanup := entry.Cleanup
			if state == "symlink" {
				cleanup = "remove_link"
			}
			report.Entries = append(report.Entries, Entry{Path: filepath.Join(configuration.DataDir, name), Role: item.role, State: state, Bytes: info.Size(), Cleanup: cleanup})
			return nil
		}); err != nil {
			return report, err
		}
	}
	children, err := fs.ReadDir(root.FS(), ".")
	if err != nil {
		return report, err
	}
	for _, child := range children {
		name := child.Name()
		known := name == "logs" || filepath.Join(configuration.DataDir, name) == abs
		for _, item := range managed {
			known = known || item.name == name
		}
		if known {
			continue
		}
		entry, err := inspectEntry(filepath.Join(configuration.DataDir, name), "unrecognized data-directory entry", "retain")
		if err != nil {
			return report, err
		}
		report.Entries = append(report.Entries, entry)
	}
	if logs.State == "directory" {
		children, err := fs.ReadDir(root.FS(), "logs")
		if err != nil {
			return report, err
		}
		for _, child := range children {
			if child.Name() == "core" {
				continue
			}
			entry, err := inspectEntry(filepath.Join(configuration.DataDir, "logs", child.Name()), "unrecognized log-directory entry", "retain")
			if err != nil {
				return report, err
			}
			report.Entries = append(report.Entries, entry)
		}
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
	if entry.State == "symlink" && cleanup != "retain" {
		entry.Cleanup = "blocked"
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
	// recovery state remains unknown so cleanup cannot delete unverified data.
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

// ValidateCleanup rejects broad paths, foreign databases and linked roots.
// Reserved subdirectories are owned by the selected instance; unrelated entries
// beside them remain in place, and parent directories are never recursively removed.
func ValidateCleanup(report Report) error {
	home, _ := os.UserHomeDir()
	configHome, _ := os.UserConfigDir()
	cacheHome, _ := os.UserCacheDir()
	for _, shared := range []string{string(filepath.Separator), home, configHome, cacheHome, os.TempDir()} {
		if report.DataDir == shared && shared != "" {
			return errors.New("refusing to clean a filesystem root, home or shared system directory")
		}
	}
	if len(report.Entries) == 0 {
		return errors.New("instance inventory is required")
	}
	executable := report.Entries[0].Path
	for _, item := range managed {
		destination := filepath.Join(report.DataDir, item.name)
		relative, err := filepath.Rel(destination, executable)
		if err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			return errors.New("panel executable is inside a cleanup directory; move it outside the managed data first")
		}
	}
	for _, entry := range report.Entries {
		if entry.Cleanup == "blocked" {
			return fmt.Errorf("cannot verify ownership or file type of %s", entry.Path)
		}
	}
	if report.DatabaseIdentity == "unrecognized" || report.DatabaseIdentity == "unknown" {
		return errors.New("data directory does not contain a verified panel database")
	}
	if strings.TrimSpace(report.SettingsPath) == "" {
		return errors.New("settings path is required")
	}
	return nil
}
