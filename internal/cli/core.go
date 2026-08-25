// SPDX-License-Identifier: GPL-3.0-or-later

package cli

import (
	"errors"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/rehuony/sing-box-panel/internal/application"
	"github.com/rehuony/sing-box-panel/internal/capability"
	"github.com/rehuony/sing-box-panel/internal/runtimeidentity"
	"github.com/rehuony/sing-box-panel/internal/store"
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
	var detach bool
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
			task, err := instance.QueueCatalogRefresh(cmd.Context())
			if err != nil {
				return &Error{Kind: ErrorDomain, Code: "catalog_refresh_queue_failed", Message: err.Error(), Cause: err}
			}
			return renderQueuedTask(cmd, state, instance, task, detach)
		},
	}
	command.Flags().BoolVar(&detach, "detach", false, "return the durable task immediately")
	return command
}

func newCoreListCommand(state *options, open openApplicationFunc) *cobra.Command {
	var version, architecture, variant, source, verification string
	var limit int
	command := &cobra.Command{
		Use:   "list",
		Short: "List installed exact core artifacts",
		Args:  cobra.NoArgs,
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
		},
	}
	command.Flags().StringVar(&version, "core-version", "", "filter by exact sing-box version")
	command.Flags().StringVar(&architecture, "arch", "", "filter by amd64 or arm64")
	command.Flags().StringVar(&variant, "variant", "", "filter by exact artifact variant")
	command.Flags().StringVar(&source, "source", "", "filter by official or user_verified")
	command.Flags().StringVar(&verification, "verification", "", "filter by verified, revoked, or quarantined")
	command.Flags().IntVar(&limit, "limit", 50, "maximum artifacts to return (1-200)")
	return command
}

func newCoreShowCommand(state *options, open openApplicationFunc) *cobra.Command {
	return &cobra.Command{
		Use:   "show ARTIFACT",
		Short: "Show one installed exact core artifact",
		Args:  cobra.ExactArgs(1),
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
		},
	}
}

func newCoreInstallCommand(state *options, open openApplicationFunc) *cobra.Command {
	var detach bool
	command := &cobra.Command{
		Use:   "install ASSET_ID",
		Short: "Install one cached official asset as a durable verified task",
		Args:  cobra.ExactArgs(1),
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
		},
	}
	command.Flags().BoolVar(&detach, "detach", false, "return the durable task immediately")
	return command
}

func newCoreImportCommand(state *options, open openApplicationFunc) *cobra.Command {
	var filePath, digest, version, architecture, variant, sourceDescription string
	var detach bool
	command := &cobra.Command{
		Use:   "import",
		Short: "Import an administrator-verified local tar.gz as a durable task",
		Args:  cobra.NoArgs,
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
		},
	}
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
	return &cobra.Command{
		Use:   "remove ARTIFACT",
		Short: "Unregister an unused core artifact while retaining shared content for safe garbage collection",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			instance, err := openApplication(cmd.Context(), state.settingsPath, open)
			if err != nil {
				return err
			}
			defer instance.Close()
			if err := instance.RemoveCoreArtifact(cmd.Context(), args[0]); err != nil {
				return classifyCoreError("core_remove_failed", err)
			}
			result := map[string]any{"artifact_id": args[0], "unregistered": true, "content_retained": true}
			return writeResult(cmd.OutOrStdout(), state.format, result, "unregistered core artifact "+args[0]+"; shared content retained")
		},
	}
}

func newCoreRestrictCommand(
	name string,
	verificationState store.CoreArtifactVerificationState,
	state *options,
	open openApplicationFunc,
) *cobra.Command {
	return &cobra.Command{
		Use:   name + " ARTIFACT",
		Short: "Prevent new checks, activations, and starts from using immutable core bytes",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			instance, err := openApplication(cmd.Context(), state.settingsPath, open)
			if err != nil {
				return err
			}
			defer instance.Close()
			artifact, err := instance.RestrictCoreArtifactVerification(
				cmd.Context(), args[0], verificationState,
			)
			if err != nil {
				return classifyCoreError("core_verification_restriction_failed", err)
			}
			return writeResult(
				cmd.OutOrStdout(), state.format, artifact,
				"core artifact "+artifact.ID+" is "+string(artifact.VerificationState),
			)
		},
	}
}

