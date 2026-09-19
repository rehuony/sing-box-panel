// SPDX-License-Identifier: GPL-3.0-or-later

package cli

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/rehuony/sing-box-panel/internal/installation"
	"github.com/rehuony/sing-box-panel/internal/panelprocess"
	"github.com/rehuony/sing-box-panel/internal/settings"
	panelSystemd "github.com/rehuony/sing-box-panel/internal/systemd"
	"github.com/spf13/cobra"
)

type instanceFilesReport struct {
	installation.Report
	Service          panelSystemd.FilesResult `json:"service"`
	ServiceState     string                   `json:"service_state"`
	ServiceMatches   bool                     `json:"service_matches_instance"`
	ServiceDataDir   string                   `json:"service_data_dir,omitempty"`
	ServiceDataKnown bool                     `json:"service_data_known"`
	Preview          bool                     `json:"preview"`
}

func inspectInstanceFiles(ctx context.Context, path string, scope panelSystemd.Scope, service panelSystemd.Service) (instanceFilesReport, error) {
	files, err := installation.Inspect(ctx, path)
	result := instanceFilesReport{Report: files, ServiceState: "unavailable"}
	if err != nil {
		return result, err
	}
	if service == nil {
		return result, nil
	}
	result.Service, err = service.Files(ctx, scope)
	if errors.Is(err, panelSystemd.ErrUnsupportedOS) {
		result.ServiceState = "unsupported"
		return result, nil
	}
	if err != nil {
		return result, err
	}
	result.ServiceState = "inspected"
	result.ServiceMatches = result.Service.SettingsPath != "" && filepath.Clean(result.Service.SettingsPath) == files.SettingsPath
	if result.Service.SettingsPath != "" {
		configuration, err := settings.Load(result.Service.SettingsPath)
		if err == nil {
			result.ServiceDataDir = configuration.DataDir
			result.ServiceDataKnown = true
		}
	}
	return result, nil
}

func newSystemFilesCommand(state *options, service panelSystemd.Service) *cobra.Command {
	var rawScope string
	command := &cobra.Command{Use: "files", Short: "List selected instance files, storage paths and cleanup scope", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			scope, err := parseSystemScope(rawScope)
			if err != nil {
				return err
			}
			report, err := inspectInstanceFiles(cmd.Context(), state.settingsPath, scope, service)
			if err != nil {
				return &Error{Kind: ErrorValidation, Code: "instance_files_unavailable", Message: err.Error(), Cause: err}
			}
			return writeResult(cmd.OutOrStdout(), state.format, report, instanceFilesText(report))
		},
	}
	addSystemScopeFlag(command, &rawScope)
	return command
}

func newSystemCleanCommand(state *options, service panelSystemd.Service) *cobra.Command {
	var rawScope string
	var yes bool
	command := &cobra.Command{Use: "clean", Short: "Preview cleanup; --yes stops the instance and removes its settings and managed data", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			scope, err := parseSystemScope(rawScope)
			if err != nil {
				return err
			}
			report, err := inspectInstanceFiles(cmd.Context(), state.settingsPath, scope, service)
			if err != nil {
				return &Error{Kind: ErrorValidation, Code: "instance_files_unavailable", Message: err.Error(), Cause: err}
			}
			report.Preview = !yes
			if !yes {
				return writeResult(cmd.OutOrStdout(), state.format, report, instanceFilesText(report)+"\nPreview only. Pass --yes to stop this instance and permanently remove its settings, configuration, logs and cores.")
			}
			if err := installation.ValidateCleanup(report.Report); err != nil {
				return classifyCleanupError(err)
			}
			serviceRemoved, err := stopInstanceForCleanup(cmd.Context(), report, service)
			if err != nil {
				if len(serviceRemoved) > 0 {
					result := installation.CleanupResult{Removed: serviceRemoved, Retained: []string{}}
					_ = writeResult(cmd.OutOrStdout(), state.format, result, cleanupText(result))
				}
				return classifyCleanupError(err)
			}
			result, err := installation.Clean(cmd.Context(), report.Report)
			result.Removed = append(serviceRemoved, result.Removed...)
			if err != nil {
				if len(result.Removed) > 0 {
					_ = writeResult(cmd.OutOrStdout(), state.format, result, cleanupText(result))
				}
				return classifyCleanupError(err)
			}
			return writeResult(cmd.OutOrStdout(), state.format, result, cleanupText(result))
		},
	}
	addSystemScopeFlag(command, &rawScope)
	command.Flags().BoolVar(&yes, "yes", false, "permanently remove the selected instance's settings, database, logs, cores and matching managed service")
	return command
}

