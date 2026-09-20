// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/rehuony/sing-box-panel/internal/application"
	"github.com/rehuony/sing-box-panel/internal/corelogs"
	coreruntime "github.com/rehuony/sing-box-panel/internal/runtime"
	"github.com/rehuony/sing-box-panel/internal/settings"
	"github.com/rehuony/sing-box-panel/internal/store"
)

type runtimeServices struct {
	database *store.Store
	commands *application.Application
	manager  runtimeManager
	identity runtimeIdentityResolver
}

type runtimeManager interface {
	Check(context.Context, coreruntime.AppliedBundle) error
	Start(context.Context, coreruntime.AppliedBundle) error
	Stop(context.Context) error
	Restart(context.Context, coreruntime.AppliedBundle) error
	Close(context.Context) error
	Wait() error
	MonitoringLevel() coreruntime.MonitoringLevel
	ObserveLiveIdentity() coreruntime.LiveIdentity
}

type runtimeIdentityResolver interface {
	Resolve(context.Context) (application.RuntimeIdentity, error)
	ProcessStartToken(context.Context, int) (string, error)
}

func newRuntimeServices(
	database *store.Store,
	commands *application.Application,
	configuration settings.Settings,
) (*runtimeServices, error) {
	logs, err := corelogs.New(configuration.DataDir)
	if err != nil {
		return nil, err
	}
	manager, err := coreruntime.NewManager(coreruntime.Options{
		RuntimeDir: filepath.Join(configuration.DataDir, "runtime"),
		Stdout:     logs.Writer(), Stderr: logs.Writer(),
		ObserveOutput: logs.Follow,
	})
	if err != nil {
		return nil, err
	}
	return &runtimeServices{
		database: database, commands: commands, manager: manager,
		identity: application.NewRuntimeIdentityResolver(database),
	}, nil
}

