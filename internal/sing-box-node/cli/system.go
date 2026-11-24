// SPDX-License-Identifier: GPL-3.0-or-later

package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/rehuony/sing-box-panel/internal/pathguard"
	"github.com/rehuony/sing-box-panel/internal/procctl"
	"github.com/rehuony/sing-box-panel/internal/sing-box-node/settings"
	"github.com/rehuony/sing-box-panel/internal/systemd"
	"github.com/spf13/cobra"
)

// stopTimeout bounds how long prune waits for a manually started node.
const stopTimeout = 30 * time.Second

// instanceEntry is one file or directory of the selected node.
type instanceEntry struct {
	Path  string `json:"path"`
	Role  string `json:"role"`
	State string `json:"state"`
	Bytes int64  `json:"bytes"`
}

// instanceReport is everything the selected settings own: the settings
// file, its data directory with every entry in it, the service whose unit
// names the settings, and the executable, which prune keeps.
type instanceReport struct {
	SettingsPath string          `json:"settings_path"`
	Executable   string          `json:"executable"`
	DataDir      string          `json:"data_dir"`
	Service      *systemd.Status `json:"service,omitempty"`
	Entries      []instanceEntry `json:"entries"`
}

func newSystemCommand(settingsPath *string, service systemdService) *cobra.Command {
	var asJSON bool
	root := &cobra.Command{
		Use:   "system",
		Short: "Inspect or remove this node's files",
		Args:  cobra.NoArgs,
		RunE:  func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
	root.PersistentFlags().BoolVar(&asJSON, "json", false, "print the result as JSON")

	df := &cobra.Command{
		Use:   "df",
		Short: "Show the settings, data and service files of the selected node and their sizes",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			report, err := inspectInstance(cmd.Context(), *settingsPath, service)
			if err != nil {
				return err
			}
			if asJSON {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(report)
			}
			_, err = fmt.Fprint(cmd.OutOrStdout(), report.text())
			return err
		},
	}

	var yes, keepUser bool
	prune := &cobra.Command{
		Use:   "prune",
		Short: "Preview, or with --yes remove, the selected node's service, settings and data",
		Long: "Without --yes, list what would be removed. With --yes, stop the node, uninstall the service whose unit names " +
			"these settings, then delete the settings file and the whole data directory, including the enrollment, installed " +
			"cores and logs. The executable is kept. This cannot be undone.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			report, err := inspectInstance(cmd.Context(), *settingsPath, service)
			if err != nil {
				return err
			}
			if !yes {
				if asJSON {
					return json.NewEncoder(cmd.OutOrStdout()).Encode(report)
				}
				_, err = fmt.Fprintf(cmd.OutOrStdout(), "%s\nPass --yes to stop the node and permanently delete its service, settings and data.\n", report.text())
				return err
			}
			removed, err := pruneInstance(cmd.Context(), report, service, keepUser)
			if asJSON {
				if outputErr := json.NewEncoder(cmd.OutOrStdout()).Encode(map[string]any{"removed": removed}); outputErr != nil {
					return errors.Join(err, outputErr)
				}
			} else {
				text := fmt.Sprintf("Removed %d paths:\n", len(removed))
				for _, path := range removed {
					text += "  " + path + "\n"
				}
				if _, outputErr := fmt.Fprint(cmd.OutOrStdout(), text); outputErr != nil {
					return errors.Join(err, outputErr)
				}
			}
			return err
		},
	}
	prune.Flags().BoolVar(&yes, "yes", false, "stop the node and permanently delete its service, settings and data")
	prune.Flags().BoolVar(&keepUser, "keep-user", false, "keep the system service account when uninstalling the service")
	root.AddCommand(df, prune)
	return root
}

// inspectInstance reports the selected node's files without changing any.
func inspectInstance(ctx context.Context, settingsPath string, service systemdService) (instanceReport, error) {
	path, err := filepath.Abs(filepath.Clean(settingsPath))
	if err != nil {
		return instanceReport{}, err
	}
	report := instanceReport{SettingsPath: path}
	if executable, err := os.Executable(); err == nil {
		report.Executable = executable
	}
	for _, file := range []struct{ path, role string }{{path, "settings"}, {path + ".lock", "settings lock"}} {
		if entry, ok := inspectEntry(file.path, file.role); ok {
			report.Entries = append(report.Entries, entry)
		}
	}
	dataDir, err := settings.LoadDataDir(path)
	switch {
	case errors.Is(err, os.ErrNotExist):
	case err != nil:
		return instanceReport{}, err
	default:
		report.DataDir = dataDir
		if entry, ok := inspectEntry(dataDir, "data directory"); ok {
			report.Entries = append(report.Entries, entry)
			children, err := os.ReadDir(dataDir)
			if err != nil {
				return instanceReport{}, fmt.Errorf("read the data directory: %w", err)
			}
			for _, child := range children {
				if entry, ok := inspectEntry(filepath.Join(dataDir, child.Name()), "data"); ok {
					report.Entries = append(report.Entries, entry)
				}
			}
		}
	}
	if service != nil {
		if status, err := service.Status(ctx, systemd.ScopeAuto); err == nil && status.UnitFileStatePath == path {
			report.Service = &status
			if files, err := service.Files(ctx, status.Scope); err == nil {
				for _, file := range files.Files {
					if file.State != "missing" {
						report.Entries = append(report.Entries, instanceEntry{Path: file.Path, Role: "service", State: file.State})
					}
				}
			}
		}
	}
	return report, nil
}

