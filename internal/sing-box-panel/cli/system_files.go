// SPDX-License-Identifier: GPL-3.0-or-later

package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"

	"github.com/rehuony/sing-box-panel/internal/cliui"
	"github.com/rehuony/sing-box-panel/internal/pathguard"
	"github.com/rehuony/sing-box-panel/internal/procctl"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/installation"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/settings"
	"github.com/rehuony/sing-box-panel/internal/systemd"
	"github.com/spf13/cobra"
)

func newSystemCommand(state *options, service systemdService) *cobra.Command {
	root := cliui.Group("system", "Inspect or prune the selected instance's files and data")
	root.AddCommand(newSystemDFCommand(state, service), newSystemPruneCommand(state, service))
	return root
}

type instanceFilesReport struct {
	installation.Report
	Service          systemd.FilesResult `json:"service"`
	ServiceState     string              `json:"service_state"`
	ServiceMatches   bool                `json:"service_matches_instance"`
	ServiceDataDir   string              `json:"service_data_dir,omitempty"`
	ServiceDataKnown bool                `json:"service_data_known"`
	Preview          bool                `json:"preview"`
}

func inspectInstanceFiles(ctx context.Context, path string, scope systemd.Scope, service systemdService, historyPaths ...string) (instanceFilesReport, error) {
	historyPath := installation.DefaultHistoryPath()
	if len(historyPaths) > 0 {
		historyPath = historyPaths[0]
	}
	files, inspectErr := installation.InspectWithHistory(ctx, path, historyPath)
	var err error
	result := instanceFilesReport{Report: files, ServiceState: "unavailable"}
	if service == nil {
		return result, inspectErr
	}
	result.Service, err = service.Files(ctx, scope)
	if errors.Is(err, systemd.ErrUnsupportedOS) {
		result.ServiceState = "unsupported"
		return result, inspectErr
	}
	if err != nil {
		result.Warnings = append(result.Warnings, err.Error())
		return result, errors.Join(inspectErr, err)
	}
	result.ServiceState = "inspected"
	result.ServiceMatches = result.Service.StatePath != "" && filepath.Clean(result.Service.StatePath) == files.SettingsPath
	if result.Service.StatePath != "" {
		dataDir, err := settings.LoadDataDir(result.Service.StatePath)
		if err == nil {
			result.ServiceDataDir = dataDir
			result.ServiceDataKnown = true
		}
	}
	return result, inspectErr
}

func newSystemDFCommand(state *options, service systemdService) *cobra.Command {
	var rawScope string
	command := &cobra.Command{Use: "df", Short: "List selected instance files, storage paths and cleanup scope", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			scope, err := cliui.ParseScope(rawScope)
			if err != nil {
				return err
			}
			report, err := inspectInstanceFiles(cmd.Context(), state.settingsPath, scope, service, state.historyPath)
			if err != nil {
				_ = cliui.Write(cmd.OutOrStdout(), state.format, report, instanceFilesText(report, cliui.NewStyle(cmd.OutOrStdout(), state.format)))
				return &cliui.Error{Kind: cliui.ErrorValidation, Code: "instance_files_unavailable", Message: err.Error(), Cause: err}
			}
			return cliui.Write(cmd.OutOrStdout(), state.format, report, instanceFilesText(report, cliui.NewStyle(cmd.OutOrStdout(), state.format)))
		},
	}
	cliui.AddScopeFlag(command, &rawScope)
	return command
}

func newSystemPruneCommand(state *options, service systemdService) *cobra.Command {
	var rawScope string
	var yes bool
	command := &cobra.Command{Use: "prune", Short: "Preview cleanup; --yes stops the instance and removes its settings and entire data directory", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			scope, err := cliui.ParseScope(rawScope)
			if err != nil {
				return err
			}
			report, err := inspectInstanceFiles(cmd.Context(), state.settingsPath, scope, service, state.historyPath)
			if err != nil {
				_ = cliui.Write(cmd.OutOrStdout(), state.format, report, instanceFilesText(report, cliui.NewStyle(cmd.OutOrStdout(), state.format)))
				return &cliui.Error{Kind: cliui.ErrorValidation, Code: "instance_files_unavailable", Message: err.Error(), Cause: err}
			}
			report.Preview = !yes
			style := cliui.NewStyle(cmd.OutOrStdout(), state.format)
			if !yes {
				return cliui.Write(cmd.OutOrStdout(), state.format, report, instanceFilesText(report, style))
			}
			result, cleanupErr := pruneInstance(cmd.Context(), report, service)
			if err := cliui.Write(cmd.OutOrStdout(), state.format, result, cleanupText(result, cleanupErr, style)); err != nil {
				return err
			}
			if err := writeCleanupWarnings(cmd.ErrOrStderr(), state.format, result, cleanupErr); err != nil {
				cleanupErr = errors.Join(cleanupErr, err)
			}
			if cleanupErr != nil {
				return classifyCleanupError(cleanupErr)
			}
			return nil
		},
	}
	cliui.AddScopeFlag(command, &rawScope)
	command.Flags().BoolVar(&yes, "yes", false, "permanently remove the selected settings, all data-directory contents and matching managed service")
	return command
}

