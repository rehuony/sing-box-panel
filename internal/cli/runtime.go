// SPDX-License-Identifier: GPL-3.0-or-later

package cli

import (
	"errors"
	"fmt"
	"strings"

	"github.com/rehuony/sing-box-panel/internal/application"
	"github.com/rehuony/sing-box-panel/internal/store"
	"github.com/spf13/cobra"
)

func newCoreEnableCommand(state *options, open openApplicationFunc) *cobra.Command {
	var detach bool
	command := &cobra.Command{
		Use:   "enable CORE_ARTIFACT_ID",
		Short: "Switch the runtime to one verified installed core using the current saved configuration",
		Long: `Queue a checked replacement of the running core. The saved configuration is
carried forward unchanged: the selected binary must accept an execution
snapshot of that configuration with "sing-box check" before the live process
is replaced. A failed preflight leaves the running core and the saved file untouched.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			coreID := strings.TrimSpace(args[0])
			if coreID == "" {
				return &Error{Kind: ErrorUsage, Code: "core_required", Message: "CORE_ARTIFACT_ID must not be blank; see core list"}
			}
			return applyConfiguration(cmd, state, open, coreID, detach)
		},
	}
	command.Flags().BoolVar(&detach, "detach", false, "return after the durable runtime task is queued")
	return command
}

func newConfigApplyCommand(state *options, open openApplicationFunc) *cobra.Command {
	var coreID string
	var detach bool
	command := &cobra.Command{
		Use:   "apply",
		Short: "Check the current saved configuration and restart the core with it",
		Long: `Snapshot the current valid saved configuration, run the selected core's
"sing-box check" in the serialized runtime lane, and restart the core with
those bytes only after the check succeeds. Without --core the currently applied
core is kept; --core CORE_ARTIFACT_ID switches to another verified installed
core in the same step. Preflight failure leaves the live core and the saved
file unchanged.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if cmd.Flags().Changed("core") && strings.TrimSpace(coreID) == "" {
				return &Error{Kind: ErrorUsage, Code: "core_required", Message: "--core must name an installed core artifact; see core list"}
			}
			return applyConfiguration(cmd, state, open, coreID, detach)
		},
	}
	command.Flags().StringVar(&coreID, "core", "", coreFlagUsage)
	command.Flags().BoolVar(&detach, "detach", false, "return after the durable runtime task is queued")
	return command
}

func applyConfiguration(cmd *cobra.Command, state *options, open openApplicationFunc, coreID string, detach bool) error {
	instance, err := openApplication(cmd.Context(), state.settingsPath, open)
	if err != nil {
		return err
	}
	defer instance.Close()
	var task application.Task
	if coreID = strings.TrimSpace(coreID); coreID != "" {
		task, err = instance.EnableCore(cmd.Context(), coreID)
	} else {
		task, err = instance.QueueRuntimeRestart(cmd.Context())
		if application.IsNoAppliedBundle(err) {
			return coreRequiredError()
		}
	}
	if err != nil {
		return classifyConfigurationRuntimeError("runtime_apply_queue_failed", err)
	}
	return renderQueuedTask(cmd, state, instance, task, detach)
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
	queue func(*application.Application, *cobra.Command) (application.Task, error),
) *cobra.Command {
	var detach bool
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
			task, err := queue(instance, cmd)
			if err != nil {
				return classifyRuntimeError("runtime_"+name+"_queue_failed", err)
			}
			return renderQueuedTask(cmd, state, instance, task, detach)
		},
	}
	command.Flags().BoolVar(&detach, "detach", false, "return after the durable runtime task is queued")
	return command
}

func newCoreStartCommand(state *options, open openApplicationFunc) *cobra.Command {
	return newCoreLifecycleCommand("start", state, open, func(instance *application.Application, cmd *cobra.Command) (application.Task, error) {
		return instance.QueueRuntimeStart(cmd.Context())
	})
}

func newCoreStopCommand(state *options, open openApplicationFunc) *cobra.Command {
	return newCoreLifecycleCommand("stop", state, open, func(instance *application.Application, cmd *cobra.Command) (application.Task, error) {
		return instance.QueueRuntimeStop(cmd.Context())
	})
}

func newCoreRestartCommand(state *options, open openApplicationFunc) *cobra.Command {
	return newCoreLifecycleCommand("restart", state, open, func(instance *application.Application, cmd *cobra.Command) (application.Task, error) {
		return instance.QueueRuntimeRestart(cmd.Context())
	})
}

func newCoreRollbackCommand(state *options, open openApplicationFunc) *cobra.Command {
	return newCoreLifecycleCommand("rollback", state, open, func(instance *application.Application, cmd *cobra.Command) (application.Task, error) {
		return instance.QueueRuntimeRollback(cmd.Context(), "")
	})
}

func classifyRuntimeError(code string, err error) error {
	switch {
	case errors.Is(err, store.ErrConfigurationFileUnparsed):
		return &Error{Kind: ErrorValidation, Code: "configuration_file_unparsed", Message: "saved configuration is not valid JSON; correct it with config import first", Cause: err}
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
