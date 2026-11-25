// SPDX-License-Identifier: GPL-3.0-or-later

package cliui

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/rehuony/sing-box-panel/internal/systemd"
	"github.com/spf13/cobra"
)

// SystemdOptions configures one service's systemd commands.
type SystemdOptions[T any] struct {
	// Name is the binary, which hints name in the commands they suggest.
	Name string
	// Long describes the group in its help.
	Long string
	// Service is the service's lifecycle; nil when systemd is unavailable.
	Service      systemd.Service[T]
	SettingsPath *string
	Format       *Format
	// DataDir returns the data directory the settings at path select, with
	// an error wrapping os.ErrNotExist when they are missing; now also
	// requires settings the service can start with.
	DataDir func(path string, now bool) (string, error)
	// Created reports settings that installing or starting created, which
	// carry the one-time password.
	Created func(cmd *cobra.Command, value T) error
	// Status writes the result of status.
	Status func(cmd *cobra.Command, status systemd.Status) error
}

// Systemd is the systemd command group both binaries share: install,
// uninstall, status, start, stop, restart and logs of the service's own
// unit, with one output style and one error classification.
func Systemd[T any](options SystemdOptions[T]) *cobra.Command {
	root := Group("systemd", "Install or manage the "+options.Name+" systemd service")
	root.Long = options.Long
	root.AddCommand(
		installCommand(options), uninstallCommand(options), statusCommand(options),
		controlCommand(options, systemd.ActionStart), controlCommand(options, systemd.ActionStop),
		controlCommand(options, systemd.ActionRestart), logsCommand(options),
	)
	return root
}

func installCommand[T any](options SystemdOptions[T]) *cobra.Command {
	var rawScope string
	var force, now bool
	command := &cobra.Command{
		Use:   "install",
		Short: "Install and enable the audited systemd unit, replacing one an earlier release wrote",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if options.Service == nil {
				return systemdUnavailable()
			}
			scope, err := ParseScope(rawScope)
			if err != nil {
				return err
			}
			path, err := filepath.Abs(filepath.Clean(*options.SettingsPath))
			if err != nil {
				return Fail(ErrorValidation, "system_settings_path_invalid", err)
			}
			dataDir, err := options.DataDir(path, now)
			if errors.Is(err, os.ErrNotExist) {
				dataDir, err = "", nil
			}
			if err != nil {
				return Fail(ErrorValidation, "system_settings_invalid", err)
			}
			result, err := options.Service.Install(cmd.Context(), systemd.InstallRequest{
				Scope: scope, StatePath: path, DataDir: dataDir, Force: force, Now: now,
			})
			if result.Initial != nil {
				if outputErr := options.Created(cmd, *result.Initial); outputErr != nil {
					return errors.Join(err, outputErr)
				}
			}
			if err != nil {
				return SystemdError("system_install_failed", err)
			}
			style := NewStyle(cmd.OutOrStdout(), *options.Format)
			state, next := "enabled", style.Next(ServiceCommand(options.Name, result.Scope, "start"))
			if result.Started {
				state, next = "enabled and started", ""
			}
			text := Section(style.Success("Installed "+result.Unit), Fields(
				Field{Label: "Scope", Value: string(result.Scope)},
				Field{Label: "Unit file", Value: style.Path(result.UnitPath)},
				Field{Label: "Settings", Value: style.Path(result.StatePath)},
				Field{Label: "Data directory", Value: style.Path(result.DataDir)},
				Field{Label: "State", Value: state},
			), next)
			return Write(cmd.OutOrStdout(), *options.Format, result, text)
		},
	}
	AddScopeFlag(command, &rawScope)
	command.Flags().BoolVar(&force, "force", false, "replace conflicting files that this installer did not write")
	command.Flags().BoolVar(&now, "now", false, "start the unit after enabling it")
	return command
}

