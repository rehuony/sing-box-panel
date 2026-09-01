// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/rehuony/sing-box-panel/internal/application"
	coreruntime "github.com/rehuony/sing-box-panel/internal/runtime"
	"github.com/rehuony/sing-box-panel/internal/store"
)

const (
	runtimeReconcileInterval            = time.Second
	runtimeObservationHeartbeatInterval = 30 * time.Second
)

type runtimeReconciler struct {
	services           *runtimeServices
	clock              taskClock
	exhaustedEpisode   string
	lastUnexpectedExit string
	lastError          string
	runningFence       string
	runningSince       time.Time
	lastHeartbeat      time.Time
	stableFence        string
}

func startRuntimeReconciler(ctx context.Context, services *runtimeServices) <-chan struct{} {
	return startRuntimeReconcilerWithClock(ctx, services, systemClock{})
}

func startRuntimeReconcilerWithClock(
	ctx context.Context,
	services *runtimeServices,
	clock taskClock,
) <-chan struct{} {
	done := make(chan struct{})
	reconciler := &runtimeReconciler{services: services, clock: clock}
	go func() {
		defer close(done)
		ticker := clock.NewTicker(runtimeReconcileInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C():
				reconciler.reconcile(ctx)
			}
		}
	}()
	return done
}

func (reconciler *runtimeReconciler) reconcile(ctx context.Context) {
	live := reconciler.services.manager.ObserveLiveIdentity()
	if live.Running {
		if err := reconciler.observeRunningIncarnation(ctx, live); err != nil {
			reconciler.recordReconcileError(err)
		} else {
			reconciler.lastError = ""
		}
		return
	}
	if live.State != coreruntime.StateFailed {
		reconciler.resetRunningIncarnation()
		reconciler.lastError = ""
		return
	}
	result, err := reconciler.services.reconcileFailedRuntime(ctx)
	if err != nil {
		reconciler.recordReconcileError(err)
		return
	}
	reconciler.lastError = ""
	reconciler.recordUnexpectedExit(ctx, result)
	reconciler.recordRecoveryDecision(result)
}

func (reconciler *runtimeReconciler) recordReconcileError(err error) {
	if err == nil || err.Error() == reconciler.lastError {
		return
	}
	reconciler.lastError = err.Error()
	recordOperationalLog(reconciler.services.commands, application.LogRecordRequest{
		Source: store.LogSourcePanel, Level: store.LogLevelWarn, Code: "runtime.recovery_reconcile_failed",
		Message:  "Automatic runtime recovery reconciliation failed",
		Metadata: mustLogMetadata(map[string]any{"error": err.Error()}),
	})
}

func (reconciler *runtimeReconciler) recordRecoveryDecision(result application.RuntimeRecoveryResult) {
	if result.Task != nil {
		reconciler.exhaustedEpisode = ""
		recordRuntimeRecoveryQueued(reconciler.services.commands, result)
		return
	}
	if result.Exhausted && result.EpisodeID != "" && result.EpisodeID != reconciler.exhaustedEpisode {
		reconciler.exhaustedEpisode = result.EpisodeID
		recordRuntimeRecoveryExhausted(reconciler.services.commands, result)
	}
}

