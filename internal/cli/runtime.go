// SPDX-License-Identifier: GPL-3.0-or-later

package cli

import (
	"errors"
	"fmt"

	"github.com/rehuony/sing-box-panel/internal/application"
	"github.com/rehuony/sing-box-panel/internal/configuration"
	"github.com/rehuony/sing-box-panel/internal/panelprocess"
	"github.com/rehuony/sing-box-panel/internal/store"
	"github.com/spf13/cobra"
)

func newCoreEnableCommand(state *options, open openApplicationFunc) *cobra.Command {
	var selection coreSelection
	command := &cobra.Command{
		Use:   "enable VERSION",
		Short: "Select an installed core while preserving its running or stopped state",
		Long: `Check and select a version. A stopped core stays stopped; a running
core is restarted with the selected version. The saved configuration is
carried forward unchanged: the selected binary must accept an execution
snapshot of that configuration with "sing-box check" before the live process
is replaced. A failed preflight leaves the running core and the saved file untouched.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return enableCore(cmd, state, open, args[0], selection)
		},
	}
	configureCoreSelection(command, &selection, true, state, open)
	return command
}

func enableCore(cmd *cobra.Command, state *options, open openApplicationFunc, version string, selection coreSelection) error {
	instance, err := openApplication(cmd.Context(), state.settingsPath, open)
	if err != nil {
		return err
	}
	defer instance.Close()
	artifact, err := resolveInstalledCore(cmd, instance, version, selection)
	if err != nil {
		return err
	}
	result, err := instance.EnableCore(cmd.Context(), artifact.ID)
	if err != nil {
		return classifyConfigurationRuntimeError("runtime_apply_failed", err)
	}
	return writeResult(cmd.OutOrStdout(), state.format, result, "Core: "+result.ObservationState)
}

func newCoreStatusCommand(state *options, open openApplicationFunc) *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show desired, applied, rollback, and actual live identities",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			instance, err := openApplication(cmd.Context(), state.settingsPath, open)
			if err != nil {
				return err
			}
			defer instance.Close()
			status, err := instance.RuntimeStatus(cmd.Context())
			if err != nil {
				return classifyRuntimeError("runtime_status_failed", err)
			}
			text := fmt.Sprintf(
				"%s\tdesired=%t\tapplied=%s\trollback=%s",
				status.ObservationState, status.DesiredRunning,
				emptyAsDash(status.AppliedBundleID), emptyAsDash(status.RollbackBundleID),
			)
			return writeResult(cmd.OutOrStdout(), state.format, status, text)
		},
	}
}

func newCoreLifecycleCommand(
	name string,
	state *options,
	open openApplicationFunc,
	execute func(*application.Application, *cobra.Command) (application.RuntimeStatus, error),
) *cobra.Command {
	command := &cobra.Command{
		Use:   name,
		Short: "Manage " + name + " sing-box runtime",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			instance, err := openApplication(cmd.Context(), state.settingsPath, open)
			if err != nil {
				return err
			}
			defer instance.Close()
			result, err := execute(instance, cmd)
			if err != nil {
				return classifyRuntimeError("runtime_"+name+"_failed", err)
			}
			return writeResult(cmd.OutOrStdout(), state.format, result, "Core: "+result.ObservationState)
		},
	}
	return command
}

func newCoreStartCommand(state *options, open openApplicationFunc) *cobra.Command {
	return newCoreLifecycleCommand("start", state, open, func(instance *application.Application, cmd *cobra.Command) (application.RuntimeStatus, error) {
		return instance.StartRuntime(cmd.Context())
	})
}

func newCoreStopCommand(state *options, open openApplicationFunc) *cobra.Command {
	return newCoreLifecycleCommand("stop", state, open, func(instance *application.Application, cmd *cobra.Command) (application.RuntimeStatus, error) {
		return instance.StopRuntime(cmd.Context())
	})
}

func newCoreRestartCommand(state *options, open openApplicationFunc) *cobra.Command {
	return newCoreLifecycleCommand("restart", state, open, func(instance *application.Application, cmd *cobra.Command) (application.RuntimeStatus, error) {
		return instance.RestartRuntime(cmd.Context())
	})
}

func newCoreRollbackCommand(state *options, open openApplicationFunc) *cobra.Command {
	return newCoreLifecycleCommand("rollback", state, open, func(instance *application.Application, cmd *cobra.Command) (application.RuntimeStatus, error) {
		return instance.RollbackRuntime(cmd.Context(), "")
	})
}

func classifyRuntimeError(code string, err error) error {
	switch {
	case errors.Is(err, application.ErrConfigurationNotSaved):
		return &Error{Kind: ErrorValidation, Code: "configuration_not_saved", Message: err.Error(), Cause: err}
	case errors.Is(err, panelprocess.ErrUnavailable):
		return &Error{Kind: ErrorUnavailable, Code: "panel_unavailable", Message: err.Error(), Cause: err}
	case errors.Is(err, store.ErrConfigurationFileUnparsed):
		return &Error{Kind: ErrorValidation, Code: "configuration_file_unparsed", Message: "saved sing-box configuration is not valid JSON; correct it in the Web UI first", Cause: err}
	case application.IsMonitoringTierUnavailable(err):
		return &Error{Kind: ErrorUnavailable, Code: "monitoring_tier_unavailable", Message: err.Error(), Cause: err}
	case application.IsActivationBundleNotReady(err):
		return &Error{Kind: ErrorConflict, Code: "activation_bundle_not_ready", Message: err.Error(), Cause: err}
	case application.IsNoAppliedBundle(err):
		return &Error{Kind: ErrorUnavailable, Code: "no_applied_bundle", Message: err.Error(), Cause: err}
	case application.IsNoRollbackBundle(err):
		return &Error{Kind: ErrorUnavailable, Code: "no_rollback_bundle", Message: err.Error(), Cause: err}
	case application.IsStartupArtifactNotFound(err):
		return &Error{Kind: ErrorDomain, Code: "startup_artifact_not_found", Message: err.Error(), Cause: err}
	default:
		return &Error{Kind: ErrorDomain, Code: code, Message: err.Error(), Cause: err}
	}
}

func emptyAsDash(value string) string {
	if value == "" {
		return "-"
	}
	return value
}

func classifyConfigurationRuntimeError(code string, err error) error {
	switch {
	case errors.Is(err, application.ErrConfigurationNotSaved):
		return &Error{Kind: ErrorValidation, Code: "configuration_not_saved", Message: err.Error(), Cause: err}
	case errors.Is(err, panelprocess.ErrUnavailable):
		return &Error{Kind: ErrorUnavailable, Code: "panel_unavailable", Message: err.Error(), Cause: err}
	case errors.Is(err, store.ErrConfigurationFileUnparsed):
		return &Error{Kind: ErrorValidation, Code: "configuration_file_unparsed", Message: "saved sing-box configuration is not valid JSON; correct it in the Web UI first", Cause: err}
	case errors.Is(err, configuration.ErrInvalidDocument):
		return &Error{Kind: ErrorValidation, Code: "configuration_invalid", Message: err.Error(), Cause: err}
	case errors.Is(err, application.ErrConfigurationSchemaValidation):
		return &Error{Kind: ErrorValidation, Code: "configuration_schema_validation_failed", Message: err.Error(), Cause: err}
	case application.IsCoreArtifactNotFound(err):
		return &Error{Kind: ErrorDomain, Code: "core_artifact_not_found", Message: err.Error(), Cause: err}
	case errors.Is(err, application.ErrCorePlatformMismatch):
		return &Error{Kind: ErrorConflict, Code: "core_platform_mismatch", Message: err.Error(), Cause: err}
	case errors.Is(err, store.ErrCompiledStartupEvidenceStale):
		return &Error{Kind: ErrorConflict, Code: "configuration_changed", Message: "configuration changed while checking; retry", Cause: err}
	default:
		return &Error{Kind: ErrorDomain, Code: code, Message: err.Error(), Cause: err}
	}
}
