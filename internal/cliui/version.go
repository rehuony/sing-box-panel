// SPDX-License-Identifier: GPL-3.0-or-later

package cliui

import (
	"github.com/rehuony/sing-box-panel/internal/buildinfo"
	"github.com/spf13/cobra"
)

// VersionResult is the build information both binaries report: one release
// version shared by the panel and the node, and the panel-node protocol.
type VersionResult struct {
	buildinfo.Info
	Protocol string `json:"protocol"`
}

// Version prints name and its version; structured output carries the
// complete build information.
func Version(name string, info buildinfo.Info, protocol string, format *Format) *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print " + name + " build information",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return Write(cmd.OutOrStdout(), *format, VersionResult{Info: info, Protocol: protocol}, name+" "+info.DisplayVersion())
		},
	}
}
