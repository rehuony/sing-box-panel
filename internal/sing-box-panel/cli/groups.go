// SPDX-License-Identifier: GPL-3.0-or-later

package cli

import (
	"github.com/spf13/cobra"
)

func group(use, short string) *cobra.Command {
	return &cobra.Command{Use: use, Short: short, Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error { return cmd.Help() }}
}
