// SPDX-License-Identifier: GPL-3.0-or-later

package cli

import (
	"fmt"
	"strings"

	"github.com/rehuony/sing-box-panel/internal/application"
	"github.com/rehuony/sing-box-panel/internal/store"
	"github.com/spf13/cobra"
)

func newCoreCheckCommand(state *options, open openApplicationFunc) *cobra.Command {
	var detach bool
	command := &cobra.Command{
		Use:   "check STARTUP_ARTIFACT",
		Short: "Queue an exact binary/config check for a pending startup artifact",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			instance, err := openApplication(cmd.Context(), state.settingsPath, open)
			if err != nil {
				return err
			}
			defer instance.Close()
			task, err := instance.QueueStartupCheck(cmd.Context(), args[0])
			if err != nil {
				return classifyRuntimeError("startup_check_queue_failed", err)
			}
			return renderQueuedTask(cmd, state, instance, task, detach)
		},
	}
	command.Flags().BoolVar(&detach, "detach", false, "return after the durable check task is queued")
	return command
}

func newCoreActivateCommand(state *options, open openApplicationFunc) *cobra.Command {
	var monitoring string
	var detach bool
	command := &cobra.Command{
		Use:   "activate STARTUP_ARTIFACT",
		Short: "Freeze and apply one checked startup artifact",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return applyStartupArtifact(cmd, state, open, args[0], monitoring, detach)
		},
	}
	command.Flags().StringVar(&monitoring, "monitoring", string(store.MonitoringProcessOnly), "health evidence tier (process_only or limited)")
	command.Flags().BoolVar(&detach, "detach", false, "return after the durable runtime task is queued")
	return command
}

func newConfigApplyCommand(state *options, open openApplicationFunc) *cobra.Command {
	var artifactID, monitoring string
	var detach bool
	command := &cobra.Command{
		Use:   "apply",
		Short: "Apply one checked startup artifact",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if strings.TrimSpace(artifactID) == "" {
				return &Error{Kind: ErrorUsage, Code: "startup_artifact_required", Message: "--artifact is required"}
			}
			return applyStartupArtifact(cmd, state, open, artifactID, monitoring, detach)
		},
	}
	command.Flags().StringVar(&artifactID, "artifact", "", "ready startup artifact ID")
	command.Flags().StringVar(&monitoring, "monitoring", string(store.MonitoringProcessOnly), "health evidence tier (process_only or limited)")
	command.Flags().BoolVar(&detach, "detach", false, "return after the durable runtime task is queued")
	return command
}

func applyStartupArtifact(
	cmd *cobra.Command,
	state *options,
	open openApplicationFunc,
	startupArtifactID string,
	monitoring string,
	detach bool,
) error {
	instance, err := openApplication(cmd.Context(), state.settingsPath, open)
	if err != nil {
		return err
	}
	defer instance.Close()
	prepared, task, err := instance.PrepareAndQueueRuntimeApply(
		cmd.Context(), startupArtifactID, store.MonitoringTier(monitoring),
	)
	if err != nil {
		return classifyRuntimeError("runtime_apply_queue_failed", err)
	}
	if detach {
		result := struct {
			Activation application.ActivationSummary `json:"activation"`
			Task       application.Task              `json:"task"`
		}{Activation: prepared.Summary(), Task: task}
		return writeResult(
			cmd.OutOrStdout(), state.format, result,
			"prepared bundle "+prepared.Bundle.ID+"; queued task "+task.ID,
		)
	}
	return renderQueuedTask(cmd, state, instance, task, false)
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
		return instance.QueueRuntimeRollback(cmd.Context())
	})
}

func classifyRuntimeError(code string, err error) error {
	switch {
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
