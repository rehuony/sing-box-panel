// SPDX-License-Identifier: GPL-3.0-or-later

package cli

import (
	"github.com/rehuony/sing-box-panel/internal/store"
	"github.com/spf13/cobra"
)

func newCoreCommand(state *options, open openApplicationFunc) *cobra.Command {
	root := group("core", "Manage sing-box artifacts and runtime")
	root.AddCommand(
		newCoreCatalogListCommand(state, open), newCoreCatalogRefreshCommand(state, open),
		newCoreListCommand(state, open), newCoreShowCommand(state, open),
		newCoreInstallCommand(state, open), newCoreImportCommand(state, open), newCoreRemoveCommand(state, open),
		newCoreRestrictCommand("quarantine", store.CoreArtifactQuarantined, state, open),
		newCoreRestrictCommand("revoke", store.CoreArtifactRevoked, state, open),
		newCoreEnableCommand(state, open), newCoreStatusCommand(state, open),
		newCoreStartCommand(state, open), newCoreStopCommand(state, open), newCoreRestartCommand(state, open),
		newCoreRollbackCommand(state, open),
	)
	return root
}

func newTaskCommand(state *options, open openApplicationFunc) *cobra.Command {
	return newDurableTaskCommand(state, open)
}

func group(use, short string) *cobra.Command {
	return &cobra.Command{Use: use, Short: short, Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error { return cmd.Help() }}
}
