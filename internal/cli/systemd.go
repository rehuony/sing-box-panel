// SPDX-License-Identifier: GPL-3.0-or-later

package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/rehuony/sing-box-panel/internal/installation"
	"github.com/rehuony/sing-box-panel/internal/settings"
	panelSystemd "github.com/rehuony/sing-box-panel/internal/systemd"
	"github.com/spf13/cobra"
)

// ConfigurationFileName is the logical name of the sing-box document in SQLite.
const ConfigurationFileName = "config.json"

func newSystemdCommand(state *options, service panelSystemd.Service) *cobra.Command {
	root := group("systemd", "Install or manage the sing-box-panel systemd service")
	root.AddCommand(
		newSystemInstallCommand(state, service),
		newSystemUninstallCommand(state, service),
		newSystemStatusCommand(state, service),
		newSystemControlCommand(state, service, panelSystemd.ActionStart),
		newSystemControlCommand(state, service, panelSystemd.ActionStop),
		newSystemControlCommand(state, service, panelSystemd.ActionRestart),
		newSystemLogsCommand(state, service),
	)
	return root
}

func newSystemInstallCommand(state *options, service panelSystemd.Service) *cobra.Command {
	var rawScope string
	var force, now bool
	command := &cobra.Command{
		Use:   "install",
		Short: "Install and enable the audited systemd unit",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if service == nil {
				return systemServiceUnavailable()
			}
			scope, err := parseSystemScope(rawScope)
			if err != nil {
				return err
			}
			settingsPath, err := filepath.Abs(filepath.Clean(state.settingsPath))
			if err != nil {
				return &Error{Kind: ErrorValidation, Code: "system_settings_path_invalid", Message: err.Error(), Cause: err}
			}
			var dataDir string
			if now {
				var value settings.Settings
				value, err = settings.Load(settingsPath)
				dataDir = value.DataDir
			} else {
				dataDir, err = settings.ConfiguredDataDir(settingsPath)
			}
			if err != nil {
				if !errors.Is(err, os.ErrNotExist) {
					return &Error{Kind: ErrorValidation, Code: "system_settings_invalid", Message: err.Error(), Cause: err}
				}
				dataDir = ""
			}
			result, err := service.Install(cmd.Context(), panelSystemd.InstallRequest{
				Scope: scope, SettingsPath: settingsPath, DataDir: dataDir, Force: force, Now: now,
			})
			if err != nil {
				return classifySystemError("system_install_failed", err)
			}
			if result.SettingsCreated {
				value, err := settings.Load(settingsPath)
				if err != nil {
					return err
				}
				if err := writeServerInitialization(cmd, state, value); err != nil {
					return err
				}
			}
			text := fmt.Sprintf("installed and enabled %s %s at %s", result.Scope, result.Unit, result.UnitPath)
			if result.Started {
				text += "; service started"
			}
			return writeResult(cmd.OutOrStdout(), state.format, result, text)
		},
	}
	addSystemScopeFlag(command, &rawScope)
	command.Flags().BoolVar(&force, "force", false, "replace conflicting regular managed destinations")
	command.Flags().BoolVar(&now, "now", false, "start the unit after enabling it")
	return command
}

func newSystemUninstallCommand(state *options, service panelSystemd.Service) *cobra.Command {
	var rawScope string
	var force, keepUser bool
	command := &cobra.Command{
		Use:   "uninstall",
		Short: "Uninstall the service and, in system scope, its dedicated account",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if service == nil {
				return systemServiceUnavailable()
			}
			scope, err := parseSystemScope(rawScope)
			if err != nil {
				return err
			}
			result, err := service.Uninstall(cmd.Context(), panelSystemd.UninstallRequest{Scope: scope, Force: force, KeepUser: keepUser})
			if err != nil {
				if result.Scope != "" {
					text := fmt.Sprintf("Uninstall interrupted; %d service files removed; settings and data retained", len(result.RemovedPaths))
					text += uninstallAccountSummary(result)
					for _, path := range result.RemovedPaths {
						text += "\n" + path + " [removed]"
					}
					if writeErr := writeResult(cmd.OutOrStdout(), state.format, result, text); writeErr != nil {
						return errors.Join(err, writeErr)
					}
				}
				return classifySystemError("system_uninstall_failed", err)
			}
			text := fmt.Sprintf("uninstalled %s %s; settings and data retained", result.Scope, result.Unit)
			text += uninstallAccountSummary(result)
			return writeResult(cmd.OutOrStdout(), state.format, result, text)
		},
	}
	addSystemScopeFlag(command, &rawScope)
	command.Flags().BoolVar(&force, "force", false, "remove conflicting regular destinations at the exact managed paths")
	command.Flags().BoolVar(&keepUser, "keep-user", false, "retain the system service account and its file ownership")
	return command
}

