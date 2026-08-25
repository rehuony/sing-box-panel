// SPDX-License-Identifier: GPL-3.0-or-later

package cli

import "github.com/spf13/cobra"

func newCompletionCommand(root *cobra.Command) *cobra.Command {
	completion := &cobra.Command{Use: "completion", Short: "Generate shell completion", Args: cobra.NoArgs}
	completion.AddCommand(
		&cobra.Command{Use: "bash", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
			return root.GenBashCompletion(cmd.OutOrStdout())
		}},
		&cobra.Command{Use: "zsh", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
			return root.GenZshCompletion(cmd.OutOrStdout())
		}},
		&cobra.Command{Use: "fish", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
			return root.GenFishCompletion(cmd.OutOrStdout(), true)
		}},
	)
	return completion
}