func uninstallCommand[T any](options SystemdOptions[T]) *cobra.Command {
	var rawScope string
	var force, keepUser bool
	command := &cobra.Command{
		Use:   "uninstall",
		Short: "Uninstall the service and, in system scope, its dedicated account",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if options.Service == nil {
				return systemdUnavailable()
			}
			scope, err := ParseScope(rawScope)
			if err != nil {
				return err
			}
			result, err := options.Service.Uninstall(cmd.Context(), systemd.UninstallRequest{Scope: scope, Force: force, KeepUser: keepUser})
			style := NewStyle(cmd.OutOrStdout(), *options.Format)
			if err != nil {
				if result.Scope != "" {
					if writeErr := Write(cmd.OutOrStdout(), *options.Format, result, uninstallText(result, true, style)); writeErr != nil {
						return errors.Join(err, writeErr)
					}
				}
				return SystemdError("system_uninstall_failed", err)
			}
			return Write(cmd.OutOrStdout(), *options.Format, result, uninstallText(result, false, style))
		},
	}
	AddScopeFlag(command, &rawScope)
	command.Flags().BoolVar(&force, "force", false, "remove conflicting regular destinations at the exact managed paths")
	command.Flags().BoolVar(&keepUser, "keep-user", false, "retain the system service account and its file ownership")
	return command
}

func uninstallText(result systemd.UninstallResult, failed bool, style Style) string {
	heading := style.Success("Uninstalled " + result.Unit)
	if failed {
		heading = style.Failure("Uninstall interrupted; confirmed results only")
	}
	fields := []Field{
		{Label: "Scope", Value: string(result.Scope)},
		{Label: "Service files", Value: fmt.Sprintf("%d removed", len(result.RemovedPaths))},
		{Label: "Settings and data", Value: style.Label("retained")},
	}
	note := ""
	if result.Scope == systemd.ScopeSystem {
		user, group := AccountStates(result)
		fields = append(fields, Field{Label: "Service user", Value: style.Label(user)}, Field{Label: "Service group", Value: style.Label(group)})
		if result.AccountNote != "" {
			note = style.Hint("Note:", result.AccountNote)
		}
	}
	entries := make([]TreeEntry, 0, len(result.RemovedPaths))
	for _, path := range result.RemovedPaths {
		entries = append(entries, TreeEntry{Path: path, Label: "removed"})
	}
	return Section(heading, Fields(fields...), style.Tree(entries), note)
}

// AccountStates reports what uninstalling did with the service user and its
// group: removed, retained, absent or unknown.
func AccountStates(result systemd.UninstallResult) (user, group string) {
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
	return state(result.AccountRetained, result.AccountRemoved), state(result.GroupRetained, result.GroupRemoved)
}

func statusCommand[T any](options SystemdOptions[T]) *cobra.Command {
	var rawScope string
	command := &cobra.Command{
		Use:   "status",
		Short: "Show the unit's systemd state and the settings its unit file names",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if options.Service == nil {
				return systemdUnavailable()
			}
			scope, err := ParseScope(rawScope)
			if err != nil {
				return err
			}
			status, err := options.Service.Status(cmd.Context(), scope)
			if err != nil {
				return SystemdError("system_status_failed", err)
			}
			return options.Status(cmd, status)
		},
	}
	AddScopeFlag(command, &rawScope)
	return command
}

// UnitTitle is the heading of a unit's status: the unit, colored by its state.
func UnitTitle(status systemd.Status, style Style) string {
	switch status.ActiveState {
	case "active":
		return style.Success(status.Unit)
	case "failed":
		return style.Failure(status.Unit)
	}
	return style.Title(status.Unit)
}

// UnitFields are systemd's view of the unit.
func UnitFields(status systemd.Status, style Style) []Field {
	pid := ""
	if status.MainPID > 0 {
		pid = strconv.Itoa(status.MainPID)
	}
	return []Field{
		{Label: "Scope", Value: string(status.Scope)},
		{Label: "State", Value: unitState(status.ActiveState, status.SubState, status.UnitFileState)},
		{Label: "PID", Value: pid},
		{Label: "Unit file", Value: style.Path(status.UnitPath)},
	}
}

// UnitNotes warn about a unit file systemd has not loaded or that an
// earlier release wrote, naming the command that resolves it.
func UnitNotes(name string, status systemd.Status, style Style) string {
	var notes []string
	if status.UnitOutdated {
		notes = append(notes, style.Warning("The unit file was written by another release; the service may not start."),
			style.Next(ServiceCommand(name, status.Scope, "install")))
	}
	if status.NeedDaemonReload {
		notes = append(notes, style.Warning("systemd reports a daemon reload is needed; restart the service to apply unit changes."))
	}
	return Lines(notes...)
}

func unitState(active, sub, enablement string) string {
	text := active
	if sub != "" && sub != active {
		text += " (" + sub + ")"
	}
	if enablement != "" {
		text += ", " + enablement
	}
	return text
}