func newCoreCapabilityCommand(state *options, open openApplicationFunc) *cobra.Command {
	root := group("capability", "Inspect and pin capability manifests")
	root.AddCommand(
		newCoreCapabilityStatusCommand(state, open),
		newCoreCapabilityPackCommand(state),
		newCoreCapabilityRefreshCommand(state, open),
		newCoreCapabilityInspectCommand(state, open),
		newCoreCapabilityUpgradeCommand(state, open),
		newCoreCapabilityQuarantineCommand(state, open),
	)
	return root
}

func newCoreCapabilityQuarantineCommand(state *options, open openApplicationFunc) *cobra.Command {
	var manifestSHA256, reasonCode string
	command := &cobra.Command{
		Use:   "quarantine",
		Short: "Permanently quarantine one immutable capability manifest",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if !cmd.Flags().Changed("sha256") || !cmd.Flags().Changed("reason") {
				return &Error{
					Kind:    ErrorUsage,
					Code:    "capability_quarantine_flag_required",
					Message: "--sha256 and --reason are required",
				}
			}
			instance, err := openApplication(cmd.Context(), state.settingsPath, open)
			if err != nil {
				return err
			}
			defer instance.Close()
			result, err := instance.QuarantineCapabilityManifest(cmd.Context(), application.CapabilityQuarantineRequest{
				ManifestSHA256: manifestSHA256,
				ReasonCode:     reasonCode,
			})
			if err != nil {
				return classifyCapabilityError("capability_quarantine_failed", err)
			}
			return writeResult(
				cmd.OutOrStdout(), state.format, result,
				"capability manifest "+result.ManifestSHA256+" is permanently quarantined: "+result.ReasonCode,
			)
		},
	}
	command.Flags().StringVar(&manifestSHA256, "sha256", "", "required canonical manifest SHA-256")
	command.Flags().StringVar(&reasonCode, "reason", "", "required stable lowercase reason code")
	return command
}

func newCoreCapabilityStatusCommand(state *options, open openApplicationFunc) *cobra.Command {
	var version string
	command := &cobra.Command{
		Use:   "status",
		Short: "Resolve exact-version capability support",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			instance, err := openApplication(cmd.Context(), state.settingsPath, open)
			if err != nil {
				return err
			}
			defer instance.Close()
			result, err := instance.CoreCapabilityStatus(cmd.Context(), version)
			if err != nil {
				if classified := classifyOmittedCoreVersionError(err, version); classified != nil {
					return classified
				}
				return &Error{Kind: ErrorUnavailable, Code: "core_version_resolution_failed", Message: err.Error(), Cause: err}
			}
			text := fmt.Sprintf("%s\t%s\t%s", result.Resolution.ExactVersion, result.Resolution.Source, result.SupportLevel)
			if result.Quarantined {
				text += "\tquarantined:" + result.ReasonCode
			}
			return writeResult(cmd.OutOrStdout(), state.format, result, text)
		},
	}
	command.Flags().StringVar(&version, "core-version", "", "exact sing-box version; omit to use the actual running version")
	return command
}

func newCoreCapabilityRefreshCommand(state *options, open openApplicationFunc) *cobra.Command {
	var filePath string
	command := &cobra.Command{
		Use:   "refresh",
		Short: "Validate and atomically store a local capability generation as candidates",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if strings.TrimSpace(filePath) == "" {
				return &Error{Kind: ErrorUsage, Code: "capability_generation_file_required", Message: "--file is required; use - for stdin"}
			}
			source, err := readInputFile(
				cmd.InOrStdin(),
				filePath,
				int64(capability.MaximumGenerationBytes),
				"capability generation",
			)
			if err != nil {
				return &Error{Kind: ErrorValidation, Code: "capability_generation_input_failed", Message: err.Error(), Cause: err}
			}
			instance, err := openApplication(cmd.Context(), state.settingsPath, open)
			if err != nil {
				return err
			}
			defer instance.Close()
			result, err := instance.RefreshCapabilityGeneration(cmd.Context(), source)
			if err != nil {
				return classifyCapabilityError("capability_refresh_failed", err)
			}
			verb := "stored"
			if !result.Created {
				verb = "retained existing"
			}
			text := fmt.Sprintf(
				"%s candidate generation %s@%s (%d manifests); no pins changed",
				verb,
				result.Generation.Repository,
				result.Generation.CommitSHA,
				result.Generation.ManifestCount,
			)
			return writeResult(cmd.OutOrStdout(), state.format, result, text)
		},
	}
	command.Flags().StringVar(&filePath, "file", "", "complete local generation JSON file, or - for stdin")
	return command
}