// observeRunningIncarnation requires the same manager-owned PID and OS start
// token to be confirmed on every reconciliation tick. ObservedAt advances at a
// bounded 30-second cadence, while StableObservedAt is set only after one full
// locally witnessed stability window. A heartbeat therefore cannot reset a
// recovery episode by itself.
func (reconciler *runtimeReconciler) observeRunningIncarnation(
	ctx context.Context,
	live coreruntime.LiveIdentity,
) error {
	observation, err := reconciler.services.database.RuntimeObservation(ctx)
	if errors.Is(err, store.ErrRuntimeObservationNotFound) {
		reconciler.resetRunningIncarnation()
		return nil
	}
	if err != nil {
		reconciler.resetRunningIncarnation()
		return err
	}
	if observation.PID != live.PID || observation.ActivationBundleID != live.BundleID ||
		!observation.StartedAt.Equal(live.StartedAt) {
		reconciler.resetRunningIncarnation()
		return errors.New("running manager identity does not match the persisted runtime observation")
	}
	startToken, err := reconciler.services.identity.ProcessStartToken(ctx, live.PID)
	if errors.Is(err, os.ErrNotExist) {
		reconciler.resetRunningIncarnation()
		return nil
	}
	if err != nil {
		reconciler.resetRunningIncarnation()
		return fmt.Errorf("confirm running process incarnation: %w", err)
	}
	if startToken != observation.ProcessStartToken {
		reconciler.resetRunningIncarnation()
		return errors.New("running process start token does not match the persisted runtime observation")
	}

	fence := fmt.Sprintf("%d/%s", observation.PID, observation.ProcessStartToken)
	now := reconciler.clock.Now().UTC()
	if reconciler.runningFence != fence {
		reconciler.runningFence = fence
		reconciler.runningSince = now
		reconciler.lastHeartbeat = observation.ObservedAt
		reconciler.stableFence = ""
		return nil
	}
	if reconciler.stableFence != fence && !now.Before(reconciler.runningSince.Add(store.RuntimeRecoveryStableWindow)) {
		confirmed, err := reconciler.services.database.ConfirmRuntimeObservation(
			ctx, observation.PID, observation.ProcessStartToken, now,
		)
		if err != nil {
			return err
		}
		if !confirmed {
			reconciler.resetRunningIncarnation()
			return errors.New("runtime observation changed before stable-run confirmation")
		}
		reconciler.lastHeartbeat = now
		reconciler.stableFence = fence
		return nil
	}
	if reconciler.lastHeartbeat.IsZero() ||
		!now.Before(reconciler.lastHeartbeat.Add(runtimeObservationHeartbeatInterval)) {
		heartbeat, err := reconciler.services.database.HeartbeatRuntimeObservation(
			ctx, observation.PID, observation.ProcessStartToken, now,
		)
		if err != nil {
			return err
		}
		if !heartbeat {
			reconciler.resetRunningIncarnation()
			return errors.New("runtime observation changed before heartbeat")
		}
		reconciler.lastHeartbeat = now
	}
	return nil
}

func (reconciler *runtimeReconciler) resetRunningIncarnation() {
	reconciler.runningFence = ""
	reconciler.runningSince = time.Time{}
	reconciler.lastHeartbeat = time.Time{}
	reconciler.stableFence = ""
}

func recordRuntimeRecoveryQueued(commands *application.Application, result application.RuntimeRecoveryResult) {
	recordOperationalLog(commands, application.LogRecordRequest{
		Source: store.LogSourcePanel, Level: store.LogLevelWarn, Code: "runtime.recovery_queued",
		Message: "Automatic sing-box recovery attempt queued",
		Metadata: mustLogMetadata(map[string]any{
			"task_id": result.Task.ID, "episode_id": result.EpisodeID, "bundle_id": result.BundleID,
			"generation": result.Task.Generation,
			"attempt":    result.Attempt, "maximum_attempts": store.RuntimeRecoveryMaximumAttempts,
		}),
	})
}

func recordRuntimeRecoveryExhausted(commands *application.Application, result application.RuntimeRecoveryResult) {
	recordOperationalLog(commands, application.LogRecordRequest{
		Source: store.LogSourcePanel, Level: store.LogLevelError, Code: "runtime.recovery_exhausted",
		Message: "Automatic sing-box recovery attempts exhausted",
		Metadata: mustLogMetadata(map[string]any{
			"episode_id": result.EpisodeID, "bundle_id": result.BundleID,
			"generation": result.Generation, "attempt": result.Attempt,
			"attempts": store.RuntimeRecoveryMaximumAttempts,
		}),
	})
}

func (reconciler *runtimeReconciler) recordUnexpectedExit(
	ctx context.Context,
	result application.RuntimeRecoveryResult,
) {
	live := reconciler.services.manager.ObserveLiveIdentity()
	if live.State != coreruntime.StateFailed || live.StartedAt.IsZero() {
		return
	}
	eventKey := fmt.Sprintf("%s/%s", live.BundleID, live.StartedAt.UTC().Format(time.RFC3339Nano))
	if eventKey == reconciler.lastUnexpectedExit {
		return
	}
	generation := result.Generation
	bundleID := result.BundleID
	if generation == 0 || bundleID == "" {
		bootstrap, err := reconciler.services.database.Bootstrap(ctx)
		if err != nil {
			return
		}
		generation = bootstrap.Hub.TargetGeneration
		bundleID = bootstrap.Hub.AppliedBundleID
	}
	if result.Task != nil && result.Task.Generation > 0 {
		generation = result.Task.Generation - 1
	}
	reconciler.lastUnexpectedExit = eventKey
	recordOperationalLog(reconciler.services.commands, application.LogRecordRequest{
		Source: store.LogSourceCore, Level: store.LogLevelError, Code: "runtime.unexpected_exit",
		Message: "Managed sing-box process exited unexpectedly",
		Metadata: mustLogMetadata(map[string]any{
			"bundle_id": bundleID, "generation": generation, "attempt": result.Attempt,
			"started_at": live.StartedAt.UTC(),
		}),
	})
}

