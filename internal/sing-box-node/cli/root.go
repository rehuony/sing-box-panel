// SPDX-License-Identifier: GPL-3.0-or-later

// Package cli owns the node's public command tree.
package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/rehuony/sing-box-panel/internal/buildinfo"
	"github.com/rehuony/sing-box-panel/internal/nodeapi"
	"github.com/rehuony/sing-box-panel/internal/procctl"
	"github.com/rehuony/sing-box-panel/internal/sing-box-node/agent"
	"github.com/rehuony/sing-box-panel/internal/sing-box-node/settings"
	"github.com/rehuony/sing-box-panel/internal/systemd"
	"github.com/spf13/cobra"
)

type Dependencies struct {
	Stdout io.Writer
	Stderr io.Writer
	Build  buildinfo.Info
	// Systemd is the node's systemd lifecycle; nil uses this host's.
	Systemd systemdService
}

func NewRootCommand(deps Dependencies) *cobra.Command {
	root := &cobra.Command{
		Use:           "sing-box-node",
		Short:         "Run a sing-box node managed by a panel",
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}
	root.SetOut(deps.Stdout)
	root.SetErr(deps.Stderr)
	settingsPath := root.PersistentFlags().StringP("config", "c", settings.DefaultPath(), "node settings file")
	service := deps.Systemd
	if service == nil {
		service = defaultSystemd()
	}
	root.AddCommand(newVersionCommand(deps.Build), newEnrollCommand(deps, settingsPath, service), newUpdateCommand(deps, settingsPath, service),
		newServerCommand(deps, settingsPath), newConfigCommand(settingsPath), newSystemCommand(settingsPath, service),
		newSystemdCommand(settingsPath, service))
	return root
}

func newEnrollCommand(deps Dependencies, settingsPath *string, service systemdService) *cobra.Command {
	var code string
	command := &cobra.Command{
		Use:   "enroll",
		Short: "Enroll this node with a panel using a one-time code",
		Long: "Enroll this node with a panel using a one-time code. A running node switches to the new panel within a few seconds.\n\n" +
			"Missing settings are created first, with a generated console password.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			value, created, err := settings.LoadOrInitialize(cmd.Context(), *settingsPath)
			if created {
				if outputErr := writeInitialization(cmd.ErrOrStderr(), value); outputErr != nil {
					return errors.Join(err, outputErr)
				}
			}
			if err != nil {
				return err
			}
			enrollment, err := agent.Enroll(cmd.Context(), value.DataDir, code, deps.Build)
			if err != nil {
				return err
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "Enrolled as node %s with the panel at %s%s. %s\n",
				enrollment.NodeID, enrollment.Endpoint, enrollment.Path, startHint(cmd.Context(), value, service))
			return err
		},
	}
	command.Flags().StringVar(&code, "code", "", "enrollment code shown by the panel")
	_ = command.MarkFlagRequired("code")
	return command
}

// startHint says how the enrolled node starts using its enrollment: a
// running node switches by itself, an installed service starts through
// systemd, and anything else starts in the terminal.
func startHint(ctx context.Context, value settings.Settings, service systemdService) string {
	if running, err := procctl.LeaseHeld(value.DataDir); err == nil && running {
		return "The running node switches to it within a few seconds."
	}
	if service != nil {
		if status, err := service.Status(ctx, systemd.ScopeAuto); err == nil && status.UnitFileStatePath == value.Path() {
			prefix := ""
			if status.Scope == systemd.ScopeSystem {
				prefix = "sudo "
			}
			return "Start its service with: " + prefix + "sing-box-node systemd start"
		}
	}
	return "Start the node with: sing-box-node server start"
}

type versionResult struct {
	buildinfo.Info
	Protocol string `json:"protocol"`
}

func newVersionCommand(info buildinfo.Info) *cobra.Command {
	var asJSON bool
	command := &cobra.Command{
		Use:   "version",
		Short: "Print sing-box-node build information",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			result := versionResult{Info: info, Protocol: fmt.Sprintf("%d.%d", nodeapi.ProtocolMajor, nodeapi.ProtocolMinor)}
			if asJSON {
				encoder := json.NewEncoder(cmd.OutOrStdout())
				encoder.SetEscapeHTML(false)
				return encoder.Encode(result)
			}
			_, err := fmt.Fprintf(cmd.OutOrStdout(), "sing-box-node %s (protocol %s)\n", info.DisplayVersion(), result.Protocol)
			return err
		},
	}
	command.Flags().BoolVar(&asJSON, "json", false, "print build information as JSON")
	return command
}
