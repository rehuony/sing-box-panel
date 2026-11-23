// SPDX-License-Identifier: GPL-3.0-or-later

package cli

import "github.com/spf13/cobra"

func newCompletionCommand(root *cobra.Command) *cobra.Command {
	completion := &cobra.Command{Use: "completion", Short: "Generate shell completion scripts", Args: cobra.NoArgs}
	bash := &cobra.Command{
		Use:   "bash",
		Short: "Generate Bash completion script",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return root.GenBashCompletionV2(cmd.OutOrStdout(), true)
		},
	}
	zsh := &cobra.Command{
		Use:   "zsh",
		Short: "Generate Zsh completion script",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return root.GenZshCompletion(cmd.OutOrStdout())
		},
	}
	fish := &cobra.Command{
		Use:   "fish",
		Short: "Generate fish completion script",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return root.GenFishCompletion(cmd.OutOrStdout(), true)
		},
	}
	completion.AddCommand(bash, zsh, fish)
	return completion
}
