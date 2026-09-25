// SPDX-License-Identifier: GPL-3.0-or-later

package installation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/rehuony/sing-box-panel/internal/jsonstrict"
	"github.com/rehuony/sing-box-panel/internal/settings"
	"github.com/rehuony/sing-box-panel/internal/store"
)

// CleanupHistory is discovery evidence, never authority to delete a path or
// select storage at startup. Only root paths are retained, not their contents.
type CleanupHistory struct {
	SettingsPath   string                `json:"settings_path"`
	DataDirs       []string              `json:"data_directories"`
	ServicePaths   []string              `json:"service_paths"`
	Scope          string                `json:"scope"`
	Outcome        string                `json:"outcome"`
	UpdatedAt      time.Time             `json:"updated_at"`
	RemovedCount   int                   `json:"removed_count"`
	RemainingCount int                   `json:"remaining_count"`
	ServiceAccount *AccountCleanupResult `json:"service_account,omitempty"`
}

type historyDocument struct {
	Version   int                       `json:"version"`
	Instances map[string]CleanupHistory `json:"instances"`
}

// DefaultHistoryPath is independent of both instance configuration and data.
func DefaultHistoryPath() string {
	if os.Geteuid() == 0 {
		return "/var/lib/sing-box-panel-state/cleanup-history.json"
	}
	home := os.Getenv("XDG_STATE_HOME")
	if !filepath.IsAbs(home) {
		userHome, _ := os.UserHomeDir()
		home = filepath.Join(userHome, ".local", "state")
	}
	return filepath.Join(home, "sing-box-panel", "cleanup-history.json")
}

func readHistory(path string) (historyDocument, error) {
	empty := historyDocument{Version: 1, Instances: map[string]CleanupHistory{}}
	if !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return empty, errors.New("cleanup history path must be absolute and normalized")
	}
	if info, err := os.Lstat(filepath.Dir(path)); err == nil && (!info.IsDir() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0o077 != 0) {
		return empty, errors.New("cleanup history directory must be a private physical directory")
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return empty, err
	}
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return empty, nil
	}
	if err != nil {
		return empty, err
	}
	if !info.Mode().IsRegular() || info.Size() > settings.MaximumBytes || info.Mode().Perm()&0o077 != 0 {
		return empty, errors.New("cleanup history must be a private regular file of at most 1 MiB")
	}
	file, err := os.Open(path)
	if err != nil {
		return empty, err
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil || !os.SameFile(info, opened) {
		return empty, errors.New("cleanup history changed while reading")
	}
	raw, err := io.ReadAll(io.LimitReader(file, settings.MaximumBytes+1))
	if err != nil {
		return empty, err
	}
	var document historyDocument
	if err := jsonstrict.Decode(raw, settings.MaximumBytes, &document); err != nil {
		return empty, fmt.Errorf("read cleanup history: %w", err)
	}
	if document.Version != 1 || document.Instances == nil {
		return empty, errors.New("unsupported cleanup history format")
	}
	for key, record := range document.Instances {
		if key != record.SettingsPath {
			return empty, errors.New("cleanup history instance key does not match its settings path")
		}
		if err := validateHistory(record); err != nil {
			return empty, err
		}
	}
	return document, nil
}

func validateHistory(record CleanupHistory) error {
	if record.ServiceAccount != nil {
		for _, state := range []string{record.ServiceAccount.User, record.ServiceAccount.Group} {
			switch state {
			case "absent", "retained", "removed", "unknown":
			default:
				return errors.New("cleanup history contains an invalid account outcome")
			}
		}
	}
	for _, path := range append(append([]string{record.SettingsPath}, record.DataDirs...), record.ServicePaths...) {
		if !filepath.IsAbs(path) || filepath.Clean(path) != path || strings.ContainsRune(path, 0) {
			return errors.New("cleanup history contains an invalid path")
		}
	}
	if record.Scope != "" && record.Scope != "system" && record.Scope != "user" {
		return errors.New("cleanup history contains an invalid service scope")
	}
	switch record.Outcome {
	case "started", "completed", "interrupted":
	default:
		return errors.New("cleanup history contains an invalid outcome")
	}
	return nil
}

