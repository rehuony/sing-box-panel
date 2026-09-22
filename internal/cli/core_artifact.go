// SPDX-License-Identifier: GPL-3.0-or-later

package cli

import (
	"errors"
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
	"text/tabwriter"

	"github.com/rehuony/sing-box-panel/internal/application"
	"github.com/rehuony/sing-box-panel/internal/store"
	"github.com/spf13/cobra"
)

func newCoreListCommand(state *options, open openApplicationFunc) *cobra.Command {
	var version, architecture, variant, source string
	var limit int
	command := &cobra.Command{Use: "list", Short: "List installed exact core artifacts", Args: cobra.NoArgs,
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
			result, err := instance.ListCoreArtifacts(cmd.Context(), application.CoreArtifactListFilter{
				ExactVersion: version, Architecture: architecture, Variant: variant,
				SourceKind: store.CoreArtifactSourceKind(source), Limit: limit,
			})
			if err != nil {
				return &Error{Kind: ErrorValidation, Code: "core_filter_invalid", Message: err.Error(), Cause: err}
			}
			all, err := allCoreArtifacts(cmd.Context(), instance, application.CoreArtifactListFilter{})
			if err != nil {
				return classifyCoreError("core_list_failed", err)
			}
			status, statusErr := instance.RuntimeStatus(cmd.Context())
			return writeResult(cmd.OutOrStdout(), state.format, result, coreArtifactPageText(result, shortBuildIDs(all), status, statusErr))
		}}
	command.Flags().StringVar(&version, "core-version", "", "filter by exact sing-box version")
	command.Flags().StringVar(&architecture, "arch", runtime.GOARCH, "filter by amd64 or arm64 (defaults to this machine)")
	command.Flags().StringVar(&variant, "variant", "musl", "filter by exact artifact variant")
	command.Flags().StringVar(&source, "source", "", "filter by official or user_verified")
	command.Flags().IntVar(&limit, "limit", 50, "maximum artifacts to return (1-200)")
	return command
}

func newCoreShowCommand(state *options, open openApplicationFunc) *cobra.Command {
	var selection coreSelection
	command := &cobra.Command{Use: "show VERSION", Short: "Show one installed exact core artifact", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			instance, err := openApplication(cmd.Context(), state.settingsPath, open)
			if err != nil {
				return err
			}
			defer instance.Close()
			artifact, err := resolveInstalledCore(cmd, instance, args[0], selection)
			if err != nil {
				return classifyCoreError("core_read_failed", err)
			}
			return writeResult(cmd.OutOrStdout(), state.format, artifact, coreArtifactText(artifact))
		}}
	configureCoreSelection(command, &selection, true, state, open)
	return command
}

func newCoreInstallCommand(state *options, open openApplicationFunc) *cobra.Command {
	var selection coreSelection
	command := &cobra.Command{Use: "install VERSION", Short: "Install an exact official version from the cached catalog", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			instance, err := openApplication(cmd.Context(), state.settingsPath, open)
			if err != nil {
				return err
			}
			defer instance.Close()
			version, err := parseCoreVersion(args[0])
			if err != nil {
				return err
			}
			assets, err := instance.ListCatalogAssets(cmd.Context(), application.CatalogAssetFilter{ExactVersion: version, Architecture: selection.architecture, Variant: "musl"})
			if err != nil {
				return classifyCoreError("core_catalog_failed", err)
			}
			if len(assets.Assets) == 0 {
				return &Error{Kind: ErrorDomain, Code: "core_version_not_in_catalog", Message: fmt.Sprintf("core %s (%s musl) is absent from the cached catalog; run sing-box-panel core refresh --force then sing-box-panel core catalog --arch %s", version, selection.architecture, selection.architecture)}
			}
			if len(assets.Assets) != 1 {
				return &Error{Kind: ErrorConflict, Code: "core_catalog_ambiguous", Message: "multiple official assets match this version and architecture; run sing-box-panel core refresh --force then sing-box-panel core catalog"}
			}
			result, err := instance.InstallCore(cmd.Context(), assets.Assets[0].AssetID)
			if err != nil {
				return classifyCoreError("core_install_failed", err)
			}
			return writeResult(cmd.OutOrStdout(), state.format, result, coreArtifactText(result))
		}}
	configureCoreSelection(command, &selection, false, state, open)
	return command
}