func stopInstanceForCleanup(ctx context.Context, report instanceFilesReport, service panelSystemd.Service) ([]string, error) {
	var removed []string
	if report.ServiceState == "unavailable" {
		return nil, errors.New("system service ownership could not be inspected")
	}
	if !report.ServiceMatches && len(report.Service.Files) > 0 && report.Service.Files[0].State != "missing" {
		if !report.ServiceDataKnown {
			return nil, errors.New("installed service settings are unknown; resolve its ownership before cleanup")
		}
		selected, err := filepath.EvalSymlinks(report.DataDir)
		if err != nil {
			selected = report.DataDir
		}
		other, err := filepath.EvalSymlinks(report.ServiceDataDir)
		if err != nil {
			other = report.ServiceDataDir
		}
		if selected == other {
			return nil, errors.New("another service configuration shares this data directory; select that service's settings before cleanup")
		}
	}
	if report.ServiceMatches {
		for _, file := range report.Service.Files {
			if file.State != "missing" && !file.Managed {
				return nil, fmt.Errorf("refusing cleanup of unmanaged service file %s", file.Path)
			}
		}
		status, err := service.Status(ctx, report.Service.Scope)
		if err != nil && !errors.Is(err, panelSystemd.ErrNotInstalled) {
			return nil, err
		}
		if err == nil && (status.NeedDaemonReload || filepath.Clean(status.UnitFileSettingsPath) != report.SettingsPath) {
			return nil, errors.New("service settings are ambiguous or changed on disk; resolve the service configuration before cleanup")
		}
		result, err := service.Uninstall(ctx, panelSystemd.UninstallRequest{Scope: report.Service.Scope})
		if err != nil && !errors.Is(err, panelSystemd.ErrNotInstalled) {
			return nil, err
		}
		removed = result.RemovedPaths
	}
	stopCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	_, err := panelprocess.Stop(stopCtx, report.DataDir)
	return removed, err
}

func classifyCleanupError(err error) error {
	return &Error{Kind: ErrorConflict, Code: "instance_cleanup_failed", Message: err.Error(), Cause: err}
}

func instanceFilesText(report instanceFilesReport) string {
	var text strings.Builder
	fmt.Fprintf(&text, "settings\t%s\ndata directory\t%s\ndatabase identity\t%s\n", report.SettingsPath, report.DataDir, report.DatabaseIdentity)
	for _, entry := range report.Entries {
		fmt.Fprintf(&text, "%s\t%s\t%s\t%s\n", entry.State, entry.Cleanup, entry.Role, entry.Path)
	}
	for _, entry := range report.Service.Files {
		action := "retain"
		if report.ServiceMatches && entry.Managed {
			action = "uninstall"
		}
		fmt.Fprintf(&text, "%s\t%s\tsystem service\t%s\n", entry.State, action, entry.Path)
	}
	text.WriteString("System journal entries and operating-system accounts remain managed by the OS.")
	return text.String()
}

func cleanupText(result installation.CleanupResult) string {
	var text strings.Builder
	for _, path := range result.Removed {
		fmt.Fprintf(&text, "removed\t%s\n", path)
	}
	for _, path := range result.Retained {
		fmt.Fprintf(&text, "retained\t%s\n", path)
	}
	return strings.TrimSuffix(text.String(), "\n")
}