func controlCommand[T any](options SystemdOptions[T], action systemd.Action) *cobra.Command {
	var rawScope string
	verb := strings.ToUpper(string(action)[:1]) + string(action)[1:]
	command := &cobra.Command{
		Use:   string(action),
		Short: verb + " the systemd unit",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if options.Service == nil {
				return systemdUnavailable()
			}
			scope, err := ParseScope(rawScope)
			if err != nil {
				return err
			}
			result, err := options.Service.Control(cmd.Context(), scope, action)
			if result.Initial != nil {
				if outputErr := options.Created(cmd, *result.Initial); outputErr != nil {
					return errors.Join(err, outputErr)
				}
			}
			if err != nil {
				return SystemdError("system_"+string(action)+"_failed", err)
			}
			past := map[systemd.Action]string{systemd.ActionStart: "Started", systemd.ActionStop: "Stopped", systemd.ActionRestart: "Restarted"}[result.Action]
			return Write(cmd.OutOrStdout(), *options.Format, result, fmt.Sprintf("%s %s %s", past, result.Scope, result.Unit))
		},
	}
	AddScopeFlag(command, &rawScope)
	return command
}

func logsCommand[T any](options SystemdOptions[T]) *cobra.Command {
	var rawScope, since string
	var lines int
	command := &cobra.Command{
		Use:   "logs",
		Short: "Read bounded journal entries for the systemd unit",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if options.Service == nil {
				return systemdUnavailable()
			}
			scope, err := ParseScope(rawScope)
			if err != nil {
				return err
			}
			result, err := options.Service.Logs(cmd.Context(), systemd.LogsRequest{Scope: scope, Lines: lines, Since: since})
			if err != nil {
				return SystemdError("system_logs_failed", err)
			}
			text := result.Text
			if text == "" {
				text = "No journal entries"
			}
			return Write(cmd.OutOrStdout(), *options.Format, result, text)
		},
	}
	AddScopeFlag(command, &rawScope)
	command.Flags().IntVar(&lines, "lines", 100, "maximum journal entries to return (1-100000)")
	command.Flags().StringVar(&since, "since", "", "journalctl time expression passed as one literal argument")
	return command
}

// ServiceCommand is the systemd command an operator runs for scope.
func ServiceCommand(name string, scope systemd.Scope, verb string) string {
	if scope == systemd.ScopeSystem {
		return "sudo " + name + " systemd " + verb
	}
	return name + " systemd " + verb
}

// AddScopeFlag registers --scope, the systemd scope a command acts in.
func AddScopeFlag(command *cobra.Command, target *string) {
	command.Flags().StringVar(target, "scope", "auto", "systemd scope: auto, system, or user")
	if err := command.RegisterFlagCompletionFunc("scope", func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
		return []string{"auto", "system", "user"}, cobra.ShellCompDirectiveNoFileComp
	}); err != nil {
		panic(fmt.Sprintf("register systemd scope completion: %v", err))
	}
}

// ParseScope parses a --scope value as a usage error when it is invalid.
func ParseScope(raw string) (systemd.Scope, error) {
	scope, err := systemd.ParseScope(raw)
	if err != nil {
		return "", Fail(ErrorUsage, "invalid_systemd_scope", err)
	}
	return scope, nil
}

// SystemdError classifies a systemd failure; code names a failure the
// lifecycle does not classify itself.
func SystemdError(code string, err error) error {
	switch {
	case errors.Is(err, systemd.ErrUnsupportedOS):
		return Fail(ErrorUnavailable, "systemd_unsupported", err)
	case errors.Is(err, systemd.ErrPermission):
		return Fail(ErrorPermission, "systemd_permission_denied", err)
	case errors.Is(err, systemd.ErrNotInstalled):
		return Fail(ErrorUnavailable, "systemd_not_installed", err)
	case errors.Is(err, systemd.ErrConflict):
		return Fail(ErrorConflict, "systemd_file_conflict", err)
	case errors.Is(err, systemd.ErrInvalid):
		return Fail(ErrorValidation, code, err)
	default:
		return Fail(ErrorUnavailable, code, err)
	}
}

func systemdUnavailable() error {
	return &Error{Kind: ErrorUnavailable, Code: "systemd_unavailable", Message: "the systemd service manager is unavailable"}
}
