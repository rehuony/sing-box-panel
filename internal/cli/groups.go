// SPDX-License-Identifier: GPL-3.0-or-later

package cli

import (
	"github.com/rehuony/sing-box-panel/internal/store"
	"github.com/spf13/cobra"
)

func newCoreCommand(state *options, open openApplicationFunc) *cobra.Command {
	root := group("core", "Manage sing-box artifacts and runtime")
	root.AddCommand(
		newCoreCatalogCommand(state, open),
		newCoreListCommand(state, open), newCoreShowCommand(state, open),
		newCoreInstallCommand(state, open), newCoreImportCommand(state, open), newCoreRemoveCommand(state, open),
		newCoreRestrictCommand("quarantine", store.CoreArtifactQuarantined, state, open),
		newCoreRestrictCommand("revoke", store.CoreArtifactRevoked, state, open),
		newCoreCheckCommand(state, open), newCoreActivateCommand(state, open),
		newCoreRollbackCommand(state, open), newCoreStatusCommand(state, open),
		newCoreStartCommand(state, open), newCoreStopCommand(state, open), newCoreRestartCommand(state, open),
	)
	return root
}

func newConfigCommand(state *options, open openApplicationFunc) *cobra.Command {
	root := group("config", "Manage the global canonical configuration")
	root.AddCommand(
		newConfigShowCommand(state, open), newConfigReplaceCommand(state, open),
		newConfigGetCommand(state, open), newConfigSetCommand(state, open),
		newConfigUnsetCommand(state, open), newConfigExportCommand(state, open),
		newConfigImportCommand(state, open), newConfigValidateCommand(state),
		newConfigDiffCommand(state, open), newConfigApplyCommand(state, open),
	)
	root.AddCommand(newConfigCompileCommand(state, open))
	root.AddCommand(newConfigRevisionCommand(state, open))
	return root
}

func newTaskCommand(state *options, open openApplicationFunc) *cobra.Command {
	return newDurableTaskCommand(state, open)
}

func group(use, short string) *cobra.Command {
	return &cobra.Command{Use: use, Short: short, Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error { return cmd.Help() }}
}
