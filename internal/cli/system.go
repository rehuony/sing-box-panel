// SPDX-License-Identifier: GPL-3.0-or-later

package cli

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/rehuony/sing-box-panel/internal/settings"
	panelSystemd "github.com/rehuony/sing-box-panel/internal/systemd"
	"github.com/spf13/cobra"
)

func newSystemCommand(state *options, service panelSystemd.Service) *cobra.Command {
	root := group("system", "Manage the sing-box-panel systemd unit")
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
			value, err := settings.Load(settingsPath)
			if err != nil {
				return &Error{Kind: ErrorValidation, Code: "system_settings_invalid", Message: err.Error(), Cause: err}
			}
			result, err := service.Install(cmd.Context(), panelSystemd.InstallRequest{
				Scope: scope, SettingsPath: settingsPath, DataDir: value.DataDir, Force: force, Now: now,
			})
			if err != nil {
				return classifySystemError("system_install_failed", err)
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
	var force bool
	command := &cobra.Command{
		Use:   "uninstall",
		Short: "Stop, disable, and remove files owned by the built-in installer",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if service == nil {
				return systemServiceUnavailable()
			}
			scope, err := parseSystemScope(rawScope)
			if err != nil {
				return err
			}
			result, err := service.Uninstall(cmd.Context(), panelSystemd.UninstallRequest{Scope: scope, Force: force})
			if err != nil {
				return classifySystemError("system_uninstall_failed", err)
			}
			text := fmt.Sprintf("uninstalled %s %s; settings and data retained", result.Scope, result.Unit)
			return writeResult(cmd.OutOrStdout(), state.format, result, text)
		},
	}
	addSystemScopeFlag(command, &rawScope)
	command.Flags().BoolVar(&force, "force", false, "remove conflicting regular destinations at the exact managed paths")
	return command
}

func newSystemStatusCommand(state *options, service panelSystemd.Service) *cobra.Command {
	var rawScope string
	command := &cobra.Command{
		Use:   "status",
		Short: "Show exact systemd load, enablement, and process state",
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
			text := fmt.Sprintf("%s\t%s\t%s\t%s\t%s\tpid=%d", status.Scope, status.LoadState, status.ActiveState, status.SubState, status.UnitFileState, status.MainPID)
			return writeResult(cmd.OutOrStdout(), state.format, status, text)
		},
	}
	addSystemScopeFlag(command, &rawScope)
	return command
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
