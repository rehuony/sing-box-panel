// SPDX-License-Identifier: GPL-3.0-or-later

package cli

import (
	"fmt"
	"runtime"
	"strings"
	"text/tabwriter"

	"github.com/rehuony/sing-box-panel/internal/application"
	"github.com/spf13/cobra"
)

func newCoreCatalogListCommand(state *options, open openApplicationFunc) *cobra.Command {
	var version, architecture, variant string
	command := &cobra.Command{
		Use:   "catalog",
		Short: "List cached official stable release assets",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			instance, err := openApplication(cmd.Context(), state.settingsPath, open)
			if err != nil {
				return err
			}
			defer instance.Close()
			if version != "" {
				version, err = parseCoreVersion(version)
				if err != nil {
					return err
				}
			}
			if err := validateCoreArchitecture(architecture); err != nil {
				return err
			}
			result, err := instance.ListCatalogAssets(cmd.Context(), application.CatalogAssetFilter{
				ExactVersion: version, Architecture: architecture, Variant: variant,
			})
			if err != nil {
				if application.IsCatalogNotInitialized(err) {
					return &Error{Kind: ErrorUnavailable, Code: "catalog_not_initialized", Message: "official catalog is not cached; run sing-box-panel core refresh then sing-box-panel core catalog", Cause: err}
				}
				return &Error{Kind: ErrorValidation, Code: "catalog_filter_invalid", Message: err.Error(), Cause: err}
			}
			return writeResult(cmd.OutOrStdout(), state.format, result, catalogAssetListText(result))
		},
	}
	command.Flags().StringVar(&version, "core-version", "", "filter by exact sing-box version")
	command.Flags().StringVar(&architecture, "arch", runtime.GOARCH, "filter by amd64 or arm64 (defaults to this machine)")
	command.Flags().StringVar(&variant, "variant", "musl", "filter by exact artifact variant")
	return command
}

func newCoreCatalogRefreshCommand(state *options, open openApplicationFunc) *cobra.Command {
	var force bool
	command := &cobra.Command{
		Use:   "refresh",
		Short: "Refresh the official stable release catalog",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			instance, err := openApplication(cmd.Context(), state.settingsPath, open)
			if err != nil {
				return err
			}
			defer instance.Close()
			result, err := instance.RefreshCatalog(cmd.Context(), application.CatalogRefreshOptions{Force: force})
			if err != nil {
				return &Error{Kind: ErrorDomain, Code: "catalog_refresh_failed", Message: err.Error(), Cause: err}
			}
			return writeResult(cmd.OutOrStdout(), state.format, result, fmt.Sprintf("Refreshed official catalog: %d releases, %d assets", len(result.Catalog.Releases), len(result.Catalog.Assets())))
		},
	}
	command.Flags().BoolVar(&force, "force", false, "bypass the configured catalog refresh interval")
	return command
}

func catalogAssetListText(result application.CatalogAssetList) string {
	if len(result.Assets) == 0 {
		return "no matching official assets"
	}
	var output strings.Builder
	table := tabwriter.NewWriter(&output, 0, 4, 2, ' ', 0)
	fmt.Fprintln(table, "VERSION\tARCH\tSOURCE\tSTATUS\tPACKAGE")
	for _, asset := range result.Assets {
		fmt.Fprintf(table, "%s\t%s\tofficial\tavailable\t%s\n", asset.Version, asset.Architecture, asset.Name)
	}
	_ = table.Flush()
	return strings.TrimSuffix(output.String(), "\n")
}
