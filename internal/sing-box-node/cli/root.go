// SPDX-License-Identifier: GPL-3.0-or-later

// Package cli owns the node's public command tree.
package cli

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/rehuony/sing-box-panel/internal/buildinfo"
	"github.com/rehuony/sing-box-panel/internal/nodeapi"
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
	root.AddCommand(newVersionCommand(deps.Build))
	return root
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
