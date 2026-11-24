// SPDX-License-Identifier: GPL-3.0-or-later

package cli

import (
	"fmt"
	"path/filepath"

	"github.com/rehuony/sing-box-panel/internal/cliui"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/installation"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/settings"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/store"
	"github.com/rehuony/sing-box-panel/internal/systemd"
	"github.com/spf13/cobra"
)

// ConfigurationFileName is the logical name of each node's sing-box document
// in SQLite.
const ConfigurationFileName = "config.json"

// systemdService is the panel's systemd lifecycle.
type systemdService = systemd.Service[settings.Settings]

func newSystemdCommand(state *options, service systemdService) *cobra.Command {
	return cliui.Systemd(cliui.SystemdOptions[settings.Settings]{
		Name: "sing-box-panel", Service: service, SettingsPath: &state.settingsPath, Format: &state.format,
		DataDir: func(path string, now bool) (string, error) {
			if now {
				value, err := settings.Load(path)
				return value.DataDir, err
			}
			return settings.ConfiguredDataDir(path)
		},
		Created: func(cmd *cobra.Command, value settings.Settings) error {
			return writeServerInitialization(cmd, state, value)
		},
		Status: func(cmd *cobra.Command, status systemd.Status) error {
			report := buildSystemStatusReport(status, state.settingsPath)
			style := cliui.NewStyle(cmd.OutOrStdout(), state.format)
			return cliui.Write(cmd.OutOrStdout(), state.format, report, systemStatusText(report, style))
		},
	})
}

func accountCleanupSummary(account installation.AccountCleanupResult) string {
	text := fmt.Sprintf("service user %s; service group %s", account.User, account.Group)
	if account.Note != "" {
		text += " (" + account.Note + ")"
	}
	return text
}

func uninstallAccountResult(result systemd.UninstallResult) *installation.AccountCleanupResult {
	if result.Scope != systemd.ScopeSystem {
		return nil
	}
	user, group := cliui.AccountStates(result)
	return &installation.AccountCleanupResult{User: user, Group: group, Note: result.AccountNote}
}

// systemStatusReport joins systemd's view of the unit with two facts read
// from disk: the settings path written in the installed unit file, and the
// storage locations declared by that settings file as it exists now. Both are
// labeled with their source. Neither proves what the running process started
// with, so the live settings are always reported as unknown, and the CLI's
// own --config path is listed separately rather than substituted.
type systemStatusReport struct {
	Service           systemd.Status        `json:"service"`
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

func buildSystemStatusReport(status systemd.Status, cliSettingsPath string) systemStatusReport {
	report := systemStatusReport{
		Service:         status,
		CLISettingsPath: cliSettingsPath,
		UnitFile: unitFileReport{
			Path: status.UnitPath, Source: sourceUnitFileOnDisk, Stale: status.NeedDaemonReload,
			SettingsState: settingsStateUnknown,
		},
		SettingsFile:      settingsFileReport{Source: sourceSettingsFileOnDisk, State: settingsStateUnknown},
		Configuration:     configurationLocation{Name: ConfigurationFileName, Storage: "sqlite", Table: store.ConfigurationFileTable},
		LiveSettingsState: settingsStateUnknown,
	}
	if absolute, err := filepath.Abs(filepath.Clean(cliSettingsPath)); err == nil {
		report.CLISettingsPath = absolute
	}
	if status.UnitFileStatePath == "" {
		return report
	}
	report.UnitFile.SettingsPath = status.UnitFileStatePath
	report.UnitFile.SettingsState = settingsStateParsed
	match := status.UnitFileStatePath == report.CLISettingsPath
	report.UnitFile.MatchesCLISettings = &match
	report.SettingsFile.Path = status.UnitFileStatePath
	// Load failures are reported only as a state: the error text could echo
	// settings content, and an unreadable file must not hide the unit state.
	dataDir, err := settings.ConfiguredDataDir(status.UnitFileStatePath)
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

func systemStatusText(report systemStatusReport, style cliui.Style) string {
	fields := cliui.UnitFields(report.Service, style)
	unitNote := "on disk; systemd reports no daemon reload needed"
	if report.UnitFile.Stale {
		unitNote = "on disk; systemd reports daemon reload needed; restart to apply command changes"
	}
	for i := range fields {
		if fields[i].Label == "Unit file" {
			fields[i].Value += " (" + unitNote + ")"
		}
	}
	unitSettings := "unknown (" + report.UnitFile.Source + " does not state one unambiguous --config path)"
	switch {
	case report.UnitFile.SettingsPath == "":
	case report.UnitFile.MatchesCLISettings != nil && *report.UnitFile.MatchesCLISettings:
		unitSettings = style.Path(report.UnitFile.SettingsPath) + " (" + report.UnitFile.Source + "; same as --config)"
	default:
		unitSettings = style.Path(report.UnitFile.SettingsPath) + " (" + report.UnitFile.Source + "; differs from --config)"
	}
	dataDir, databasePath, database := "unknown", "unknown", "the service database"
	switch report.SettingsFile.State {
	case settingsStateLoaded:
		dataDir = style.Path(report.SettingsFile.DataDir) + " (" + report.SettingsFile.Source + ")"
		databasePath = style.Path(report.SettingsFile.DatabasePath) + " (" + report.SettingsFile.Source + ")"
		database = style.Path(report.Configuration.DatabasePath)
	case settingsStateUnavailable:
		dataDir = "unknown (" + report.SettingsFile.Source + " unreadable or invalid)"
	}
	fields = append(fields,
		cliui.Field{Label: "Settings in unit file", Value: unitSettings},
		cliui.Field{Label: "CLI settings", Value: style.Path(report.CLISettingsPath) + " (this command's --config)"},
		cliui.Field{Label: "Live settings", Value: report.LiveSettingsState + " (running process not inspected)"},
		cliui.Field{Label: "Data directory", Value: dataDir},
		cliui.Field{Label: "Database", Value: databasePath},
		cliui.Field{Label: "Configuration", Value: fmt.Sprintf("each node's %s stored in %s table %s of %s",
			report.Configuration.Name, report.Configuration.Storage, report.Configuration.Table, database)},
	)
	return cliui.Section(cliui.UnitTitle(report.Service, style), cliui.Fields(fields...),
		cliui.UnitNotes("sing-box-panel", report.Service, style))
}