func stopInstanceForCleanup(ctx context.Context, report instanceFilesReport, service systemdService) (*systemd.UninstallResult, error) {
	var uninstalled *systemd.UninstallResult
	if report.ServiceState == "unavailable" {
		return nil, errors.New("system service ownership could not be inspected")
	}
	if !report.ServiceMatches && len(report.Service.Files) > 0 && report.Service.Files[0].State != "missing" {
		if !report.ServiceDataKnown {
			return nil, errors.New("installed service settings are unknown; resolve its ownership before cleanup")
		}
		overlap, err := pathguard.Overlap(report.DataDir, report.ServiceDataDir)
		if err != nil {
			return nil, err
		}
		if overlap {
			return nil, errors.New("another service configuration shares or overlaps this data directory; select that service's settings before cleanup")
		}
		for _, path := range []string{report.Service.StatePath, report.ServiceDataDir} {
			contained, err := pathguard.Contains(report.DataDir, path)
			if err != nil {
				return nil, err
			}
			if contained {
				return nil, errors.New("another service depends on a settings or data path inside this data directory; relocate it before cleanup")
			}
		}
	}
	if report.ServiceMatches {
		for _, file := range report.Service.Files {
			if file.State != "missing" && !file.Retained && !file.Managed {
				return nil, fmt.Errorf("refusing cleanup of unmanaged service file %s", file.Path)
			}
		}
		status, err := service.Status(ctx, report.Service.Scope)
		if err != nil && !errors.Is(err, systemd.ErrNotInstalled) {
			return nil, err
		}
		if err == nil && status.LoadState == "loaded" {
			if err := status.ValidateStatePath(report.SettingsPath, "settings"); err != nil {
				return nil, err
			}
		}
		result, err := service.Uninstall(ctx, systemd.UninstallRequest{Scope: report.Service.Scope})
		uninstalled = &result
		if err != nil && !errors.Is(err, systemd.ErrNotInstalled) {
			return uninstalled, err
		}
	}
	if report.DataDir == "" {
		return uninstalled, nil
	}
	for _, entry := range report.Entries {
		if entry.Path == filepath.Join(report.DataDir, procctl.SocketName) && entry.State != "socket" && entry.State != "missing" {
			// A leftover non-socket cannot answer stop requests. Clean still must
			// acquire the runtime lease, so a live owner cannot be bypassed.
			return uninstalled, nil
		}
	}
	stopCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	_, err := procctl.Stop(stopCtx, report.DataDir)
	return uninstalled, err
}

func classifyCleanupError(err error) error {
	return &cliui.Error{Kind: cliui.ErrorConflict, Code: "instance_cleanup_failed", Message: err.Error(), Cause: err}
}

