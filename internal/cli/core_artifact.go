// SPDX-License-Identifier: GPL-3.0-or-later

package cli

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/rehuony/sing-box-panel/internal/application"
	"github.com/rehuony/sing-box-panel/internal/store"
	"github.com/spf13/cobra"
)

func newCoreListCommand(state *options, open openApplicationFunc) *cobra.Command {
	var version, architecture, variant, source, verification string
	var limit int
	command := &cobra.Command{Use: "list", Short: "List installed exact core artifacts", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			instance, err := openApplication(cmd.Context(), state.settingsPath, open)
			if err != nil {
				return err
			}
			defer instance.Close()
			result, err := instance.ListCoreArtifacts(cmd.Context(), application.CoreArtifactListFilter{
				ExactVersion: version, Architecture: architecture, Variant: variant,
				SourceKind: store.CoreArtifactSourceKind(source), VerificationState: store.CoreArtifactVerificationState(verification), Limit: limit,
			})
			if err != nil {
				return &Error{Kind: ErrorValidation, Code: "core_filter_invalid", Message: err.Error(), Cause: err}
			}
			return writeResult(cmd.OutOrStdout(), state.format, result, coreArtifactPageText(result))
		}}
	command.Flags().StringVar(&version, "core-version", "", "filter by exact sing-box version")
	command.Flags().StringVar(&architecture, "arch", "", "filter by amd64 or arm64")
	command.Flags().StringVar(&variant, "variant", "", "filter by exact artifact variant")
	command.Flags().StringVar(&source, "source", "", "filter by official or user_verified")
	command.Flags().StringVar(&verification, "verification", "", "filter by verified, revoked, or quarantined")
	command.Flags().IntVar(&limit, "limit", 50, "maximum artifacts to return (1-200)")
	return command
}

func newCoreShowCommand(state *options, open openApplicationFunc) *cobra.Command {
	return &cobra.Command{Use: "show ARTIFACT", Short: "Show one installed exact core artifact", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			instance, err := openApplication(cmd.Context(), state.settingsPath, open)
			if err != nil {
				return err
			}
			defer instance.Close()
			artifact, err := instance.CoreArtifact(cmd.Context(), args[0])
			if err != nil {
				return classifyCoreError("core_read_failed", err)
			}
			return writeResult(cmd.OutOrStdout(), state.format, artifact, coreArtifactText(artifact))
		}}
}

func newCoreInstallCommand(state *options, open openApplicationFunc) *cobra.Command {
	var detach bool
	command := &cobra.Command{Use: "install ASSET_ID", Short: "Install one cached official asset as a durable verified task", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			assetID, err := strconv.ParseInt(args[0], 10, 64)
			if err != nil || assetID <= 0 {
				return &Error{Kind: ErrorUsage, Code: "asset_id_invalid", Message: "ASSET_ID must be a positive GitHub asset ID"}
			}
			instance, err := openApplication(cmd.Context(), state.settingsPath, open)
			if err != nil {
				return err
			}
			defer instance.Close()
			task, err := instance.QueueCoreInstall(cmd.Context(), assetID)
			if err != nil {
				return classifyCoreError("core_install_queue_failed", err)
			}
			return renderQueuedTask(cmd, state, instance, task, detach)
		}}
	command.Flags().BoolVar(&detach, "detach", false, "return the durable task immediately")
	return command
}

func newCoreImportCommand(state *options, open openApplicationFunc) *cobra.Command {
	var filePath, digest, version, architecture, variant, sourceDescription string
	var detach bool
	command := &cobra.Command{Use: "import", Short: "Import an administrator-verified local tar.gz as a durable task", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			for flag, value := range map[string]string{"file": filePath, "sha256": digest, "version": version, "arch": architecture} {
				if strings.TrimSpace(value) == "" {
					return &Error{Kind: ErrorUsage, Code: "core_import_flag_required", Message: "--" + flag + " is required"}
				}
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
			task, err := instance.QueueCoreImport(cmd.Context(), application.CoreImportRequest{
				SourcePath: absolutePath, SourceDescription: sourceDescription, SHA256: digest,
				ExactVersion: version, Architecture: architecture, Variant: variant,
			})
			if err != nil {
				return &Error{Kind: ErrorValidation, Code: "core_import_invalid", Message: err.Error(), Cause: err}
			}
			return renderQueuedTask(cmd, state, instance, task, detach)
		}}
	command.Flags().StringVar(&filePath, "file", "", "absolute or working-directory-relative local tar.gz path")
	command.Flags().StringVar(&digest, "sha256", "", "administrator-verified archive SHA-256")
	command.Flags().StringVar(&version, "version", "", "expected exact sing-box version")
	command.Flags().StringVar(&architecture, "arch", "", "expected architecture: amd64 or arm64")
	command.Flags().StringVar(&variant, "variant", "plain", "artifact variant")
	command.Flags().StringVar(&sourceDescription, "source", "administrator verified local archive", "non-secret source description")
	command.Flags().BoolVar(&detach, "detach", false, "return the durable task immediately")
	return command
}

