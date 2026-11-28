// SPDX-License-Identifier: GPL-3.0-or-later

// Package cli owns the node's public command tree.
package cli

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/rehuony/sing-box-panel/internal/buildinfo"
	"github.com/rehuony/sing-box-panel/internal/cliui"
	"github.com/rehuony/sing-box-panel/internal/nodeapi"
	"github.com/rehuony/sing-box-panel/internal/procctl"
	"github.com/rehuony/sing-box-panel/internal/sing-box-node/agent"
	"github.com/rehuony/sing-box-panel/internal/sing-box-node/settings"
	"github.com/rehuony/sing-box-panel/internal/systemd"
	"github.com/spf13/cobra"
)

type Dependencies struct {
	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer
	Build  buildinfo.Info
	// Systemd is the node's systemd lifecycle; nil uses this host's.
	Systemd systemdService
}

type options struct {
	settingsPath string
	format       cliui.Format
}

func NewRootCommand(deps Dependencies) *cobra.Command {
	state := &options{}
	root := cliui.Root(cliui.Group("sing-box-node", "Run a sing-box node managed by a panel"))
	root.SetIn(deps.Stdin)
	root.SetOut(deps.Stdout)
	root.SetErr(deps.Stderr)
	root.PersistentFlags().StringVarP(&state.settingsPath, "config", "c", settings.DefaultPath(), "settings file path")
	cliui.AddFormatFlag(root.PersistentFlags(), &state.format)
	service := deps.Systemd
	if service == nil {
		service = defaultSystemd()
	}
	root.AddCommand(
		cliui.Version("sing-box-node", deps.Build, fmt.Sprintf("%d.%d", nodeapi.ProtocolMajor, nodeapi.ProtocolMinor), &state.format),
		newEnrollCommand(deps, state, service),
		newUpdateCommand(deps, state, service),
		newServerCommand(deps, state),
		newConfigCommand(state),
		newSystemCommand(state, service),
		newSystemdCommand(state, service),
		cliui.Completion(root),
	)
	return root
}

func newEnrollCommand(deps Dependencies, state *options, service systemdService) *cobra.Command {
	var code string
	command := &cobra.Command{
		Use:   "enroll",
		Short: "Enroll this node with a panel using a one-time code",
		Long: "Enroll this node with a panel using a one-time code. A running node switches to the new panel within a few seconds.\n\n" +
			"Missing settings are created first, with a generated console password.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			value, created, err := settings.LoadOrInitialize(cmd.Context(), state.settingsPath)
			if created {
				if outputErr := writeInitialization(cmd.ErrOrStderr(), state.format, value); outputErr != nil {
					return errors.Join(err, outputErr)
				}
			}
			if err != nil {
				return cliui.Fail(cliui.ErrorValidation, "settings_invalid", err)
			}
			enrollment, err := agent.Enroll(cmd.Context(), value.DataDir, code, deps.Build)
			var refused *agent.EnrollmentError
			switch {
			case errors.As(err, &refused) && refused.Rejected:
				return cliui.Fail(cliui.ErrorValidation, "enrollment_rejected", err)
			case errors.As(err, &refused):
				return cliui.Fail(cliui.ErrorUnavailable, "enrollment_failed", err)
			case err != nil:
				return cliui.Fail(cliui.ErrorValidation, "enrollment_invalid", err)
			}
			style := cliui.NewStyle(cmd.OutOrStdout(), state.format)
			text := cliui.Section(style.Success("Enrolled as node "+enrollment.NodeID), cliui.Fields(
				cliui.Field{Label: "Panel", Value: style.Accent(enrollment.URL)},
				cliui.Field{Label: "Settings", Value: style.Path(value.Path())},
				cliui.Field{Label: "Data directory", Value: style.Path(value.DataDir)},
			), startHint(cmd.Context(), value, service, style))
			return cliui.Write(cmd.OutOrStdout(), state.format, map[string]any{
				"enrolled": true, "node_id": enrollment.NodeID, "panel_id": enrollment.PanelID, "panel": enrollment.URL,
			}, text)
		},
	}
	command.Flags().StringVar(&code, "code", "", "enrollment code shown by the panel")
	_ = command.MarkFlagRequired("code")
	return command
}

// startHint says how the enrolled node starts using its enrollment: a
// running node switches by itself, an installed service starts through
// systemd, and anything else starts in the terminal.
func startHint(ctx context.Context, value settings.Settings, service systemdService, style cliui.Style) string {
	if running, err := procctl.LeaseHeld(value.DataDir); err == nil && running {
		return "The running node switches to it within a few seconds."
	}
	if service != nil {
		if status, err := service.Status(ctx, systemd.ScopeAuto); err == nil && status.UnitFileStatePath == value.Path() {
			return style.Next(cliui.ServiceCommand("sing-box-node", status.Scope, "start"))
		}
	}
	return style.Next("sing-box-node server start")
}