func newCoreCapabilityInspectCommand(state *options, open openApplicationFunc) *cobra.Command {
	var request application.CapabilityUpgradeRequest
	command := &cobra.Command{
		Use:   "inspect",
		Short: "Inspect one exact immutable capability candidate",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			instance, err := openApplication(cmd.Context(), state.settingsPath, open)
			if err != nil {
				return err
			}
			defer instance.Close()
			var resolution application.CoreVersionResolution
			request, resolution, err = resolveCapabilityReference(cmd, instance, request)
			if err != nil {
				return err
			}
			result, err := instance.InspectCapabilityCandidate(cmd.Context(), request)
			if err != nil {
				return classifyCapabilityError("capability_inspect_failed", err)
			}
			output := struct {
				Resolution application.CoreVersionResolution `json:"resolution"`
				application.CapabilityManifestCandidate
			}{Resolution: resolution, CapabilityManifestCandidate: result}
			text := fmt.Sprintf(
				"core %s from %s\n%s\t%s\t%s\t%s",
				resolution.ExactVersion,
				resolution.Source,
				result.ExactCoreVersion,
				result.CommitSHA,
				result.ManifestSHA256,
				result.SupportLevel,
			)
			if result.Quarantined {
				text += "\tquarantined:" + result.ReasonCode
			}
			return writeResult(cmd.OutOrStdout(), state.format, output, text)
		},
	}
	addCapabilityReferenceFlags(command, &request)
	return command
}

func newCoreCapabilityUpgradeCommand(state *options, open openApplicationFunc) *cobra.Command {
	var request application.CapabilityUpgradeRequest
	var accept bool
	command := &cobra.Command{
		Use:   "upgrade",
		Short: "Preview, then explicitly pin one exact immutable capability candidate",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			instance, err := openApplication(cmd.Context(), state.settingsPath, open)
			if err != nil {
				return err
			}
			defer instance.Close()
			var resolution application.CoreVersionResolution
			request, resolution, err = resolveCapabilityReference(cmd, instance, request)
			if err != nil {
				return err
			}
			if !accept {
				preview, err := instance.PreviewCapabilityUpgrade(cmd.Context(), request)
				if err != nil {
					return classifyCapabilityError("capability_upgrade_preview_failed", err)
				}
				output := struct {
					Resolution application.CoreVersionResolution `json:"resolution"`
					application.CapabilityUpgradePreview
				}{Resolution: resolution, CapabilityUpgradePreview: preview}
				text := fmt.Sprintf("core %s from %s\n", resolution.ExactVersion, resolution.Source) +
					capabilityUpgradePreviewText(preview) + "; preview only, rerun with --accept to move the pin"
				return writeResult(cmd.OutOrStdout(), state.format, output, text)
			}
			result, err := instance.UpgradeCapability(cmd.Context(), request)
			if err != nil {
				return classifyCapabilityError("capability_upgrade_failed", err)
			}
			output := struct {
				Resolution application.CoreVersionResolution `json:"resolution"`
				application.CapabilityUpgrade
			}{Resolution: resolution, CapabilityUpgrade: result}
			text := fmt.Sprintf("core %s from %s\n", resolution.ExactVersion, resolution.Source) +
				capabilityUpgradePreviewText(result.Preview) + "; pinned " + result.Pin.ManifestSHA256
			return writeResult(cmd.OutOrStdout(), state.format, output, text)
		},
	}
	addCapabilityReferenceFlags(command, &request)
	command.Flags().BoolVar(&accept, "accept", false, "accept the displayed preview and atomically move the exact-version pin")
	return command
}

func addCapabilityReferenceFlags(command *cobra.Command, request *application.CapabilityUpgradeRequest) {
	command.Flags().StringVar(&request.ExactCoreVersion, "core-version", "", "exact sing-box version; omit to use the actual running version")
	command.Flags().StringVar(&request.CommitSHA, "commit", "", "required immutable capability repository commit SHA")
	command.Flags().StringVar(&request.ManifestSHA256, "sha256", "", "required canonical manifest SHA-256")
}