func (services *runtimeServices) reconcileFailedRuntime(
	ctx context.Context,
) (application.RuntimeRecoveryResult, error) {
	if services == nil || services.database == nil || services.commands == nil || services.manager == nil {
		return application.RuntimeRecoveryResult{}, nil
	}
	live := services.manager.ObserveLiveIdentity()
	if live.Running || live.State != coreruntime.StateFailed {
		return application.RuntimeRecoveryResult{}, nil
	}
	bootstrap, err := services.database.Bootstrap(ctx)
	if err != nil {
		return application.RuntimeRecoveryResult{}, err
	}
	recoveryEligible := bootstrap.Hub.DesiredRunning && bootstrap.Hub.AppliedBundleID != "" &&
		bootstrap.Hub.DesiredBundleID == bootstrap.Hub.AppliedBundleID

	var expectedObservation *store.RuntimeObservation
	observation, err := services.database.RuntimeObservation(ctx)
	switch {
	case err == nil:
		if proveErr := services.proveCapturedObservationExited(&observation); proveErr != nil {
			// A matching start token proves that this exact PID incarnation still
			// exists, even when executable or artifact identity verification fails.
			return application.RuntimeRecoveryResult{}, proveErr
		}
		expectedObservation = &observation
	case errors.Is(err, store.ErrRuntimeObservationNotFound):
	default:
		return application.RuntimeRecoveryResult{}, err
	}
	reason := runtimeFailureReason(live, "unexpected_exit")
	occurredAt := runtimeFailureTime(live, time.Now())
	transitionTask := store.Task{}
	if recoveryEligible {
		transitionTask.Generation = bootstrap.Hub.TargetGeneration
	}
	var transition store.RuntimeTransitionInput
	if expectedObservation != nil {
		occurredAt = runtimeEventTime(occurredAt, expectedObservation.ObservedAt)
		transition = runtimeTransitionFromObservation(
			runtimeIncarnationTransitionKey(*expectedObservation, store.RuntimeTransitionFailed, reason),
			store.RuntimeTransitionFailed,
			reason,
			*expectedObservation,
			occurredAt,
			nil,
			transitionTask,
		)
	} else {
		bundleID := live.BundleID
		if bundleID == "" {
			bundleID = bootstrap.Hub.AppliedBundleID
		}
		transition = runtimeTransitionWithoutObservation(
			runtimeTransitionDedupeKey(
				"unobserved-failure",
				live.BundleID,
				live.StartedAt.UTC().Format(time.RFC3339Nano),
				reason,
			),
			store.RuntimeTransitionFailed,
			reason,
			bundleID,
			occurredAt,
			nil,
			transitionTask,
		)
	}
	if !recoveryEligible {
		if expectedObservation == nil {
			return application.RuntimeRecoveryResult{}, services.appendRuntimeTransition(ctx, transition)
		}
		cleared, err := services.database.ClearRuntimeObservationAndTransitions(
			ctx,
			expectedObservation.PID,
			expectedObservation.ProcessStartToken,
			[]store.RuntimeTransitionInput{transition},
		)
		if err != nil {
			return application.RuntimeRecoveryResult{}, err
		}
		if !cleared {
			return application.RuntimeRecoveryResult{}, store.ErrRuntimeIdentityMismatch
		}
		return application.RuntimeRecoveryResult{}, nil
	}

	return services.commands.RequestRuntimeRecovery(ctx, application.RuntimeRecoveryRequest{
		ExpectedBundleID:    bootstrap.Hub.AppliedBundleID,
		ExpectedGeneration:  bootstrap.Hub.TargetGeneration,
		ExpectedObservation: expectedObservation,
		StableRunProven:     runtimeObservationProvesStableRun(expectedObservation),
		Transition:          &transition,
	})
}

func runtimeObservationProvesStableRun(observation *store.RuntimeObservation) bool {
	return observation != nil && observation.StableObservedAt != nil
}
