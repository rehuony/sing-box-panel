// SPDX-License-Identifier: GPL-3.0-or-later

// Package cli owns the node's public command tree.
package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"log"

	"github.com/rehuony/sing-box-panel/internal/buildinfo"
	"github.com/rehuony/sing-box-panel/internal/nodeapi"
	"github.com/rehuony/sing-box-panel/internal/sing-box-node/agent"
	"github.com/rehuony/sing-box-panel/internal/sing-box-node/settings"
	"github.com/spf13/cobra"
)

type Dependencies struct {
	Stdout io.Writer
	Stderr io.Writer
	Build  buildinfo.Info
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
	root.AddCommand(newVersionCommand(deps.Build), newEnrollCommand(deps, dataDir), newRunCommand(deps, dataDir))
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
		Short: "Connect to the enrolled panel and keep the session until stopped",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			logger := log.New(cmd.ErrOrStderr(), "", log.LstdFlags)
			return agent.Run(cmd.Context(), *dataDir, agent.Options{Build: deps.Build, Logf: logger.Printf})
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