// inspectEntry describes path without following a symbolic link at it; a
// directory's size counts every regular file beneath it.
func inspectEntry(path, role string) (instanceEntry, bool) {
	info, err := os.Lstat(path)
	if err != nil {
		return instanceEntry{}, false
	}
	entry := instanceEntry{Path: path, Role: role, State: "file", Bytes: info.Size()}
	switch {
	case info.Mode()&os.ModeSymlink != 0:
		entry.State, entry.Bytes = "link", 0
	case info.Mode()&os.ModeSocket != 0:
		entry.State, entry.Bytes = "socket", 0
	case info.IsDir():
		entry.State, entry.Bytes = "directory", 0
		_ = filepath.WalkDir(path, func(_ string, item fs.DirEntry, err error) error {
			if err == nil && item.Type().IsRegular() {
				if info, err := item.Info(); err == nil {
					entry.Bytes += info.Size()
				}
			}
			return nil
		})
	}
	return entry, true
}

func (report instanceReport) text() string {
	var output strings.Builder
	fmt.Fprintf(&output, "Config      %s\n", report.SettingsPath)
	fmt.Fprintf(&output, "Executable  %s (kept)\n", report.Executable)
	if report.Service != nil {
		fmt.Fprintf(&output, "Service     %s %s, %s\n", report.Service.Scope, report.Service.Unit, report.Service.ActiveState)
	}
	if len(report.Entries) == 0 {
		output.WriteString("\nNo files remain for these settings.\n")
		return output.String()
	}
	output.WriteString("\n")
	for _, entry := range report.Entries {
		size := ""
		if entry.State == "file" || entry.State == "directory" {
			size = formatBytes(entry.Bytes)
		}
		name := entry.Path
		if entry.State == "directory" {
			name += "/"
		}
		fmt.Fprintf(&output, "  %-15s %9s  %s\n", entry.Role, size, name)
	}
	return output.String()
}

func formatBytes(bytes int64) string {
	if bytes < 1024 {
		return fmt.Sprintf("%d B", bytes)
	}
	value := float64(bytes)
	for _, suffix := range []string{"KiB", "MiB", "GiB"} {
		value /= 1024
		if value < 1024 {
			return fmt.Sprintf("%.1f %s", value, suffix)
		}
	}
	return fmt.Sprintf("%.1f TiB", value/1024)
}

// pruneInstance stops the node, uninstalls its service and deletes its
// settings and data directory, refusing roots, homes and shared system
// directories. It returns what it removed, even after a failure.
func pruneInstance(ctx context.Context, report instanceReport, service systemdService, keepUser bool) ([]string, error) {
	var removed []string
	if report.DataDir != "" {
		if err := pathguard.RejectShared(report.DataDir); err != nil {
			return nil, fmt.Errorf("refusing to delete the data directory %s: %w", report.DataDir, err)
		}
		if info, err := os.Lstat(report.DataDir); err == nil && info.Mode()&os.ModeSymlink != 0 {
			return nil, fmt.Errorf("refusing to delete the data directory %s: it is a symbolic link", report.DataDir)
		}
		if report.Executable != "" && pathguard.Within(report.Executable, report.DataDir) {
			return nil, fmt.Errorf("refusing to delete the data directory %s: it holds the running executable", report.DataDir)
		}
	}
	if report.Service != nil {
		result, err := service.Uninstall(ctx, systemd.UninstallRequest{Scope: report.Service.Scope, KeepUser: keepUser})
		removed = append(removed, result.RemovedPaths...)
		if err != nil {
			return removed, fmt.Errorf("uninstall the service: %w", err)
		}
	}
	if report.DataDir != "" {
		if running, err := procctl.LeaseHeld(report.DataDir); err == nil && running {
			stopCtx, cancel := context.WithTimeout(ctx, stopTimeout)
			_, err := procctl.Stop(stopCtx, report.DataDir)
			cancel()
			if err != nil {
				return removed, fmt.Errorf("stop the running node: %w", err)
			}
		}
		if _, err := os.Lstat(report.DataDir); err == nil {
			if err := os.RemoveAll(report.DataDir); err != nil {
				return removed, fmt.Errorf("delete the data directory: %w", err)
			}
			removed = append(removed, report.DataDir)
		}
	}
	for _, path := range []string{report.SettingsPath, report.SettingsPath + ".lock"} {
		if err := os.Remove(path); err == nil {
			removed = append(removed, path)
		} else if !errors.Is(err, os.ErrNotExist) {
			return removed, fmt.Errorf("delete %s: %w", path, err)
		}
	}
	// The settings directory goes only when nothing else is in it.
	directory := filepath.Dir(report.SettingsPath)
	if entries, err := os.ReadDir(directory); err == nil && len(entries) == 0 && filepath.Base(directory) == "sing-box-node" {
		if err := os.Remove(directory); err == nil {
			removed = append(removed, directory)
		}
	}
	slices.Sort(removed)
	return slices.Compact(removed), nil
}