func instanceFilesText(report instanceFilesReport, style cliui.Style) string {
	heading := style.Title("Instance files")
	if report.Preview {
		heading = style.Warning("Cleanup preview — no changes")
	}
	config, executable := style.Path(report.SettingsPath), ""
	for _, entry := range report.Entries {
		switch {
		case entry.Role == "panel settings" && entry.State == "missing":
			config += " (missing)"
		case entry.Role == "panel executable":
			executable = style.Path(entry.Path)
		}
	}
	entries := make([]cliui.TreeEntry, 0, len(report.Entries)+len(report.Service.Files))
	for _, entry := range report.Entries {
		if entry.State == "missing" {
			continue
		}
		var labels []string
		switch entry.State {
		case "file":
		case "directory":
			if entry.Empty != nil && *entry.Empty {
				labels = append(labels, "empty")
			}
		case "symlink":
			labels = append(labels, "link")
		default:
			labels = append(labels, entry.State)
		}
		switch entry.Role {
		case "panel executable":
			labels = append(labels, "executable")
		case "cleanup history":
			labels = append(labels, "history")
		case "historical path; ownership unconfirmed":
			labels = append(labels, "historical; confirm ownership")
		case "panel settings":
			labels = append(labels, "settings")
		case "instance data directory":
			labels = append(labels, "data")
		}
		if entry.Cleanup == "retain" {
			labels = append(labels, "retained")
		}
		entries = append(entries, cliui.TreeEntry{Path: entry.Path, Label: strings.Join(labels, ", "),
			Directory: entry.State == "directory", Size: entry.Bytes, Sized: entry.State == "file"})
	}
	for _, entry := range report.Service.Files {
		if entry.State == "missing" {
			continue
		}
		label := "service"
		if report.Service.Scope != "" {
			label += ", " + string(report.Service.Scope)
		}
		if entry.State == "enablement link" {
			label += ", enablement link"
		} else if entry.Retained {
			label += ", retained"
		} else if !report.ServiceMatches || !entry.Managed {
			label += ", outside scope"
		}
		entries = append(entries, cliui.TreeEntry{Path: entry.Path, Label: label})
	}
	var notes []string
	if !hasCleanupTargets(report) {
		notes = append(notes, "No removable resources remain for this instance.")
	} else if report.Preview {
		notes = append(notes, style.Warning("Pass --yes to stop this instance and permanently delete its settings and all data."))
	}
	for _, warning := range report.Warnings {
		notes = append(notes, style.Warning("Warning:")+" "+warning)
	}
	fields := cliui.Fields(
		cliui.Field{Label: "Config", Value: config},
		cliui.Field{Label: "Executable", Value: executable},
		cliui.Field{Label: "Service", Value: instanceServiceText(report)},
	)
	return cliui.Section(heading, fields, style.Tree(entries), cliui.Lines(notes...))
}

// instanceServiceText reports the inspected service and whether its unit
// runs the selected settings.
func instanceServiceText(report instanceFilesReport) string {
	switch report.ServiceState {
	case "unsupported":
		return "unsupported on this platform"
	case "unavailable":
		return "inspection unavailable"
	}
	if report.ServiceMatches {
		return string(report.Service.Scope) + ", uses these settings"
	}
	for _, file := range report.Service.Files {
		if file.State != "missing" {
			return string(report.Service.Scope) + ", uses other settings"
		}
	}
	return "not installed"
}

func cleanupText(result installation.CleanupResult, cleanupErr error, style cliui.Style) string {
	heading := style.Success("Cleanup results")
	if cleanupErr != nil {
		heading = style.Failure("Cleanup interrupted; confirmed results only")
	}
	entries := make([]cliui.TreeEntry, 0, len(result.Removed)+len(result.Retained)+len(result.Remaining))
	for _, path := range result.Removed {
		entries = append(entries, cliui.TreeEntry{Path: path, Label: "removed"})
	}
	for _, path := range result.Retained {
		entries = append(entries, cliui.TreeEntry{Path: path, Label: "retained"})
	}
	for _, path := range result.Remaining {
		entries = append(entries, cliui.TreeEntry{Path: path, Label: "remaining"})
	}
	fields := []cliui.Field{{Label: "Files", Value: fmt.Sprintf("%d removed, %d remaining, %d retained",
		len(result.Removed), len(result.Remaining), len(result.Retained))}}
	var notes []string
	if account := result.ServiceAccount; account != nil {
		fields = append(fields, cliui.Field{Label: "Service user", Value: style.Label(account.User)},
			cliui.Field{Label: "Service group", Value: style.Label(account.Group)})
		if account.Note != "" {
			notes = append(notes, style.Hint("Note:", account.Note))
		}
	}
	if cleanupErr == nil && len(result.Removed) == 0 {
		notes = append(notes, "No removable resources remain for this instance.")
	}
	return cliui.Section(heading, cliui.Fields(fields...), style.Tree(entries), cliui.Lines(notes...))
}

// writeCleanupWarnings reports the warnings of a cleanup that the error and
// the account summary do not already state.
func writeCleanupWarnings(writer io.Writer, format cliui.Format, result installation.CleanupResult, cleanupErr error) error {
	if format != cliui.Text {
		return nil
	}
	excluded := make(map[string]bool)
	if cleanupErr != nil {
		for _, line := range cliui.DiagnosticLines([]string{cleanupErr.Error()}) {
			excluded[line] = true
		}
	}
	if result.ServiceAccount != nil {
		excluded[accountCleanupSummary(*result.ServiceAccount)] = true
	}
	var warnings []string
	for _, line := range cliui.DiagnosticLines(result.Warnings) {
		if !excluded[line] {
			warnings = append(warnings, line)
		}
	}
	if len(warnings) == 0 {
		return nil
	}
	style := cliui.NewStyle(writer, format)
	_, err := fmt.Fprintln(writer, "\n"+style.Diagnostic(style.Warning("Warnings"), warnings))
	return err
}