func uninstallAccountSummary(result panelSystemd.UninstallResult) string {
	account := uninstallAccountResult(result)
	if account == nil {
		return ""
	}
	return "; " + accountCleanupSummary(*account)
}

func accountCleanupSummary(account installation.AccountCleanupResult) string {
	text := fmt.Sprintf("service user %s; service group %s", account.User, account.Group)
	if account.Note != "" {
		text += " (" + account.Note + ")"
	}
	return text
}

func uninstallAccountResult(result panelSystemd.UninstallResult) *installation.AccountCleanupResult {
	if result.Scope != panelSystemd.ScopeSystem {
		return nil
	}
	state := func(retained, removed bool) string {
		switch {
		case retained:
			return "retained"
		case removed:
			return "removed"
		case result.AccountInspected:
			return "absent"
		default:
			return "unknown"
		}
	}
	return &installation.AccountCleanupResult{
		User:  state(result.AccountRetained, result.AccountRemoved),
		Group: state(result.GroupRetained, result.GroupRemoved), Note: result.AccountNote,
	}
}

// systemStatusReport joins systemd's view of the unit with two facts read
// from disk: the settings path written in the installed unit file, and the
// storage locations declared by that settings file as it exists now. Both are
// labeled with their source. Neither proves what the running process started
// with, so the live settings are always reported as unknown, and the CLI's
// own --config path is listed separately rather than substituted.
type systemStatusReport struct {
	Service           panelSystemd.Status   `json:"service"`
	CLISettingsPath   string                `json:"cli_settings_path"`
	UnitFile          unitFileReport        `json:"unit_file"`
	SettingsFile      settingsFileReport    `json:"settings_file"`
	Configuration     configurationLocation `json:"configuration"`
	LiveSettingsState string                `json:"live_settings_state"`
}

// unitFileReport describes the unit file on disk at the systemd fragment
// path. Stale mirrors NeedDaemonReload. Neither value proves the command
// line or settings used by the currently running process.
type unitFileReport struct {
	Path               string `json:"path"`
	Source             string `json:"source"`
	Stale              bool   `json:"stale"`
	SettingsPath       string `json:"settings_path,omitempty"`
	SettingsState      string `json:"settings_state"`
	MatchesCLISettings *bool  `json:"matches_cli_settings,omitempty"`
}

// settingsFileReport describes the settings file on disk at the path the unit
// file names, read now; the service may have started with older content.
type settingsFileReport struct {
	Path         string `json:"path,omitempty"`
	Source       string `json:"source"`
	State        string `json:"state"`
	DataDir      string `json:"data_dir,omitempty"`
	DatabasePath string `json:"database_path,omitempty"`
}

type configurationLocation struct {
	Name         string `json:"name"`
	Storage      string `json:"storage"`
	Table        string `json:"table"`
	DatabasePath string `json:"database_path,omitempty"`
}

const (
	sourceUnitFileOnDisk     = "unit file on disk"
	sourceSettingsFileOnDisk = "settings file on disk"
	settingsStateParsed      = "parsed"
	settingsStateLoaded      = "loaded"
	settingsStateUnavailable = "unavailable"
	settingsStateUnknown     = "unknown"
)

func newSystemStatusCommand(state *options, service panelSystemd.Service) *cobra.Command {
	var rawScope string
	command := &cobra.Command{
		Use:   "status",
		Short: "Show systemd state and configuration locations declared by files on disk",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if service == nil {
				return systemServiceUnavailable()
			}
			scope, err := parseSystemScope(rawScope)
			if err != nil {
				return err
			}
			status, err := service.Status(cmd.Context(), scope)
			if err != nil {
				return classifySystemError("system_status_failed", err)
			}
			report := buildSystemStatusReport(status, state.settingsPath)
			return writeResult(cmd.OutOrStdout(), state.format, report, systemStatusText(report))
		},
	}
	addSystemScopeFlag(command, &rawScope)
	return command
}