// ReconcileStartup refuses to adopt a live unowned process and routes startup
// convergence through the same fenced, bounded recovery history used at
// runtime. An active durable intent remains the sole owner of convergence.
func (services *runtimeServices) ReconcileStartup(ctx context.Context) error {
	history, err := services.database.ListRuntimeTransitions(ctx, store.RuntimeHistoryFilter{Limit: 1})
	if err != nil {
		return err
	}
	reconciledAt := runtimeEventTime(time.Now(), history.HistoryStartedAt)
	var expectedObservation *store.RuntimeObservation
	observation, err := services.database.RuntimeObservation(ctx)
	if err == nil {
		if proveErr := services.proveCapturedObservationExited(&observation); proveErr != nil {
			uncertainSince := runtimeEventTime(observation.ObservedAt, history.HistoryStartedAt)
			transition := runtimeTransitionFromObservation(
				runtimeIncarnationTransitionKey(observation, store.RuntimeTransitionUnknown, "panel_restart_gap"),
				store.RuntimeTransitionUnknown,
				"panel_restart_gap",
				observation,
				uncertainSince,
				&uncertainSince,
				store.Task{},
			)
			_, historyErr := services.database.AppendRuntimeObservationTransition(ctx, observation, transition)
			return errors.Join(fmt.Errorf("reconcile runtime observation: %w", proveErr), historyErr)
		}
		expectedObservation = &observation
	} else if !errors.Is(err, store.ErrRuntimeObservationNotFound) {
		return err
	}

	bootstrap, err := services.database.Bootstrap(ctx)
	if err != nil {
		return err
	}
	if !bootstrap.Hub.DesiredRunning || bootstrap.Hub.AppliedBundleID == "" ||
		bootstrap.Hub.DesiredBundleID != bootstrap.Hub.AppliedBundleID {
		if expectedObservation == nil {
			return services.ensureRuntimeStopped(ctx, reconciledAt, "startup_reconciled_stopped")
		}
		uncertainSince := runtimeEventTime(expectedObservation.ObservedAt, history.HistoryStartedAt)
		unknown := runtimeTransitionFromObservation(
			runtimeIncarnationTransitionKey(*expectedObservation, store.RuntimeTransitionUnknown, "panel_restart_gap"),
			store.RuntimeTransitionUnknown,
			"panel_restart_gap",
			*expectedObservation,
			uncertainSince,
			&uncertainSince,
			store.Task{},
		)
		stopped := runtimeTransitionFromObservation(
			runtimeIncarnationTransitionKey(*expectedObservation, store.RuntimeTransitionStopped, "startup_reconciled_stopped"),
			store.RuntimeTransitionStopped,
			"startup_reconciled_stopped",
			*expectedObservation,
			runtimeEventTime(reconciledAt, uncertainSince),
			nil,
			store.Task{},
		)
		cleared, clearErr := services.database.ClearRuntimeObservationAndTransitions(
			ctx,
			expectedObservation.PID,
			expectedObservation.ProcessStartToken,
			[]store.RuntimeTransitionInput{unknown, stopped},
		)
		if clearErr != nil {
			return clearErr
		}
		if !cleared {
			return store.ErrRuntimeIdentityMismatch
		}
		return nil
	}
	var transition *store.RuntimeTransitionInput
	if expectedObservation != nil {
		uncertainSince := runtimeEventTime(expectedObservation.ObservedAt, history.HistoryStartedAt)
		value := runtimeTransitionFromObservation(
			runtimeIncarnationTransitionKey(*expectedObservation, store.RuntimeTransitionUnknown, "panel_restart_gap"),
			store.RuntimeTransitionUnknown,
			"panel_restart_gap",
			*expectedObservation,
			uncertainSince,
			&uncertainSince,
			store.Task{Generation: bootstrap.Hub.TargetGeneration},
		)
		transition = &value
	}
	recovery, err := services.commands.RequestRuntimeRecovery(ctx, application.RuntimeRecoveryRequest{
		ExpectedBundleID:    bootstrap.Hub.AppliedBundleID,
		ExpectedGeneration:  bootstrap.Hub.TargetGeneration,
		ExpectedObservation: expectedObservation,
		StableRunProven:     runtimeObservationProvesStableRun(expectedObservation),
		// A successful recovery with no remaining observation is the durable
		// shape left by a clean panel shutdown. Failed/crashed children retain
		// their observation and therefore cannot consume this reset boundary.
		CleanBoundaryProven: expectedObservation == nil,
		Transition:          transition,
	})
	if err != nil {
		return err
	}
	if recovery.Task != nil {
		recordRuntimeRecoveryQueued(services.commands, recovery)
	}
	if recovery.Exhausted {
		recordRuntimeRecoveryExhausted(services.commands, recovery)
	}
	return nil
}

func (services *runtimeServices) Close() error {
	if services == nil || services.manager == nil {
		return nil
	}
	observation, observationErr := services.captureRuntimeObservation(context.Background())
	ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	closeErr := services.manager.Close(ctx)
	waitErr := services.manager.Wait()
	if observationErr != nil {
		closeErr = errors.Join(closeErr, observationErr)
	} else if closeErr == nil && waitErr == nil {
		if observation != nil {
			live := services.manager.ObserveLiveIdentity()
			occurredAt := runtimeEventTime(runtimeTransitionTime(live, time.Now()), observation.ObservedAt)
			transition := runtimeTransitionFromObservation(
				runtimeIncarnationTransitionKey(*observation, store.RuntimeTransitionStopped, "panel_shutdown"),
				store.RuntimeTransitionStopped,
				"panel_shutdown",
				*observation,
				occurredAt,
				nil,
				store.Task{},
			)
			_, transitionErr := services.database.ClearRuntimeObservationAndTransitions(
				ctx,
				observation.PID,
				observation.ProcessStartToken,
				[]store.RuntimeTransitionInput{transition},
			)
			closeErr = errors.Join(closeErr, transitionErr)
		}
	} else {
		closeErr = errors.Join(closeErr, services.clearCapturedObservationAfterFailedStop(observation))
	}
	return errors.Join(closeErr, waitErr)
}