// WriteCleanupHistory serializes writers by locking the dedicated directory,
// avoiding an extra retained lock file and coordinating atomic replacement.
func WriteCleanupHistory(ctx context.Context, path string, record CleanupHistory) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := validateHistory(record); err != nil {
		return err
	}
	if _, err := readHistory(path); err != nil {
		return err
	}
	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return err
	}
	lock, err := store.LockDirectoryForCleanup(directory)
	if err != nil {
		return err
	}
	defer lock.Close()
	info, err := os.Lstat(directory)
	locked, statErr := lock.Stat()
	if err != nil || statErr != nil || !os.SameFile(info, locked) {
		return errors.New("cleanup history directory changed while locking")
	}
	if err := lock.Chmod(0o700); err != nil {
		return err
	}
	document, err := readHistory(path)
	if err != nil {
		return err
	}
	record.UpdatedAt = time.Now().UTC()
	previous := document.Instances[record.SettingsPath]
	record.DataDirs = uniquePaths(append(previous.DataDirs, record.DataDirs...))
	record.ServicePaths = uniquePaths(append(previous.ServicePaths, record.ServicePaths...))
	document.Instances[record.SettingsPath] = record
	raw, err := json.MarshalIndent(document, "", "  ")
	if err != nil {
		return err
	}
	if len(raw)+1 > settings.MaximumBytes {
		return errors.New("cleanup history exceeds 1 MiB; retained existing history")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return settings.WriteAtomic(path, append(raw, '\n'))
}

func uniquePaths(paths []string) []string {
	paths = append([]string{}, paths...)
	slices.Sort(paths)
	return slices.Compact(paths)
}

// InspectWithHistory adds retained history and existing historical roots without
// descending into them. Live settings and .location remain the only data source.
func InspectWithHistory(ctx context.Context, settingsPath, historyPath string) (Report, error) {
	report, inspectErr := Inspect(ctx, settingsPath)
	report.HistoryPath = historyPath
	entry, err := inspectEntry(historyPath, "cleanup history", "retain")
	if err == nil && entry.State != "missing" {
		report.Entries = append(report.Entries, entry)
	}
	document, historyErr := readHistory(historyPath)
	if record, exists := document.Instances[report.SettingsPath]; exists {
		report.History = &record
		seen := map[string]bool{}
		for _, entry := range report.Entries {
			seen[entry.Path] = true
		}
		for _, path := range append(append([]string{}, record.DataDirs...), record.ServicePaths...) {
			if seen[path] {
				continue
			}
			item, pathErr := inspectEntry(path, "historical path; ownership unconfirmed", "retain")
			if pathErr != nil {
				historyErr = errors.Join(historyErr, pathErr)
				report.Warnings = append(report.Warnings, pathErr.Error())
				continue
			}
			if item.State != "missing" {
				report.Entries = append(report.Entries, item)
				seen[path] = true
			}
		}
	}
	for _, problem := range []error{inspectErr, err, historyErr} {
		if problem != nil {
			report.Warnings = append(report.Warnings, problem.Error())
		}
	}
	return report, errors.Join(inspectErr, err, historyErr)
}

// Recheck records current existence, using the same no-follow inspection as df.
// It does not infer successful deletion from an attempted filesystem operation.
func Recheck(expected Report, result *CleanupResult) {
	for _, entry := range expected.Entries {
		if entry.State == "missing" {
			continue
		}
		current, err := inspectEntry(entry.Path, entry.Role, entry.Cleanup)
		if err != nil {
			result.Warnings = append(result.Warnings, err.Error())
			continue
		}
		if current.State == "missing" {
			continue
		}
		if entry.Cleanup == "retain" || entry.Cleanup == "remove_if_empty" {
			result.Retained = append(result.Retained, entry.Path)
		} else {
			result.Remaining = append(result.Remaining, entry.Path)
		}
	}
	result.Removed = uniquePaths(result.Removed)
	result.Retained = uniquePaths(result.Retained)
	result.Remaining = uniquePaths(result.Remaining)
	result.Retained = slices.DeleteFunc(result.Retained, func(path string) bool {
		return slices.Contains(result.Remaining, path) || slices.Contains(result.Removed, path)
	})
}