func buildSystemStatusReport(status panelSystemd.Status, cliSettingsPath string) systemStatusReport {
	report := systemStatusReport{
		Service:         status,
		CLISettingsPath: cliSettingsPath,
		UnitFile: unitFileReport{
			Path: status.UnitPath, Source: sourceUnitFileOnDisk, Stale: status.NeedDaemonReload,
			SettingsState: settingsStateUnknown,
		},
		SettingsFile:      settingsFileReport{Source: sourceSettingsFileOnDisk, State: settingsStateUnknown},
		Configuration:     configurationLocation{Name: ConfigurationFileName, Storage: "sqlite", Table: "configuration_file"},
		LiveSettingsState: settingsStateUnknown,
	}
	if absolute, err := filepath.Abs(filepath.Clean(cliSettingsPath)); err == nil {
		report.CLISettingsPath = absolute
	}
	if status.UnitFileSettingsPath == "" {
		return report
	}
	report.UnitFile.SettingsPath = status.UnitFileSettingsPath
	report.UnitFile.SettingsState = settingsStateParsed
	match := status.UnitFileSettingsPath == report.CLISettingsPath
	report.UnitFile.MatchesCLISettings = &match
	report.SettingsFile.Path = status.UnitFileSettingsPath
	// Load failures are reported only as a state: the error text could echo
	// settings content, and an unreadable file must not hide the unit state.
	dataDir, err := settings.ConfiguredDataDir(status.UnitFileSettingsPath)
	if err != nil {
		report.SettingsFile.State = settingsStateUnavailable
		return report
	}
	report.SettingsFile.State = settingsStateLoaded
	report.SettingsFile.DataDir = dataDir
	report.SettingsFile.DatabasePath = filepath.Join(dataDir, "panel.db")
	report.Configuration.DatabasePath = report.SettingsFile.DatabasePath
	return report
}

func systemStatusText(report systemStatusReport) string {
	service := report.Service
	var output strings.Builder
	fmt.Fprintf(&output, "service\t%s %s\t%s\t%s (%s)\t%s\tpid=%d\n",
		service.Scope, service.Unit, service.LoadState, service.ActiveState, service.SubState, service.UnitFileState, service.MainPID)
	unitNote := "on disk; systemd reports no daemon reload needed"
	if report.UnitFile.Stale {
		unitNote = "on disk; systemd reports daemon reload needed; restart to apply command changes"
	}
	fmt.Fprintf(&output, "unit file\t%s (%s)\n", report.UnitFile.Path, unitNote)
	switch {
	case report.UnitFile.SettingsPath == "":
		fmt.Fprintf(&output, "settings in unit file\tunknown (%s does not state one unambiguous --config path)\n", report.UnitFile.Source)
	case report.UnitFile.MatchesCLISettings != nil && *report.UnitFile.MatchesCLISettings:
		fmt.Fprintf(&output, "settings in unit file\t%s (%s; same as --config)\n", report.UnitFile.SettingsPath, report.UnitFile.Source)
	default:
		fmt.Fprintf(&output, "settings in unit file\t%s (%s; differs from --config)\n", report.UnitFile.SettingsPath, report.UnitFile.Source)
	}
	fmt.Fprintf(&output, "cli settings\t%s (this command's --config)\n", report.CLISettingsPath)
	fmt.Fprintf(&output, "live settings\t%s (running process not inspected)\n", report.LiveSettingsState)
	switch report.SettingsFile.State {
	case settingsStateLoaded:
		fmt.Fprintf(&output, "data dir\t%s (%s)\ndatabase\t%s (%s)\n",
			report.SettingsFile.DataDir, report.SettingsFile.Source, report.SettingsFile.DatabasePath, report.SettingsFile.Source)
		fmt.Fprintf(&output, "configuration\t%s stored in %s table %s of %s", report.Configuration.Name, report.Configuration.Storage, report.Configuration.Table, report.Configuration.DatabasePath)
	case settingsStateUnavailable:
		fmt.Fprintf(&output, "data dir\tunknown (%s unreadable or invalid)\ndatabase\tunknown\n", report.SettingsFile.Source)
		fmt.Fprintf(&output, "configuration\t%s stored in %s table %s of the service database", report.Configuration.Name, report.Configuration.Storage, report.Configuration.Table)
	default:
		output.WriteString("data dir\tunknown\ndatabase\tunknown\n")
		fmt.Fprintf(&output, "configuration\t%s stored in %s table %s of the service database", report.Configuration.Name, report.Configuration.Storage, report.Configuration.Table)
	}
	return output.String()
}