func startupCheckHandler(
	commands *application.Application,
	manager runtimeManager,
) taskHandlerFunc {
	return func(ctx context.Context, task store.Task, control taskExecutionControl) (json.RawMessage, error) {
		if err := control.SafePoint(ctx); err != nil {
			return nil, err
		}
		material, err := commands.LoadStartupCheckMaterial(ctx, task.StartupArtifactID)
		if err != nil {
			return nil, err
		}
		checkErr := manager.Check(ctx, material.Bundle)
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if err := control.SafePoint(ctx); err != nil {
			return nil, err
		}
		succeeded := checkErr == nil
		completed, completeErr := commands.CompleteStartupCheck(
			ctx, task.StartupArtifactID, succeeded,
		)
		if completeErr != nil {
			return nil, errors.Join(checkErr, completeErr)
		}
		result, marshalErr := json.Marshal(map[string]any{
			"startup_artifact_id": completed.ID,
			"state":               completed.State,
			"config_sha256":       completed.ConfigSHA256,
		})
		if checkErr != nil {
			return result, checkErr
		}
		return result, marshalErr
	}
}

func runtimeIntentHandler(services *runtimeServices) taskResultHandlerFunc {
	return func(ctx context.Context, task store.Task, control taskExecutionControl) (taskHandlerResult, error) {
		if err := control.SafePoint(ctx); err != nil {
			return taskHandlerResult{}, err
		}
		if store.RuntimeIntentKind(task.Kind) == store.RuntimeIntentStop {
			return services.stopForTask(ctx, task, control)
		}
		if task.StartupArtifactID != "" && task.ActivationBundleID == "" {
			if store.RuntimeIntentKind(task.Kind) == store.RuntimeIntentStart && services.manager.ObserveLiveIdentity().Running {
				return taskHandlerResult{}, errors.New("core is already running; restart to load saved configuration")
			}
			var err error
			task, err = services.checkConfigurationForTask(ctx, task, control)
			if err != nil {
				return taskHandlerResult{}, err
			}
		}
		material, err := services.commands.LoadRuntimeMaterial(ctx, task.ActivationBundleID)
		if err != nil {
			return taskHandlerResult{}, err
		}
		processMonitoringTier := store.MonitoringTier(services.manager.MonitoringLevel())
		if processMonitoringTier != store.MonitoringProcessOnly ||
			(material.Activation.MonitoringTier != store.MonitoringProcessOnly &&
				material.Activation.MonitoringTier != store.MonitoringLimited) {
			return taskHandlerResult{}, fmt.Errorf(
				"activation monitoring tier %q is unavailable; process probe supplies %q",
				material.Activation.MonitoringTier,
				processMonitoringTier,
			)
		}
		if err := services.revalidateRuntimeMaterial(ctx, material); err != nil {
			return taskHandlerResult{}, err
		}
		if store.CoreSelectionOnly(task) {
			return services.selectStoppedCore(ctx, task, control)
		}
		capturedObservation, err := services.captureRuntimeObservation(ctx)
		if err != nil {
			return taskHandlerResult{}, err
		}
		var recordedObservation *store.RuntimeObservation
		live := services.manager.ObserveLiveIdentity()
		alreadyExact := live.Running && live.BundleID == material.Bundle.ID &&
			live.ArtifactID == material.Bundle.ArtifactID && live.ExactVersion == material.Bundle.ExactVersion &&
			live.ArtifactDigest == material.Bundle.ArtifactDigest
		startedByTask := false
		if runtimeIntentNeedsTransition(store.RuntimeIntentKind(task.Kind), alreadyExact) {
			if err := control.SafePoint(ctx); err != nil {
				return taskHandlerResult{}, err
			}
			if live.Running {
				err = services.manager.Restart(ctx, material.Bundle)
			} else {
				err = services.manager.Start(ctx, material.Bundle)
			}
			if err != nil {
				commit, evidenceErr := services.runtimeTaskFailureCommit(task, material, capturedObservation)
				return taskHandlerResult{Runtime: commit}, errors.Join(err, evidenceErr)
			}
			startedByTask = true
			observation, recordErr := services.recordLiveObservation(
				ctx, material, capturedObservation,
			)
			if recordErr != nil {
				commit, stopErr := services.stopAfterLostIntent(
					capturedObservation,
					task,
					material.Activation.ID,
					store.RuntimeTransitionFailed,
					"observation_record_failed",
				)
				return taskHandlerResult{Runtime: commit}, errors.Join(recordErr, stopErr)
			}
			recordedObservation = &observation
			if err := services.revalidateRuntimeMaterial(ctx, material); err != nil {
				commit, stopErr := services.stopAfterLostIntent(
					recordedObservation,
					task,
					material.Activation.ID,
					store.RuntimeTransitionStopped,
					"intent_superseded",
				)
				return taskHandlerResult{Runtime: commit}, errors.Join(err, stopErr)
			}
		}
		if material.Activation.MonitoringTier == store.MonitoringLimited {
			if err := services.awaitClashAPI(ctx, material); err != nil {
				if startedByTask {
					commit, stopErr := services.stopAfterLostIntent(
						recordedObservation,
						task,
						material.Activation.ID,
						store.RuntimeTransitionFailed,
						"monitoring_handshake_failed",
					)
					return taskHandlerResult{Runtime: commit}, errors.Join(err, stopErr)
				}
				return taskHandlerResult{}, err
			}
		}
		if err := control.SafePoint(ctx); err != nil {
			if startedByTask {
				commit, stopErr := services.stopAfterLostIntent(
					recordedObservation,
					task,
					material.Activation.ID,
					store.RuntimeTransitionStopped,
					"intent_superseded",
				)
				return taskHandlerResult{Runtime: commit}, errors.Join(err, stopErr)
			}
			return taskHandlerResult{}, err
		}
		if recordedObservation == nil {
			observation, recordErr := services.recordLiveObservation(
				ctx, material, capturedObservation,
			)
			if recordErr != nil {
				if startedByTask {
					commit, stopErr := services.stopAfterLostIntent(
						recordedObservation,
						task,
						material.Activation.ID,
						store.RuntimeTransitionFailed,
						"observation_record_failed",
					)
					return taskHandlerResult{Runtime: commit}, errors.Join(recordErr, stopErr)
				}
				return taskHandlerResult{}, recordErr
			}
			recordedObservation = &observation
		}
		if err := control.SafePoint(ctx); err != nil {
			if startedByTask {
				commit, stopErr := services.stopAfterLostIntent(
					recordedObservation,
					task,
					material.Activation.ID,
					store.RuntimeTransitionStopped,
					"intent_superseded",
				)
				return taskHandlerResult{Runtime: commit}, errors.Join(err, stopErr)
			}
			return taskHandlerResult{}, err
		}
		observation, runtimeCommit, transitionErr := services.prepareRuntimeTaskRunningCommit(
			ctx,
			task,
			*recordedObservation,
			capturedObservation,
		)
		if transitionErr != nil {
			if startedByTask {
				commit, stopErr := services.stopAfterLostIntent(
					recordedObservation,
					task,
					material.Activation.ID,
					store.RuntimeTransitionFailed,
					"observation_transition_failed",
				)
				return taskHandlerResult{Runtime: commit}, errors.Join(transitionErr, stopErr)
			}
			return taskHandlerResult{}, transitionErr
		}
		recordedObservation = &observation
		payload, err := json.Marshal(map[string]any{
			"healthy": true, "monitoring_tier": material.Activation.MonitoringTier,
			"runtime": *recordedObservation,
		})
		return taskHandlerResult{Payload: payload, Runtime: &runtimeCommit}, err
	}
}