func newCoreImportCommand(state *options, open openApplicationFunc) *cobra.Command {
	var filePath, version, architecture, variant, sourceDescription string
	command := &cobra.Command{Use: "import", Short: "Import a local tar.gz", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			for flag, value := range map[string]string{"file": filePath, "version": version, "arch": architecture} {
				if strings.TrimSpace(value) == "" {
					return &Error{Kind: ErrorUsage, Code: "core_import_flag_required", Message: "--" + flag + " is required"}
				}
			}
			exactVersion, err := parseCoreVersion(version)
			if err != nil {
				return err
			}
			if err := validateCoreArchitecture(architecture); err != nil {
				return err
			}
			absolutePath, err := filepath.Abs(filepath.Clean(filePath))
			if err != nil {
				return &Error{Kind: ErrorValidation, Code: "core_import_path_invalid", Message: err.Error(), Cause: err}
			}
			instance, err := openApplication(cmd.Context(), state.settingsPath, open)
			if err != nil {
				return err
			}
			defer instance.Close()
			result, err := instance.ImportCore(cmd.Context(), application.CoreImportRequest{
				SourcePath: absolutePath, SourceDescription: sourceDescription,
				ExactVersion: exactVersion, Architecture: architecture, Variant: variant,
			})
			if err != nil {
				return &Error{Kind: ErrorValidation, Code: "core_import_invalid", Message: err.Error(), Cause: err}
			}
			return writeResult(cmd.OutOrStdout(), state.format, result, coreArtifactText(result))
		}}
	command.Flags().StringVar(&filePath, "file", "", "absolute or working-directory-relative local tar.gz path")
	command.Flags().StringVar(&version, "version", "", "expected exact sing-box version")
	command.Flags().StringVar(&architecture, "arch", runtime.GOARCH, "expected architecture: amd64 or arm64")
	command.Flags().StringVar(&variant, "variant", "musl", "artifact variant (musl)")
	command.Flags().StringVar(&sourceDescription, "source", "local archive", "non-secret source description")
	return command
}

func newCoreRemoveCommand(state *options, open openApplicationFunc) *cobra.Command {
	var selection coreSelection
	command := &cobra.Command{Use: "remove VERSION", Short: "Unregister an unused core artifact", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			instance, err := openApplication(cmd.Context(), state.settingsPath, open)
			if err != nil {
				return err
			}
			defer instance.Close()
			artifact, err := resolveInstalledCore(cmd, instance, args[0], selection)
			if err != nil {
				return err
			}
			if err := instance.RemoveCoreArtifact(cmd.Context(), artifact.ID); err != nil {
				return classifyCoreError("core_remove_failed", err)
			}
			return writeResult(cmd.OutOrStdout(), state.format, map[string]any{"artifact_id": artifact.ID, "unregistered": true}, "unregistered core "+artifact.ExactVersion)
		}}
	configureCoreSelection(command, &selection, true, state, open)
	return command
}

func classifyCoreError(code string, err error) error {
	var classified *Error
	if errors.As(err, &classified) {
		return classified
	}
	switch {
	case application.IsCatalogNotInitialized(err):
		return &Error{Kind: ErrorUnavailable, Code: "catalog_not_initialized", Message: "official catalog is not cached; run sing-box-panel core refresh then sing-box-panel core catalog", Cause: err}
	case application.IsCoreArtifactNotFound(err):
		return &Error{Kind: ErrorDomain, Code: "core_artifact_not_found", Message: err.Error(), Cause: err}
	case application.IsCoreArtifactInUse(err):
		return &Error{Kind: ErrorConflict, Code: "core_artifact_in_use", Message: err.Error(), Cause: err}
	default:
		return &Error{Kind: ErrorDomain, Code: code, Message: err.Error(), Cause: err}
	}
}

func coreArtifactPageText(result application.CoreArtifactPage, builds map[string]string, status application.RuntimeStatus, statusErr error) string {
	if len(result.Items) == 0 {
		return "no installed core artifacts; run sing-box-panel core catalog"
	}
	var output strings.Builder
	table := tabwriter.NewWriter(&output, 0, 4, 2, ' ', 0)
	fmt.Fprintln(table, "VERSION\tARCH\tSOURCE\tSTATUS\tBUILD")
	for _, artifact := range result.Items {
		state := "installed"
		if statusErr != nil {
			state = "unknown"
		} else if status.EnabledCore != nil && status.EnabledCore.CoreArtifactID == artifact.ID {
			state = "enabled (" + status.ObservationState + ")"
		}
		fmt.Fprintf(table, "%s\t%s\t%s\t%s\t%s\n", artifact.ExactVersion, artifact.Architecture, artifact.SourceKind, state, builds[artifact.ID])
	}
	_ = table.Flush()
	if result.Next != nil {
		fmt.Fprintln(&output, "More installations exist. Narrow with --core-version VERSION and --source, or increase --limit (maximum 200).")
	}
	return strings.TrimSuffix(output.String(), "\n")
}

func coreArtifactText(artifact application.CoreArtifact) string {
	return fmt.Sprintf("Version: %s\nArchitecture: %s/%s (%s)\nSource: %s\nSource description: %s\nInstallation ID: %s\nBinary: %s\nReported version: %s\nInstalled: %s\nRecorded archive SHA-256: %s\nRecorded binary SHA-256: %s", artifact.ExactVersion, artifact.OperatingSystem, artifact.Architecture, artifact.Variant, artifact.SourceKind, emptyAsDash(artifact.UserSource), artifact.ID, artifact.BinaryPath, artifact.ReportedVersion, artifact.CreatedAt.Format("2006-01-02T15:04:05Z07:00"), artifact.ArchiveSHA256, artifact.BinarySHA256)
}