func resolveCapabilityReference(
	command *cobra.Command,
	instance *application.Application,
	request application.CapabilityUpgradeRequest,
) (application.CapabilityUpgradeRequest, application.CoreVersionResolution, error) {
	for _, flag := range []string{"commit", "sha256"} {
		if !command.Flags().Changed(flag) {
			return application.CapabilityUpgradeRequest{}, application.CoreVersionResolution{}, &Error{
				Kind:    ErrorUsage,
				Code:    "capability_reference_required",
				Message: "--commit and --sha256 are required; omit --core-version only to use the actual running version",
			}
		}
	}
	if strings.TrimSpace(request.CommitSHA) == "" || strings.TrimSpace(request.ManifestSHA256) == "" ||
		(command.Flags().Changed("core-version") && strings.TrimSpace(request.ExactCoreVersion) == "") {
		return application.CapabilityUpgradeRequest{}, application.CoreVersionResolution{}, &Error{Kind: ErrorUsage, Code: "capability_reference_empty", Message: "capability reference flags must not be empty"}
	}
	resolution, err := instance.ResolveCoreVersion(command.Context(), request.ExactCoreVersion)
	if err != nil {
		if classified := classifyOmittedCoreVersionError(err, request.ExactCoreVersion); classified != nil {
			return application.CapabilityUpgradeRequest{}, application.CoreVersionResolution{}, classified
		}
		return application.CapabilityUpgradeRequest{}, application.CoreVersionResolution{}, &Error{Kind: ErrorValidation, Code: "core_version_resolution_failed", Message: err.Error(), Cause: err}
	}
	request.ExactCoreVersion = resolution.ExactVersion
	return request, resolution, nil
}

func classifyOmittedCoreVersionError(err error, explicitVersion string) error {
	if strings.TrimSpace(explicitVersion) != "" {
		return nil
	}
	switch {
	case application.IsNoRunningCore(err):
		return &Error{
			Kind:    ErrorUnavailable,
			Code:    "no_running_core",
			Message: "--core-version was omitted and no sing-box core is currently running",
			Cause:   err,
		}
	case errors.Is(err, runtimeidentity.ErrStaleObservation),
		errors.Is(err, runtimeidentity.ErrInspectionUnavailable):
		return &Error{
			Kind:    ErrorUnavailable,
			Code:    "running_core_unavailable",
			Message: "--core-version was omitted and the running sing-box core could not be verified",
			Cause:   err,
		}
	default:
		return nil
	}
}

func capabilityUpgradePreviewText(preview application.CapabilityUpgradePreview) string {
	current := "unconfigured"
	if preview.Current != nil {
		current = preview.Current.CommitSHA + "/" + preview.Current.ManifestSHA256
	}
	text := fmt.Sprintf(
		"%s: %s -> %s/%s (%s)",
		preview.Candidate.ExactCoreVersion,
		current,
		preview.Candidate.CommitSHA,
		preview.Candidate.ManifestSHA256,
		preview.Candidate.SupportLevel,
	)
	if preview.Blocked {
		text += "; blocked: " + preview.BlockReason
	}
	if len(preview.Warnings) != 0 {
		text += "; warnings: " + strings.Join(preview.Warnings, "; ")
	}
	return text
}

func renderQueuedTask(
	cmd *cobra.Command,
	state *options,
	instance *application.Application,
	task application.Task,
	detach bool,
) error {
	if detach {
		return writeResult(cmd.OutOrStdout(), state.format, task, "queued task "+task.ID)
	}
	completed, err := waitForTaskWithCancellationRequest(
		cmd.Context(), instance, task.ID, 250*time.Millisecond, "task_wait_failed",
	)
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

func classifyCapabilityError(code string, err error) error {
	switch {
	case errors.Is(err, application.ErrCapabilityQuarantineInvalid):
		return &Error{Kind: ErrorValidation, Code: "capability_quarantine_invalid", Message: err.Error(), Cause: err}
	case errors.Is(err, application.ErrCapabilityQuarantineConflict):
		return &Error{Kind: ErrorConflict, Code: "capability_quarantine_conflict", Message: err.Error(), Cause: err}
	case errors.Is(err, capability.ErrInvalidGeneration):
		return &Error{Kind: ErrorValidation, Code: "capability_generation_invalid", Message: err.Error(), Cause: err}
	case errors.Is(err, application.ErrCapabilityCandidateQuarantined),
		errors.Is(err, store.ErrCapabilityManifestQuarantined),
		errors.Is(err, store.ErrCapabilityGenerationConflict):
		return &Error{Kind: ErrorConflict, Code: "capability_candidate_blocked", Message: err.Error(), Cause: err}
	case errors.Is(err, store.ErrCapabilityGenerationNotFound),
		errors.Is(err, store.ErrCapabilityManifestNotFound):
		return &Error{Kind: ErrorDomain, Code: "capability_candidate_not_found", Message: err.Error(), Cause: err}
	default:
		return &Error{Kind: ErrorDomain, Code: code, Message: err.Error(), Cause: err}
	}
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