func (services *runtimeServices) awaitClashAPI(ctx context.Context, material application.RuntimeMaterial) error {
	endpoint, err := coreruntime.ParseClashEndpoint(material.Bundle.StartupConfig)
	if err != nil {
		return err
	}
	client, err := coreruntime.NewClashClient(endpoint)
	if err != nil {
		return err
	}
	handshakeContext, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	version, err := client.Version(handshakeContext)
	if err != nil {
		return fmt.Errorf("limited monitoring handshake: %w", err)
	}
	if version != material.Core.ExactVersion {
		return fmt.Errorf("limited monitoring handshake: core version %q does not match %q", version, material.Core.ExactVersion)
	}
	return nil
}

func runtimeIntentNeedsTransition(kind store.RuntimeIntentKind, alreadyExact bool) bool {
	return kind == store.RuntimeIntentRestart || !alreadyExact
}

func (services *runtimeServices) revalidateRuntimeMaterial(
	ctx context.Context,
	material application.RuntimeMaterial,
) error {
	current, err := services.commands.LoadRuntimeMaterial(ctx, material.Activation.ID)
	if err != nil {
		return err
	}
	if current.Startup.ID != material.Startup.ID ||
		current.Core.ID != material.Core.ID ||
		current.Bundle.ExactVersion != material.Bundle.ExactVersion ||
		current.Bundle.ArtifactDigest != material.Bundle.ArtifactDigest ||
		current.Bundle.StartupConfigDigest != material.Bundle.StartupConfigDigest {
		return store.ErrActivationBundleNotReady
	}
	return nil
}

