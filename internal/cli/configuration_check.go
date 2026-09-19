// SPDX-License-Identifier: GPL-3.0-or-later

package cli

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/rehuony/sing-box-panel/internal/application"
	"github.com/rehuony/sing-box-panel/internal/configuration"
	"github.com/rehuony/sing-box-panel/internal/store"
	"github.com/spf13/cobra"
)

const coreFlagUsage = "installed core artifact ID; defaults to the currently applied core"

func newConfigCheckCommand(state *options, open openApplicationFunc) *cobra.Command {
	var coreID string
	var detach bool
	command := &cobra.Command{
		Use:   "check",
		Short: "Run sing-box check on a snapshot of the saved configuration",
		Long: `Run sing-box check on a snapshot of the current saved configuration using
an exact installed, verified binary. Uses the currently applied core by default;
pass --core CORE_ARTIFACT_ID if none has been applied, or to select another.

Waits for completion by default. --detach returns once the check is queued.
The check persists a task and execution snapshot; it does not replace the
saved configuration or start/restart the live core.`,
		Example: `  sing-box-panel config check
  sing-box-panel config check --core CORE_ARTIFACT_ID --detach`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if cmd.Flags().Changed("core") && strings.TrimSpace(coreID) == "" {
				return &Error{Kind: ErrorUsage, Code: "core_required", Message: "--core must name an installed core artifact; see core list"}
			}
			instance, err := openApplication(cmd.Context(), state.settingsPath, open)
			if err != nil {
				return err
			}
			defer instance.Close()
			selected, err := resolveCoreSelection(cmd.Context(), instance, coreID)
			if err != nil {
				return err
			}
			compiled, err := instance.CompileConfiguration(cmd.Context(), application.ConfigurationCompileRequest{CoreArtifactID: selected})
			if err != nil {
				return classifyConfigurationRuntimeError("configuration_check_failed", err)
			}
			target := fmt.Sprintf("core %s (%s)", compiled.Artifact.CoreArtifactID, compiled.Artifact.ExactCoreVersion)
			if detach {
				return writeResult(cmd.OutOrStdout(), state.format, compiled,
					fmt.Sprintf("queued check of canonical revision %s against %s; task %s", compiled.Artifact.CanonicalRevisionID, target, compiled.Task.ID))
			}
			completed, err := waitForTaskWithCancellationRequest(
				cmd.Context(), instance, compiled.Task.ID, 250*time.Millisecond, "configuration_check_wait_failed",
			)
			if err != nil {
				return err
			}
			result := struct {
				Check application.ConfigurationCompile `json:"check"`
				Task  application.Task                 `json:"task"`
			}{compiled, completed}
			if err := writeResult(cmd.OutOrStdout(), state.format, result,
				fmt.Sprintf("configuration check against %s %s", target, completed.Status)); err != nil {
				return err
			}
			return terminalTaskError(completed)
		},
	}
	command.Flags().StringVar(&coreID, "core", "", coreFlagUsage)
	command.Flags().BoolVar(&detach, "detach", false, "return after queuing the check instead of waiting for completion")
	return command
}

// resolveCoreSelection returns the explicit core or the applied one. It never
// guesses from the catalog or a version string.
func resolveCoreSelection(ctx context.Context, instance *application.Application, explicit string) (string, error) {
	if selected := strings.TrimSpace(explicit); selected != "" {
		return selected, nil
	}
	applied, err := instance.AppliedCoreArtifactID(ctx)
	if err != nil {
		return "", &Error{Kind: ErrorDomain, Code: "applied_core_lookup_failed", Message: err.Error(), Cause: err}
	}
	if applied == "" {
		return "", coreRequiredError()
	}
	return applied, nil
}

func coreRequiredError() error {
	return &Error{Kind: ErrorUsage, Code: "core_required", Message: "no core has been applied yet; pass --core CORE_ARTIFACT_ID (see core list)"}
}

func classifyConfigurationRuntimeError(code string, err error) error {
	switch {
	case errors.Is(err, store.ErrConfigurationFileUnparsed):
		return &Error{Kind: ErrorValidation, Code: "configuration_file_unparsed", Message: "saved configuration is not valid JSON; correct it with config import first", Cause: err}
	case errors.Is(err, configuration.ErrInvalidDocument):
		return &Error{Kind: ErrorValidation, Code: "configuration_invalid", Message: err.Error(), Cause: err}
	case errors.Is(err, application.ErrConfigurationSchemaValidation):
		return &Error{Kind: ErrorValidation, Code: "configuration_schema_validation_failed", Message: err.Error(), Cause: err}
	case application.IsCoreArtifactNotFound(err):
		return &Error{Kind: ErrorDomain, Code: "core_artifact_not_found", Message: err.Error(), Cause: err}
	case errors.Is(err, application.ErrCoreArtifactVerificationBlocked):
		return &Error{Kind: ErrorConflict, Code: "core_verification_blocked", Message: err.Error(), Cause: err}
	case errors.Is(err, application.ErrCorePlatformMismatch):
		return &Error{Kind: ErrorConflict, Code: "core_platform_mismatch", Message: err.Error(), Cause: err}
	case errors.Is(err, store.ErrCompiledStartupEvidenceStale):
		return &Error{Kind: ErrorConflict, Code: "configuration_changed", Message: "configuration changed while checking; retry", Cause: err}
	default:
		return &Error{Kind: ErrorDomain, Code: code, Message: err.Error(), Cause: err}
	}
}
