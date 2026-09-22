// SPDX-License-Identifier: GPL-3.0-or-later

package cli

import (
	"context"
	"fmt"
	"runtime"
	"slices"
	"strings"

	"github.com/rehuony/sing-box-panel/internal/application"
	"github.com/rehuony/sing-box-panel/internal/coreartifact"
	"github.com/spf13/cobra"
)

type coreSelection struct {
	architecture string
	build        string
}

func parseCoreVersion(value string) (string, error) {
	version, err := coreartifact.ParseExactVersion(strings.TrimPrefix(value, "v"))
	if err != nil || version.IsZero() {
		return "", &Error{Kind: ErrorUsage, Code: "core_version_invalid", Message: "VERSION must be an exact version such as 1.13.21 or v1.13.21; run sing-box-panel core catalog"}
	}
	return version.String(), nil
}

func validateCoreArchitecture(value string) error {
	if value != "amd64" && value != "arm64" {
		return &Error{Kind: ErrorUsage, Code: "core_arch_invalid", Message: "use --arch amd64 or --arch arm64"}
	}
	return nil
}

func configureCoreSelection(command *cobra.Command, selection *coreSelection, installed bool, state *options, open openApplicationFunc) {
	command.Args = func(cmd *cobra.Command, args []string) error {
		if err := cobra.ExactArgs(1)(cmd, args); err != nil {
			return err
		}
		if _, err := parseCoreVersion(args[0]); err != nil {
			return err
		}
		return validateCoreArchitecture(selection.architecture)
	}
	command.Flags().StringVar(&selection.architecture, "arch", runtime.GOARCH, "core architecture: amd64 or arm64 (defaults to this machine)")
	_ = command.RegisterFlagCompletionFunc("arch", cobra.FixedCompletions([]string{"amd64", "arm64"}, cobra.ShellCompDirectiveNoFileComp))
	if installed {
		command.Flags().StringVar(&selection.build, "build", "", "unique short installation number from core list")
		_ = command.RegisterFlagCompletionFunc("build", func(cmd *cobra.Command, args []string, prefix string) ([]string, cobra.ShellCompDirective) {
			if len(args) != 1 {
				return nil, cobra.ShellCompDirectiveNoFileComp
			}
			version, err := parseCoreVersion(args[0])
			if err != nil {
				return nil, cobra.ShellCompDirectiveError
			}
			instance, err := openApplication(cmd.Context(), state.settingsPath, open)
			if err != nil {
				return nil, cobra.ShellCompDirectiveError
			}
			defer instance.Close()
			items, err := allCoreArtifacts(cmd.Context(), instance, application.CoreArtifactListFilter{ExactVersion: version, Architecture: selection.architecture, Variant: "musl"})
			if err != nil {
				return nil, cobra.ShellCompDirectiveError
			}
			var values []string
			for _, build := range shortBuildIDs(items) {
				if strings.HasPrefix(build, prefix) {
					values = append(values, build)
				}
			}
			slices.Sort(values)
			return values, cobra.ShellCompDirectiveNoFileComp
		})
	}
	command.ValidArgsFunction = func(cmd *cobra.Command, args []string, prefix string) ([]string, cobra.ShellCompDirective) {
		if len(args) != 0 {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		instance, err := openApplication(cmd.Context(), state.settingsPath, open)
		if err != nil {
			return nil, cobra.ShellCompDirectiveError
		}
		defer instance.Close()
		var versions []string
		if installed {
			items, err := allCoreArtifacts(cmd.Context(), instance, application.CoreArtifactListFilter{Architecture: selection.architecture, Variant: "musl"})
			if err != nil {
				return nil, cobra.ShellCompDirectiveError
			}
			for _, item := range items {
				versions = append(versions, item.ExactVersion)
			}
		} else {
			result, err := instance.ListCatalogAssets(cmd.Context(), application.CatalogAssetFilter{Architecture: selection.architecture, Variant: "musl"})
			if err != nil {
				return nil, cobra.ShellCompDirectiveError
			}
			for _, asset := range result.Assets {
				versions = append(versions, asset.Version.String())
			}
		}
		slices.Sort(versions)
		versions = slices.Compact(versions)
		versions = slices.DeleteFunc(versions, func(version string) bool { return !strings.HasPrefix(version, strings.TrimPrefix(prefix, "v")) })
		if strings.HasPrefix(prefix, "v") {
			for index := range versions {
				versions[index] = "v" + versions[index]
			}
		}
		return versions, cobra.ShellCompDirectiveNoFileComp
	}
}

func allCoreArtifacts(ctx context.Context, instance *application.Application, filter application.CoreArtifactListFilter) ([]application.CoreArtifact, error) {
	filter.Limit = 200
	var items []application.CoreArtifact
	for {
		page, err := instance.ListCoreArtifacts(ctx, filter)
		if err != nil {
			return nil, err
		}
		items = append(items, page.Items...)
		if page.Next == nil {
			return items, nil
		}
		filter.Cursor = page.Next
	}
}

// Short numbers are display-only prefixes of existing IDs. Extend colliding
// prefixes instead of choosing an arbitrary installation or changing storage.
func shortBuildIDs(items []application.CoreArtifact) map[string]string {
	result := make(map[string]string, len(items))
	for _, item := range items {
		value := strings.TrimPrefix(item.ID, "core_")
		length := min(12, len(value))
		for _, other := range items {
			if other.ID == item.ID {
				continue
			}
			otherValue := strings.TrimPrefix(other.ID, "core_")
			for length < len(value) && strings.HasPrefix(otherValue, value[:length]) {
				length++
			}
		}
		result[item.ID] = value[:length]
	}
	return result
}

func selectCoreBuild(items []application.CoreArtifact, version string, selection coreSelection, operation string) (application.CoreArtifact, error) {
	if len(items) == 0 {
		return application.CoreArtifact{}, &Error{Kind: ErrorDomain, Code: "core_version_not_installed", Message: fmt.Sprintf("core %s (%s musl) is not installed; run sing-box-panel core list or sing-box-panel core install %s --arch %s", version, selection.architecture, version, selection.architecture)}
	}
	builds := shortBuildIDs(items)
	if selection.build != "" {
		var matches []application.CoreArtifact
		for _, item := range items {
			if strings.HasPrefix(strings.TrimPrefix(item.ID, "core_"), selection.build) && len(selection.build) >= min(12, len(strings.TrimPrefix(item.ID, "core_"))) {
				matches = append(matches, item)
			}
		}
		if len(matches) == 1 {
			return matches[0], nil
		}
	} else if len(items) == 1 {
		return items[0], nil
	}
	var message strings.Builder
	fmt.Fprintf(&message, "select a unique installation of %s with --build:\nVERSION\tARCH\tSOURCE\tBUILD\tCOMMAND\n", version)
	for _, item := range items {
		fmt.Fprintf(&message, "%s\t%s\t%s\t%s\tsing-box-panel core %s %s --arch %s --build %s\n", item.ExactVersion, item.Architecture, item.SourceKind, builds[item.ID], operation, version, selection.architecture, builds[item.ID])
	}
	return application.CoreArtifact{}, &Error{Kind: ErrorConflict, Code: "core_build_required", Message: strings.TrimSuffix(message.String(), "\n")}
}

func resolveInstalledCore(cmd *cobra.Command, instance *application.Application, rawVersion string, selection coreSelection) (application.CoreArtifact, error) {
	version, err := parseCoreVersion(rawVersion)
	if err != nil {
		return application.CoreArtifact{}, err
	}
	items, err := allCoreArtifacts(cmd.Context(), instance, application.CoreArtifactListFilter{ExactVersion: version, Architecture: selection.architecture, Variant: "musl"})
	if err != nil {
		return application.CoreArtifact{}, classifyCoreError("core_list_failed", err)
	}
	return selectCoreBuild(items, version, selection, cmd.Name())
}
