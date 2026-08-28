// SPDX-License-Identifier: GPL-3.0-or-later

package cli

import (
	"fmt"
	"strings"

	"github.com/rehuony/sing-box-panel/internal/application"
	"github.com/spf13/cobra"
)

func newCoreCatalogCommand(state *options, open openApplicationFunc) *cobra.Command {
	root := group("catalog", "Inspect the official release catalog")
	root.AddCommand(newCoreCatalogListCommand(state, open), newCoreCatalogRefreshCommand(state, open))
	return root
}

func newCoreCatalogListCommand(state *options, open openApplicationFunc) *cobra.Command {
	var version, architecture, variant string
	var installable bool
	command := &cobra.Command{
		Use:   "list",
		Short: "List cached official stable release assets",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			instance, err := openApplication(cmd.Context(), state.settingsPath, open)
			if err != nil {
				return err
			}
			defer instance.Close()
			result, err := instance.ListCatalogAssets(cmd.Context(), application.CatalogAssetFilter{
				ExactVersion: version, Architecture: architecture, Variant: variant, Installable: installable,
			})
			if err != nil {
				if application.IsCatalogNotInitialized(err) {
					return &Error{Kind: ErrorUnavailable, Code: "catalog_not_initialized", Message: "official catalog is not cached; run core catalog refresh", Cause: err}
				}
				return &Error{Kind: ErrorValidation, Code: "catalog_filter_invalid", Message: err.Error(), Cause: err}
			}
			return writeResult(cmd.OutOrStdout(), state.format, result, catalogAssetListText(result))
		},
	}
	command.Flags().StringVar(&version, "core-version", "", "filter by exact sing-box version")
	command.Flags().StringVar(&architecture, "arch", "", "filter by amd64 or arm64")
	command.Flags().StringVar(&variant, "variant", "", "filter by exact artifact variant")
	command.Flags().BoolVar(&installable, "installable", false, "show only assets with trusted digest evidence")
	return command
}

func newCoreCatalogRefreshCommand(state *options, open openApplicationFunc) *cobra.Command {
	var detach, force bool
	command := &cobra.Command{
		Use:   "refresh",
		Short: "Refresh the official stable release catalog as a durable task",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			instance, err := openApplication(cmd.Context(), state.settingsPath, open)
			if err != nil {
				return err
			}
			defer instance.Close()
			task, err := instance.QueueCatalogRefresh(cmd.Context(), application.CatalogRefreshOptions{Force: force})
			if err != nil {
				return &Error{Kind: ErrorDomain, Code: "catalog_refresh_queue_failed", Message: err.Error(), Cause: err}
			}
			return renderQueuedTask(cmd, state, instance, task, detach)
		},
	}
	command.Flags().BoolVar(&detach, "detach", false, "return the durable task immediately")
	command.Flags().BoolVar(&force, "force", false, "bypass the configured catalog TTL")
	return command
}

func catalogAssetListText(result application.CatalogAssetList) string {
	if len(result.Assets) == 0 {
		return "no matching official assets"
	}
	var output strings.Builder
	for _, asset := range result.Assets {
		installable := "no-digest"
		if _, err := asset.TrustedDigest(); err == nil {
			installable = "installable"
		}
		fmt.Fprintf(&output, "%d\t%s\t%s\t%s\t%s\t%s\n", asset.AssetID, asset.Version, asset.Architecture, asset.Variant, installable, asset.Name)
	}
	return strings.TrimSuffix(output.String(), "\n")
}
