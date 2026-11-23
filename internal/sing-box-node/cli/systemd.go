// SPDX-License-Identifier: GPL-3.0-or-later

package cli

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	nodeSystemd "github.com/rehuony/sing-box-panel/internal/sing-box-node/systemd"
	"github.com/rehuony/sing-box-panel/internal/systemd"
	"github.com/spf13/cobra"
)

// systemdService is the node's systemd lifecycle.
type systemdService = systemd.Service[struct{}]

func newSystemdCommand(dataDir *string, service systemdService) *cobra.Command {
	var asJSON bool
	root := &cobra.Command{
		Use:   "systemd",
		Short: "Install or manage the sing-box-node systemd service",
		Long: "Install or manage the sing-box-node systemd service.\n\n" +
			"System scope runs the node as the dedicated sing-box-node account with its data in /var/lib/sing-box-node; " +
			"user scope runs it as the current user with the selected --data-dir.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
	root.PersistentFlags().BoolVar(&asJSON, "json", false, "print the result as JSON")
	write := func(cmd *cobra.Command, value any, text string) error {
		if asJSON {
			return json.NewEncoder(cmd.OutOrStdout()).Encode(value)
		}
		_, err := fmt.Fprintln(cmd.OutOrStdout(), text)
		return err
	}
	unavailable := func() error {
		return fmt.Errorf("%w: the systemd service manager is unavailable", systemd.ErrUnsupportedOS)
	}

	var rawScope string
	var force, now, keepUser bool
	install := &cobra.Command{
		Use:   "install",
		Short: "Install and enable the audited systemd unit",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if service == nil {
				return unavailable()
			}
			scope, err := systemd.ParseScope(rawScope)
			if err != nil {
				return err
			}
			directory, err := filepath.Abs(*dataDir)
			if err != nil {
				return err
			}
			result, err := service.Install(cmd.Context(), systemd.InstallRequest{Scope: scope, StatePath: directory, Force: force, Now: now})
			if err != nil {
				return err
			}
			text := fmt.Sprintf("Installed and enabled the %s %s at %s with data in %s", result.Scope, result.Unit, result.UnitPath, result.DataDir)
			if result.Started {
				text += "; the service started"
			} else {
				text += ".\nStart it with: sing-box-node systemd start --scope " + string(result.Scope)
			}
			return write(cmd, result, text)
		},
	}
	install.Flags().BoolVar(&force, "force", false, "replace conflicting regular managed destinations")
	install.Flags().BoolVar(&now, "now", false, "start the unit after enabling it; the node must be enrolled or have its console enabled")

	uninstall := &cobra.Command{
		Use:   "uninstall",
		Short: "Uninstall the service and, in system scope, its dedicated account",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if service == nil {
				return unavailable()
			}
			scope, err := systemd.ParseScope(rawScope)
			if err != nil {
				return err
			}
			result, err := service.Uninstall(cmd.Context(), systemd.UninstallRequest{Scope: scope, Force: force, KeepUser: keepUser})
			if err != nil {
				if result.Scope != "" {
					_ = write(cmd, result, uninstallText(result, "Uninstall interrupted; confirmed results only"))
				}
				return err
			}
			return write(cmd, result, uninstallText(result, "Service uninstalled"))
		},
	}
	uninstall.Flags().BoolVar(&force, "force", false, "remove conflicting regular destinations at the exact managed paths")
	uninstall.Flags().BoolVar(&keepUser, "keep-user", false, "retain the system service account and its file ownership")

	status := &cobra.Command{
		Use:   "status",
		Short: "Show the unit's systemd state and the data directory its unit file names",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if service == nil {
				return unavailable()
			}
			scope, err := systemd.ParseScope(rawScope)
			if err != nil {
				return err
			}
			result, err := service.Status(cmd.Context(), scope)
			if err != nil {
				return err
			}
			directory := result.UnitFileStatePath
			if directory == "" {
				directory = "unknown (the unit file does not state one unambiguous --data-dir)"
			}
			text := fmt.Sprintf("Service:   %s %s, %s (%s), %s, pid %d\nUnit file: %s\nData dir:  %s",
				result.Scope, result.Unit, result.ActiveState, result.SubState, result.UnitFileState, result.MainPID, result.UnitPath, directory)
			if result.NeedDaemonReload {
				text += "\nsystemd reports a daemon reload is needed; restart the service to apply unit changes"
			}
			for _, companion := range result.Companions {
				text += fmt.Sprintf("\nHelper:    %s, %s (%s), %s", companion.Unit, companion.ActiveState, companion.SubState, companion.UnitFileState)
			}
			return write(cmd, result, text)
		},
	}

	var lines int
	var since string
	logs := &cobra.Command{
		Use:   "logs",
		Short: "Read bounded journal entries for the systemd unit",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if service == nil {
				return unavailable()
			}
			scope, err := systemd.ParseScope(rawScope)
			if err != nil {
				return err
			}
			result, err := service.Logs(cmd.Context(), systemd.LogsRequest{Scope: scope, Lines: lines, Since: since})
			if err != nil {
				return err
			}
			text := result.Text
			if text == "" {
				text = "no journal entries"
			}
			return write(cmd, result, text)
		},
	}
	logs.Flags().IntVar(&lines, "lines", 100, "maximum journal entries to return (1-100000)")
	logs.Flags().StringVar(&since, "since", "", "journalctl time expression passed as one literal argument")

	commands := []*cobra.Command{install, uninstall, status}
	for _, action := range []systemd.Action{systemd.ActionStart, systemd.ActionStop, systemd.ActionRestart} {
		commands = append(commands, &cobra.Command{
			Use:   string(action),
			Short: strings.ToUpper(string(action)[:1]) + string(action)[1:] + " the systemd unit",
			Args:  cobra.NoArgs,
			RunE: func(cmd *cobra.Command, _ []string) error {
				if service == nil {
					return unavailable()
				}
				scope, err := systemd.ParseScope(rawScope)
				if err != nil {
					return err
				}
				result, err := service.Control(cmd.Context(), scope, action)
				if err != nil {
					return err
				}
				return write(cmd, result, fmt.Sprintf("%s %s %s", result.Action, result.Scope, result.Unit))
			},
		})
	}
	commands = append(commands, logs)
	for _, command := range commands {
		command.Flags().StringVar(&rawScope, "scope", "auto", "systemd scope: auto, system, or user")
	}
	root.AddCommand(commands...)
	return root
}

func uninstallText(result systemd.UninstallResult, heading string) string {
	text := fmt.Sprintf("%s\n  Unit: %s (%s)\n  Service files removed: %d\n  Data: retained", heading, result.Unit, result.Scope, len(result.RemovedPaths))
	if result.Scope == systemd.ScopeSystem {
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
		text += fmt.Sprintf("\n  Service user: %s\n  Service group: %s",
			state(result.AccountRetained, result.AccountRemoved), state(result.GroupRetained, result.GroupRemoved))
		if result.AccountNote != "" {
			text += "\n  Account note: " + result.AccountNote
		}
	}
	for _, path := range result.RemovedPaths {
		text += "\n  removed " + path
	}
	return text
}

// defaultSystemd is the node's lifecycle on this host, or nil when systemd
// paths cannot be resolved.
func defaultSystemd() systemdService {
	manager, err := nodeSystemd.New(systemd.Options{})
	if err != nil {
		return nil
	}
	return manager
}