func (services *runtimeServices) stopForTask(
	ctx context.Context,
	task store.Task,
	control taskExecutionControl,
) (taskHandlerResult, error) {
	observation, err := services.captureRuntimeObservation(ctx)
	if err != nil {
		return taskHandlerResult{}, err
	}
	// Fence the task immediately before changing the external process. The
	// second fence below detects cancellation or supersession that races with a
	// successful Stop, but the resulting runtime evidence must still be returned.
	if err := control.SafePoint(ctx); err != nil {
		return taskHandlerResult{}, err
	}
	if err := services.manager.Stop(ctx); err != nil {
		commit, evidenceErr := services.runtimeCommitAfterFailedTaskStop(observation, task)
		return taskHandlerResult{Runtime: commit}, errors.Join(err, evidenceErr)
	}
	live := services.manager.ObserveLiveIdentity()
	occurredAt := runtimeTransitionTime(live, time.Now())
	commit := store.RuntimeTaskCommit{
		ExpectedObservation: observation,
		ClearObservation:    true,
		Transitions:         make([]store.RuntimeTransitionInput, 0, 1),
	}
	if observation != nil {
		occurredAt = runtimeEventTime(occurredAt, observation.ObservedAt)
		transition := runtimeTransitionFromObservation(
			runtimeTaskTransitionKey(task, "stopped"),
			store.RuntimeTransitionStopped,
			"stop_succeeded",
			*observation,
			occurredAt,
			nil,
			task,
		)
		commit.Transitions = append(commit.Transitions, transition)
	} else {
		transition := runtimeTransitionWithoutObservation(
			runtimeTaskTransitionKey(task, "stopped"),
			store.RuntimeTransitionStopped,
			"stop_succeeded",
			task.ActivationBundleID,
			occurredAt,
			nil,
			task,
		)
		commit.Transitions = append(commit.Transitions, transition)
	}
	if err := control.SafePoint(ctx); err != nil {
		return taskHandlerResult{Runtime: &commit}, err
	}
	return taskHandlerResult{Payload: json.RawMessage(`{"running":false}`), Runtime: &commit}, nil
}

func (services *runtimeServices) prepareRuntimeTaskRunningCommit(
	ctx context.Context,
	task store.Task,
	observation store.RuntimeObservation,
	previous *store.RuntimeObservation,
) (store.RuntimeObservation, store.RuntimeTaskCommit, error) {
	live := services.manager.ObserveLiveIdentity()
	if !live.Running || live.PID != observation.PID || live.BundleID != observation.ActivationBundleID ||
		!live.StartedAt.Equal(observation.StartedAt) {
		return store.RuntimeObservation{}, store.RuntimeTaskCommit{}, coreruntime.ErrNotRunning
	}
	startToken, err := services.identity.ProcessStartToken(ctx, live.PID)
	if err != nil {
		return store.RuntimeObservation{}, store.RuntimeTaskCommit{}, err
	}
	if startToken != observation.ProcessStartToken {
		return store.RuntimeObservation{}, store.RuntimeTaskCommit{}, store.ErrRuntimeIdentityMismatch
	}
	expected := observation
	observation.ObservedAt = runtimeEventTime(time.Now(), observation.ObservedAt)
	transitions := make([]store.RuntimeTransitionInput, 0, 2)
	if previous != nil && !sameRuntimeIncarnation(*previous, observation) {
		boundaryAt := runtimeEventTime(live.StartedAt, previous.ObservedAt)
		transitions = append(transitions, runtimeTransitionFromObservation(
			runtimeTaskTransitionKey(task, "boundary"),
			store.RuntimeTransitionStopped,
			runtimeTaskReason(task, "boundary"),
			*previous,
			boundaryAt,
			nil,
			task,
		))
	}
	transitionAt := runtimeEventTime(runtimeTransitionTime(live, observation.ObservedAt), live.StartedAt)
	transitions = append(transitions, runtimeTransitionFromObservation(
		runtimeTaskTransitionKey(task, "running"),
		store.RuntimeTransitionRunning,
		runtimeTaskReason(task, "succeeded"),
		observation,
		transitionAt,
		nil,
		task,
	))
	return observation, store.RuntimeTaskCommit{
		ExpectedObservation: &expected,
		Observation:         &observation,
		Transitions:         transitions,
	}, nil
}

