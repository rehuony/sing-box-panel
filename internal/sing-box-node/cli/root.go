// SPDX-License-Identifier: GPL-3.0-or-later

// Package cli owns the node's public command tree.
package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"

	"github.com/rehuony/sing-box-panel/internal/buildinfo"
	"github.com/rehuony/sing-box-panel/internal/nodeapi"
	"github.com/rehuony/sing-box-panel/internal/sing-box-node/agent"
	"github.com/rehuony/sing-box-panel/internal/sing-box-node/httpapi"
	"github.com/rehuony/sing-box-panel/internal/sing-box-node/settings"
	nodeweb "github.com/rehuony/sing-box-panel/web/apps/node"
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
	dataDir := root.PersistentFlags().String("data-dir", settings.DefaultDataDir(), "node data directory")
	service := deps.Systemd
	if service == nil {
		service = defaultSystemd()
	}
	root.AddCommand(newVersionCommand(deps.Build), newEnrollCommand(deps, dataDir), newRunCommand(deps, dataDir),
		newConsoleCommand(dataDir), newSystemdCommand(dataDir, service))
	return root
}

func newEnrollCommand(deps Dependencies, dataDir *string) *cobra.Command {
	var code string
	command := &cobra.Command{
		Use:   "enroll",
		Short: "Enroll this node with a panel using a one-time code",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			enrollment, err := agent.Enroll(cmd.Context(), *dataDir, code, deps.Build)
			if err != nil {
				return err
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "Enrolled as node %s with the panel at %s%s. Start the node with: sing-box-node run\n", enrollment.NodeID, enrollment.Endpoint, enrollment.Path)
			return err
		},
	}
	command.Flags().StringVar(&code, "code", "", "enrollment code shown by the panel")
	_ = command.MarkFlagRequired("code")
	return command
}

func newRunCommand(deps Dependencies, dataDir *string) *cobra.Command {
	return &cobra.Command{
		Use:   "run",
		Short: "Run the node and keep its session with the enrolled panel until stopped",
		Long: "Run the node and keep its session with the enrolled panel until stopped.\n\n" +
			"With the console enabled the node also serves it on its loopback address, and an unenrolled node waits to be enrolled from it.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			logger := log.New(cmd.ErrOrStderr(), "", log.LstdFlags)
			link, err := agent.OpenLink(*dataDir, deps.Build)
			if err != nil {
				return err
			}
			console, err := settings.LoadConsole(*dataDir)
			if err != nil {
				return err
			}
			options := agent.Options{Logf: logger.Printf}
			if console.Enabled {
				listener, err := httpapi.Listen(console.Listen)
				if err != nil {
					return fmt.Errorf("console: %w", err)
				}
				defer listener.Close()
				options.Console = func(ctx context.Context, readings agent.ReadingsSource) error {
					handler := httpapi.NewHandler(httpapi.Options{
						DataDir: *dataDir, Link: link, Readings: readings, Build: deps.Build, Assets: nodeweb.Assets(), Logf: logger.Printf,
					})
					logger.Printf("console listening on http://%s", listener.Addr())
					return httpapi.Serve(ctx, listener, handler)
				}
			}
			return agent.Run(cmd.Context(), link, options)
		},
	}
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