func newCoreRemoveCommand(state *options, open openApplicationFunc) *cobra.Command {
	return &cobra.Command{Use: "remove ARTIFACT", Short: "Unregister an unused core artifact", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			instance, err := openApplication(cmd.Context(), state.settingsPath, open)
			if err != nil {
				return err
			}
			defer instance.Close()
			if err := instance.RemoveCoreArtifact(cmd.Context(), args[0]); err != nil {
				return classifyCoreError("core_remove_failed", err)
			}
			return writeResult(cmd.OutOrStdout(), state.format, map[string]any{"artifact_id": args[0], "unregistered": true}, "unregistered core artifact "+args[0])
		}}
}

func newCoreRestrictCommand(name string, verification store.CoreArtifactVerificationState, state *options, open openApplicationFunc) *cobra.Command {
	return &cobra.Command{Use: name + " ARTIFACT", Short: "Prevent new work from using immutable core bytes", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			instance, err := openApplication(cmd.Context(), state.settingsPath, open)
			if err != nil {
				return err
			}
			defer instance.Close()
			artifact, err := instance.RestrictCoreArtifactVerification(cmd.Context(), args[0], verification)
			if err != nil {
				return classifyCoreError("core_verification_restriction_failed", err)
			}
			return writeResult(cmd.OutOrStdout(), state.format, artifact, "core artifact "+artifact.ID+" is "+string(artifact.VerificationState))
		}}
}

func renderQueuedTask(cmd *cobra.Command, state *options, instance *application.Application, task application.Task, detach bool) error {
	if detach {
		return writeResult(cmd.OutOrStdout(), state.format, task, "queued task "+task.ID)
	}
	completed, err := waitForTaskWithCancellationRequest(cmd.Context(), instance, task.ID, 250*time.Millisecond, "task_wait_failed")
	if err != nil {
		return err
	}
	if err := writeResult(cmd.OutOrStdout(), state.format, completed, taskText(completed)); err != nil {
		return err
	}
	return terminalTaskError(completed)
}

func classifyCoreError(code string, err error) error {
	switch {
	case application.IsCatalogNotInitialized(err):
		return &Error{Kind: ErrorUnavailable, Code: "catalog_not_initialized", Message: "official catalog is not cached; run core catalog refresh", Cause: err}
	case application.IsCoreArtifactNotFound(err):
		return &Error{Kind: ErrorDomain, Code: "core_artifact_not_found", Message: err.Error(), Cause: err}
	case application.IsCoreArtifactInUse(err):
		return &Error{Kind: ErrorConflict, Code: "core_artifact_in_use", Message: err.Error(), Cause: err}
	default:
		return &Error{Kind: ErrorDomain, Code: code, Message: err.Error(), Cause: err}
	}
}

func coreArtifactPageText(result application.CoreArtifactPage) string {
	if len(result.Items) == 0 {
		return "no installed core artifacts"
	}
	var output strings.Builder
	for _, artifact := range result.Items {
		fmt.Fprintf(&output, "%s\t%s\t%s\t%s\t%s\t%s\n", artifact.ID, artifact.ExactVersion, artifact.Architecture, artifact.Variant, artifact.SourceKind, artifact.VerificationState)
	}
	return strings.TrimSuffix(output.String(), "\n")
}

func coreArtifactText(artifact application.CoreArtifact) string {
	return fmt.Sprintf("%s\t%s\t%s\t%s\t%s", artifact.ID, artifact.ExactVersion, artifact.Architecture, artifact.Variant, artifact.VerificationState)
}