func sameRuntimeIncarnation(left, right store.RuntimeObservation) bool {
	return left.PID == right.PID &&
		left.ProcessStartToken == right.ProcessStartToken &&
		left.StartedAt.Equal(right.StartedAt)
}

func (services *runtimeServices) recordLiveObservation(
	ctx context.Context,
	material application.RuntimeMaterial,
	expected *store.RuntimeObservation,
) (store.RuntimeObservation, error) {
	live := services.manager.ObserveLiveIdentity()
	if !live.Running || live.PID <= 0 {
		return store.RuntimeObservation{}, coreruntime.ErrNotRunning
	}
	startToken, err := services.identity.ProcessStartToken(ctx, live.PID)
	if err != nil {
		return store.RuntimeObservation{}, err
	}
	observedAt := runtimeEventTime(time.Now(), live.StartedAt)
	observation := store.RuntimeObservation{
		PID: live.PID, ProcessStartToken: startToken, CoreArtifactID: material.Core.ID,
		ActivationBundleID: material.Activation.ID, ExactCoreVersion: material.Core.ExactVersion,
		ArchiveSHA256: material.Core.ArchiveSHA256, BinarySHA256: material.Core.BinarySHA256,
		StartedAt: live.StartedAt, ObservedAt: observedAt,
	}
	return services.database.RecordRuntimeObservationAndTransitions(
		ctx,
		expected,
		observation,
		nil,
	)
}

func (services *runtimeServices) stopAfterLostIntent(
	observation *store.RuntimeObservation,
	task store.Task,
	bundleID string,
	state store.RuntimeTransitionState,
	reason string,
) (*store.RuntimeTaskCommit, error) {
	stopCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	if err := services.manager.Stop(stopCtx); err != nil {
		commit, evidenceErr := services.runtimeCommitAfterFailedTaskStop(observation, task)
		return commit, errors.Join(err, evidenceErr)
	}
	live := services.manager.ObserveLiveIdentity()
	occurredAt := runtimeTransitionTime(live, time.Now())
	commit := store.RuntimeTaskCommit{
		ExpectedObservation: observation,
		ClearObservation:    true,
		Transitions:         make([]store.RuntimeTransitionInput, 0, 1),
	}
	if observation != nil {
		occurredAt = runtimeEventTime(occurredAt, observation.ObservedAt)
		var transition store.RuntimeTransitionInput
		if observation.ActivationBundleID == bundleID {
			transition = runtimeTransitionFromObservation(
				runtimeTaskTransitionKey(task, "abort:"+reason),
				state,
				reason,
				*observation,
				occurredAt,
				nil,
				task,
			)
		} else {
			transition = runtimeTransitionWithoutObservation(
				runtimeTaskTransitionKey(task, "abort:"+reason),
				state,
				reason,
				bundleID,
				occurredAt,
				nil,
				task,
			)
		}
		commit.Transitions = append(commit.Transitions, transition)
		return &commit, nil
	}
	transition := runtimeTransitionWithoutObservation(
		runtimeTaskTransitionKey(task, "abort:"+reason),
		state,
		reason,
		bundleID,
		occurredAt,
		nil,
		task,
	)
	commit.Transitions = append(commit.Transitions, transition)
	return &commit, nil
}

