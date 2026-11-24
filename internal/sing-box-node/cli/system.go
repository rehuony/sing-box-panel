// SPDX-License-Identifier: GPL-3.0-or-later

package cli

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/rehuony/sing-box-panel/internal/cliui"
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
	// ServiceState is how the service was found: inspected (its unit names
	// these settings), other_settings, unknown_settings (its unit file or a
	// drop-in hides them), not_installed, unsupported or unavailable.
	ServiceState string          `json:"service_state"`
	Entries      []instanceEntry `json:"entries"`
}

func newSystemCommand(state *options, service systemdService) *cobra.Command {
	root := cliui.Group("system", "Inspect or remove this node's files")
	df := &cobra.Command{
		Use:   "df",
		Short: "Show the settings, data and service files of the selected node and their sizes",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			report, err := inspectInstance(cmd.Context(), state.settingsPath, service)
			if err != nil {
				return cliui.Fail(cliui.ErrorValidation, "instance_files_unavailable", err)
			}
			style := cliui.NewStyle(cmd.OutOrStdout(), state.format)
			return cliui.Write(cmd.OutOrStdout(), state.format, report, report.text(false, style))
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
			report, err := inspectInstance(cmd.Context(), state.settingsPath, service)
			if err != nil {
				return cliui.Fail(cliui.ErrorValidation, "instance_files_unavailable", err)
			}
			style := cliui.NewStyle(cmd.OutOrStdout(), state.format)
			if !yes {
				return cliui.Write(cmd.OutOrStdout(), state.format, report, report.text(true, style))
			}
			removed, err := pruneInstance(cmd.Context(), report, service, keepUser)
			if outputErr := cliui.Write(cmd.OutOrStdout(), state.format, map[string]any{"removed": removed}, pruneText(removed, err, style)); outputErr != nil {
				return errors.Join(err, outputErr)
			}
			return cliui.Fail(cliui.ErrorConflict, "instance_cleanup_failed", err)
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
		if entry, ok := inspectEntry(executable, "executable"); ok {
			report.Entries = append(report.Entries, entry)
		}
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
	report.ServiceState = "unavailable"
	if service != nil {
		status, err := service.Status(ctx, systemd.ScopeAuto)
		switch {
		case errors.Is(err, systemd.ErrUnsupportedOS):
			report.ServiceState = "unsupported"
		case errors.Is(err, systemd.ErrNotInstalled):
			report.ServiceState = "not_installed"
		case err != nil:
		case status.UnitFileStatePath == "":
			report.ServiceState = "unknown_settings"
		case status.UnitFileStatePath != path:
			report.ServiceState = "other_settings"
		default:
			report.ServiceState = "inspected"
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

// text lists the selected node's files as a tree, or, for a preview of
// prune, what prune would delete.
func (report instanceReport) text(preview bool, style cliui.Style) string {
	heading := style.Title("Instance files")
	if preview {
		heading = style.Warning("Cleanup preview — no changes")
	}
	service := map[string]string{
		"other_settings": "installed for other settings", "unknown_settings": "installed; its settings are unknown (check drop-ins)",
		"not_installed": "not installed",
		"unsupported":   "unsupported on this platform", "unavailable": "inspection unavailable",
	}[report.ServiceState]
	if report.Service != nil {
		service = string(report.Service.Scope) + ", uses these settings, " + report.Service.ActiveState
	}
	entries := make([]cliui.TreeEntry, 0, len(report.Entries))
	removable := false
	for _, entry := range report.Entries {
		var labels []string
		switch entry.State {
		case "link", "socket":
			labels = append(labels, entry.State)
		}
		switch entry.Role {
		case "executable":
			labels = append(labels, "executable", "retained")
		case "settings", "data directory":
			labels = append(labels, map[string]string{"settings": "settings", "data directory": "data"}[entry.Role])
		case "service":
			labels = append(labels, "service", string(report.Service.Scope))
			if entry.State == "enablement link" {
				labels = append(labels, "enablement link")
			}
		}
		removable = removable || entry.Role != "executable"
		entries = append(entries, cliui.TreeEntry{Path: entry.Path, Label: strings.Join(labels, ", "),
			Directory: entry.State == "directory", Size: entry.Bytes, Sized: entry.State == "file" || entry.State == "directory"})
	}
	note := ""
	switch {
	case !removable:
		note = "No files remain for these settings."
	case preview:
		note = style.Warning("Pass --yes to stop the node and permanently delete its service, settings and data.")
	}
	fields := cliui.Fields(
		cliui.Field{Label: "Config", Value: style.Path(report.SettingsPath)},
		cliui.Field{Label: "Executable", Value: style.Path(report.Executable)},
		cliui.Field{Label: "Service", Value: service},
	)
	return cliui.Section(heading, fields, style.Tree(entries), note)
}

// pruneText lists what prune removed, which is everything it confirmed even
// when it failed.
func pruneText(removed []string, err error, style cliui.Style) string {
	heading := style.Success("Cleanup results")
	if err != nil {
		heading = style.Failure("Cleanup interrupted; confirmed results only")
	}
	entries := make([]cliui.TreeEntry, 0, len(removed))
	for _, path := range removed {
		entries = append(entries, cliui.TreeEntry{Path: path, Label: "removed"})
	}
	note := ""
	if err == nil && len(removed) == 0 {
		note = "No files remain for these settings."
	}
	return cliui.Section(heading, cliui.Fields(cliui.Field{Label: "Files", Value: fmt.Sprintf("%d removed", len(removed))}),
		style.Tree(entries), note)
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