func newSystemControlCommand(state *options, service panelSystemd.Service, action panelSystemd.Action) *cobra.Command {
	var rawScope string
	command := &cobra.Command{
		Use:   string(action),
		Short: strings.ToUpper(string(action)[:1]) + string(action)[1:] + " the systemd unit",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if service == nil {
				return systemServiceUnavailable()
			}
			scope, err := parseSystemScope(rawScope)
			if err != nil {
				return err
			}
			result, err := service.Control(cmd.Context(), scope, action)
			if err != nil {
				return classifySystemError("system_"+string(action)+"_failed", err)
			}
			return writeResult(cmd.OutOrStdout(), state.format, result, fmt.Sprintf("%s %s %s", result.Action, result.Scope, result.Unit))
		},
	}
	addSystemScopeFlag(command, &rawScope)
	return command
}

func newSystemLogsCommand(state *options, service panelSystemd.Service) *cobra.Command {
	var rawScope, since string
	var lines int
	command := &cobra.Command{
		Use:   "logs",
		Short: "Read bounded journal entries for the systemd unit",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if service == nil {
				return systemServiceUnavailable()
			}
			scope, err := parseSystemScope(rawScope)
			if err != nil {
				return err
			}
			result, err := service.Logs(cmd.Context(), panelSystemd.LogsRequest{Scope: scope, Lines: lines, Since: since})
			if err != nil {
				return classifySystemError("system_logs_failed", err)
			}
			text := result.Text
			if text == "" {
				text = "no journal entries"
			}
			return writeResult(cmd.OutOrStdout(), state.format, result, text)
		},
	}
	addSystemScopeFlag(command, &rawScope)
	command.Flags().IntVar(&lines, "lines", 100, "maximum journal entries to return (1-100000)")
	command.Flags().StringVar(&since, "since", "", "journalctl time expression passed as one literal argument")
	return command
}

func addSystemScopeFlag(command *cobra.Command, target *string) {
	command.Flags().StringVar(target, "scope", "auto", "systemd scope: auto, system, or user")
	if err := command.RegisterFlagCompletionFunc("scope", func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
		return []string{"auto", "system", "user"}, cobra.ShellCompDirectiveNoFileComp
	}); err != nil {
		panic(fmt.Sprintf("register system scope completion: %v", err))
	}
}

func parseSystemScope(raw string) (panelSystemd.Scope, error) {
	scope, err := panelSystemd.ParseScope(raw)
	if err != nil {
		return "", &Error{Kind: ErrorUsage, Code: "invalid_systemd_scope", Message: err.Error(), Cause: err}
	}
	return scope, nil
}

func classifySystemError(code string, err error) error {
	switch {
	case errors.Is(err, panelSystemd.ErrUnsupportedOS):
		return &Error{Kind: ErrorUnavailable, Code: "systemd_unsupported", Message: err.Error(), Cause: err}
	case errors.Is(err, panelSystemd.ErrPermission):
		return &Error{Kind: ErrorPermission, Code: "systemd_permission_denied", Message: err.Error(), Cause: err}
	case errors.Is(err, panelSystemd.ErrNotInstalled):
		return &Error{Kind: ErrorUnavailable, Code: "systemd_not_installed", Message: err.Error(), Cause: err}
	case errors.Is(err, panelSystemd.ErrConflict):
		return &Error{Kind: ErrorConflict, Code: "systemd_file_conflict", Message: err.Error(), Cause: err}
	case errors.Is(err, panelSystemd.ErrInvalid):
		return &Error{Kind: ErrorValidation, Code: code, Message: err.Error(), Cause: err}
	default:
		return &Error{Kind: ErrorUnavailable, Code: code, Message: err.Error(), Cause: err}
	}
}

func systemServiceUnavailable() error {
	return &Error{Kind: ErrorUnavailable, Code: "systemd_unavailable", Message: "systemd service manager is unavailable"}
}