func (services *runtimeServices) captureRuntimeObservation(ctx context.Context) (*store.RuntimeObservation, error) {
	observation, err := services.database.RuntimeObservation(ctx)
	if errors.Is(err, store.ErrRuntimeObservationNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &observation, nil
}

func (services *runtimeServices) clearCapturedObservationAfterFailedStop(
	observation *store.RuntimeObservation,
) error {
	if observation == nil {
		return nil
	}
	if err := services.proveCapturedObservationExited(observation); err != nil {
		return err
	}
	uncertainSince := observation.ObservedAt
	transition := runtimeTransitionFromObservation(
		runtimeIncarnationTransitionKey(*observation, store.RuntimeTransitionUnknown, "termination_result_uncertain"),
		store.RuntimeTransitionUnknown,
		"termination_result_uncertain",
		*observation,
		uncertainSince,
		&uncertainSince,
		store.Task{},
	)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, err := services.database.ClearRuntimeObservationAndTransitions(
		ctx,
		observation.PID,
		observation.ProcessStartToken,
		[]store.RuntimeTransitionInput{transition},
	)
	return err
}

func (services *runtimeServices) runtimeCommitAfterFailedTaskStop(
	observation *store.RuntimeObservation,
	task store.Task,
) (*store.RuntimeTaskCommit, error) {
	if observation == nil {
		return nil, errors.Join(
			errRuntimeTaskEvidenceUnavailable,
			errors.New("failed runtime stop has no captured observation fence"),
		)
	}
	exited, err := services.capturedObservationExited(observation)
	if err != nil {
		return nil, errors.Join(errRuntimeTaskEvidenceUnavailable, err)
	}
	if !exited {
		// The exact captured incarnation is still present, so the failed Stop did
		// not change the durable runtime fact and needs no completion evidence.
		return nil, nil
	}
	uncertainSince := observation.ObservedAt
	transition := runtimeTransitionFromObservation(
		runtimeTaskTransitionKey(task, "termination-result-uncertain"),
		store.RuntimeTransitionUnknown,
		"termination_result_uncertain",
		*observation,
		uncertainSince,
		&uncertainSince,
		task,
	)
	commit := &store.RuntimeTaskCommit{
		ExpectedObservation: observation,
		ClearObservation:    true,
		Transitions:         []store.RuntimeTransitionInput{transition},
	}
	return commit, nil
}

func (services *runtimeServices) proveCapturedObservationExited(observation *store.RuntimeObservation) error {
	exited, err := services.capturedObservationExited(observation)
	if err != nil {
		return err
	}
	if exited {
		return nil
	}
	return fmt.Errorf("runtime process %d still has the captured process start token", observation.PID)
}

func (services *runtimeServices) capturedObservationExited(
	observation *store.RuntimeObservation,
) (bool, error) {
	if observation == nil {
		return true, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	startToken, err := services.identity.ProcessStartToken(ctx, observation.PID)
	if errors.Is(err, os.ErrNotExist) {
		return true, nil
	}
	if err != nil {
		return false, fmt.Errorf("prove stopped runtime process incarnation: %w", err)
	}
	return startToken != observation.ProcessStartToken, nil
}

func startTrafficSampler(ctx context.Context, services *runtimeServices) <-chan struct{} {
	done := make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				services.collectTrafficSample(ctx)
			}
		}
	}()
	return done
}

func (services *runtimeServices) collectTrafficSample(ctx context.Context) {
	observation, err := services.database.RuntimeObservation(ctx)
	if err != nil {
		return
	}
	bundle, err := services.database.GetActivationBundle(ctx, observation.ActivationBundleID)
	if err != nil || bundle.MonitoringTier != store.MonitoringLimited {
		return
	}
	sampleContext, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	result, err := services.commands.CollectLimitedTrafficSample(sampleContext, observation)
	if err != nil {
		recordOperationalLog(services.commands, application.LogRecordRequest{
			Source: store.LogSourceCore, Level: store.LogLevelWarn, Code: "traffic.sample_failed",
			Message: "Clash API traffic sample failed", Metadata: json.RawMessage(`{}`),
		})
		return
	}
	if !result.Sample.Accepted {
		recordOperationalLog(services.commands, application.LogRecordRequest{
			Source: store.LogSourceCore, Level: store.LogLevelWarn, Code: "traffic.counter_decreased",
			Message:  "Clash API counters decreased within one process; sample rejected",
			Metadata: json.RawMessage(`{}`),
		})
	}
}
