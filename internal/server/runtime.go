// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/rehuony/sing-box-panel/internal/application"
	"github.com/rehuony/sing-box-panel/internal/corelogs"
	coreruntime "github.com/rehuony/sing-box-panel/internal/runtime"
	"github.com/rehuony/sing-box-panel/internal/settings"
	"github.com/rehuony/sing-box-panel/internal/store"
)

type runtimeServices struct {
	mu              sync.Mutex
	lifetime        context.Context
	syncEnabledCore func(context.Context) error
	database        *store.Store
	commands        *application.Application
	manager         runtimeManager
	identity        runtimeIdentityResolver
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
				store.RuntimeIntent{},
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
			store.RuntimeIntent{},
		)
		stopped := runtimeTransitionFromObservation(
			runtimeIncarnationTransitionKey(*expectedObservation, store.RuntimeTransitionStopped, "startup_reconciled_stopped"),
			store.RuntimeTransitionStopped,
			"startup_reconciled_stopped",
			*expectedObservation,
			runtimeEventTime(reconciledAt, uncertainSince),
			nil,
			store.RuntimeIntent{},
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
			store.RuntimeIntent{Generation: bootstrap.Hub.TargetGeneration},
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
	if recovery.Intent != nil {
		recordRuntimeRecoveryAttempt(services.commands, recovery)
		return services.executeIntent(ctx, *recovery.Intent)
	}
	if !recovery.Exhausted && recovery.EpisodeID != "" {
		recordRuntimeRecoveryScheduled(services.commands, recovery)
	}
	if recovery.Exhausted {
		recordRuntimeRecoveryExhausted(services.commands, recovery)
	}
	return nil
}

func (services *runtimeServices) Close() error {
	if services != nil {
		services.mu.Lock()
		defer services.mu.Unlock()
	}
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
				store.RuntimeIntent{},
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

func (services *runtimeServices) performRuntimeIntent(ctx context.Context, intent store.RuntimeIntent, control runtimeExecutionGuard) (runtimeResult, error) {
	if err := control.SafePoint(ctx); err != nil {
		return runtimeResult{}, err
	}
	if store.RuntimeIntentKind(intent.Kind) == store.RuntimeIntentStop {
		return services.stopForIntent(ctx, intent, control)
	}
	material, err := services.commands.LoadRuntimeMaterial(ctx, intent.ActivationBundleID)
	if err != nil {
		return runtimeResult{}, err
	}
	processMonitoringTier := store.MonitoringTier(services.manager.MonitoringLevel())
	if processMonitoringTier != store.MonitoringProcessOnly ||
		(material.Activation.MonitoringTier != store.MonitoringProcessOnly &&
			material.Activation.MonitoringTier != store.MonitoringLimited) {
		return runtimeResult{}, fmt.Errorf(
			"activation monitoring tier %q is unavailable; process probe supplies %q",
			material.Activation.MonitoringTier,
			processMonitoringTier,
		)
	}
	if err := services.revalidateRuntimeMaterial(ctx, material); err != nil {
		return runtimeResult{}, err
	}
	if store.CoreSelectionOnly(intent) {
		return services.selectStoppedCore(ctx, intent, control)
	}
	capturedObservation, err := services.captureRuntimeObservation(ctx)
	if err != nil {
		return runtimeResult{}, err
	}
	var recordedObservation *store.RuntimeObservation
	live := services.manager.ObserveLiveIdentity()
	alreadyExact := live.Running && live.BundleID == material.Bundle.ID &&
		live.ArtifactID == material.Bundle.ArtifactID && live.ExactVersion == material.Bundle.ExactVersion
	startedByIntent := false
	if runtimeIntentNeedsTransition(store.RuntimeIntentKind(intent.Kind), alreadyExact) {
		if err := control.SafePoint(ctx); err != nil {
			return runtimeResult{}, err
		}
		if live.Running {
			err = services.manager.Restart(ctx, material.Bundle)
		} else {
			err = services.manager.Start(ctx, material.Bundle)
		}
		if err != nil {
			commit, evidenceErr := services.runtimeIntentFailureCommit(intent, material, capturedObservation)
			return runtimeResult{Runtime: commit}, errors.Join(err, evidenceErr)
		}
		startedByIntent = true
		observation, recordErr := services.recordLiveObservation(
			ctx, material, capturedObservation,
		)
		if recordErr != nil {
			commit, stopErr := services.stopAfterLostIntent(
				capturedObservation,
				intent,
				material.Activation.ID,
				store.RuntimeTransitionFailed,
				"observation_record_failed",
			)
			return runtimeResult{Runtime: commit}, errors.Join(recordErr, stopErr)
		}
		recordedObservation = &observation
		if err := services.revalidateRuntimeMaterial(ctx, material); err != nil {
			commit, stopErr := services.stopAfterLostIntent(
				recordedObservation,
				intent,
				material.Activation.ID,
				store.RuntimeTransitionStopped,
				"intent_superseded",
			)
			return runtimeResult{Runtime: commit}, errors.Join(err, stopErr)
		}
	}
	if material.Activation.MonitoringTier == store.MonitoringLimited {
		if err := services.awaitClashAPI(ctx, material); err != nil {
			if startedByIntent {
				commit, stopErr := services.stopAfterLostIntent(
					recordedObservation,
					intent,
					material.Activation.ID,
					store.RuntimeTransitionFailed,
					"monitoring_handshake_failed",
				)
				return runtimeResult{Runtime: commit}, errors.Join(err, stopErr)
			}
			return runtimeResult{}, err
		}
	}
	if err := control.SafePoint(ctx); err != nil {
		if startedByIntent {
			commit, stopErr := services.stopAfterLostIntent(
				recordedObservation,
				intent,
				material.Activation.ID,
				store.RuntimeTransitionStopped,
				"intent_superseded",
			)
			return runtimeResult{Runtime: commit}, errors.Join(err, stopErr)
		}
		return runtimeResult{}, err
	}
	if recordedObservation == nil {
		observation, recordErr := services.recordLiveObservation(
			ctx, material, capturedObservation,
		)
		if recordErr != nil {
			if startedByIntent {
				commit, stopErr := services.stopAfterLostIntent(
					recordedObservation,
					intent,
					material.Activation.ID,
					store.RuntimeTransitionFailed,
					"observation_record_failed",
				)
				return runtimeResult{Runtime: commit}, errors.Join(recordErr, stopErr)
			}
			return runtimeResult{}, recordErr
		}
		recordedObservation = &observation
	}
	if err := control.SafePoint(ctx); err != nil {
		if startedByIntent {
			commit, stopErr := services.stopAfterLostIntent(
				recordedObservation,
				intent,
				material.Activation.ID,
				store.RuntimeTransitionStopped,
				"intent_superseded",
			)
			return runtimeResult{Runtime: commit}, errors.Join(err, stopErr)
		}
		return runtimeResult{}, err
	}
	_, runtimeCommit, transitionErr := services.prepareRuntimeIntentRunningCommit(
		ctx,
		intent,
		*recordedObservation,
		capturedObservation,
	)
	if transitionErr != nil {
		if startedByIntent {
			commit, stopErr := services.stopAfterLostIntent(
				recordedObservation,
				intent,
				material.Activation.ID,
				store.RuntimeTransitionFailed,
				"observation_transition_failed",
			)
			return runtimeResult{Runtime: commit}, errors.Join(transitionErr, stopErr)
		}
		return runtimeResult{}, transitionErr
	}
	return runtimeResult{Runtime: &runtimeCommit}, nil
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
		current.Bundle.StartupConfigDigest != material.Bundle.StartupConfigDigest {
		return store.ErrActivationBundleNotReady
	}
	return nil
}

func (services *runtimeServices) stopForIntent(
	ctx context.Context,
	intent store.RuntimeIntent,
	control runtimeExecutionGuard,
) (runtimeResult, error) {
	observation, err := services.captureRuntimeObservation(ctx)
	if err != nil {
		return runtimeResult{}, err
	}
	// Fence the intent immediately before changing the external process. The
	// second fence below detects cancellation or supersession that races with a
	// successful Stop, but the resulting runtime evidence must still be returned.
	if err := control.SafePoint(ctx); err != nil {
		return runtimeResult{}, err
	}
	if err := services.manager.Stop(ctx); err != nil {
		commit, evidenceErr := services.runtimeCommitAfterFailedIntentStop(observation, intent)
		return runtimeResult{Runtime: commit}, errors.Join(err, evidenceErr)
	}
	live := services.manager.ObserveLiveIdentity()
	occurredAt := runtimeTransitionTime(live, time.Now())
	commit := store.RuntimeCommit{
		ExpectedObservation: observation,
		ClearObservation:    true,
		Transitions:         make([]store.RuntimeTransitionInput, 0, 1),
	}
	if observation != nil {
		occurredAt = runtimeEventTime(occurredAt, observation.ObservedAt)
		transition := runtimeTransitionFromObservation(
			runtimeIntentTransitionKey(intent, "stopped"),
			store.RuntimeTransitionStopped,
			"stop_succeeded",
			*observation,
			occurredAt,
			nil,
			intent,
		)
		commit.Transitions = append(commit.Transitions, transition)
	} else {
		transition := runtimeTransitionWithoutObservation(
			runtimeIntentTransitionKey(intent, "stopped"),
			store.RuntimeTransitionStopped,
			"stop_succeeded",
			intent.ActivationBundleID,
			occurredAt,
			nil,
			intent,
		)
		commit.Transitions = append(commit.Transitions, transition)
	}
	if err := control.SafePoint(ctx); err != nil {
		return runtimeResult{Runtime: &commit}, err
	}
	return runtimeResult{Runtime: &commit}, nil
}

func (services *runtimeServices) prepareRuntimeIntentRunningCommit(
	ctx context.Context,
	intent store.RuntimeIntent,
	observation store.RuntimeObservation,
	previous *store.RuntimeObservation,
) (store.RuntimeObservation, store.RuntimeCommit, error) {
	live := services.manager.ObserveLiveIdentity()
	if !live.Running || live.PID != observation.PID || live.BundleID != observation.ActivationBundleID ||
		!live.StartedAt.Equal(observation.StartedAt) {
		return store.RuntimeObservation{}, store.RuntimeCommit{}, coreruntime.ErrNotRunning
	}
	startToken, err := services.identity.ProcessStartToken(ctx, live.PID)
	if err != nil {
		return store.RuntimeObservation{}, store.RuntimeCommit{}, err
	}
	if startToken != observation.ProcessStartToken {
		return store.RuntimeObservation{}, store.RuntimeCommit{}, store.ErrRuntimeIdentityMismatch
	}
	expected := observation
	observation.ObservedAt = runtimeEventTime(time.Now(), observation.ObservedAt)
	transitions := make([]store.RuntimeTransitionInput, 0, 2)
	if previous != nil && !sameRuntimeIncarnation(*previous, observation) {
		boundaryAt := runtimeEventTime(live.StartedAt, previous.ObservedAt)
		transitions = append(transitions, runtimeTransitionFromObservation(
			runtimeIntentTransitionKey(intent, "boundary"),
			store.RuntimeTransitionStopped,
			runtimeIntentReason(intent, "boundary"),
			*previous,
			boundaryAt,
			nil,
			intent,
		))
	}
	transitionAt := runtimeEventTime(runtimeTransitionTime(live, observation.ObservedAt), live.StartedAt)
	transitions = append(transitions, runtimeTransitionFromObservation(
		runtimeIntentTransitionKey(intent, "running"),
		store.RuntimeTransitionRunning,
		runtimeIntentReason(intent, "succeeded"),
		observation,
		transitionAt,
		nil,
		intent,
	))
	return observation, store.RuntimeCommit{
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
	intent store.RuntimeIntent,
	bundleID string,
	state store.RuntimeTransitionState,
	reason string,
) (*store.RuntimeCommit, error) {
	stopCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	if err := services.manager.Stop(stopCtx); err != nil {
		commit, evidenceErr := services.runtimeCommitAfterFailedIntentStop(observation, intent)
		return commit, errors.Join(err, evidenceErr)
	}
	live := services.manager.ObserveLiveIdentity()
	occurredAt := runtimeTransitionTime(live, time.Now())
	commit := store.RuntimeCommit{
		ExpectedObservation: observation,
		ClearObservation:    true,
		Transitions:         make([]store.RuntimeTransitionInput, 0, 1),
	}
	if observation != nil {
		occurredAt = runtimeEventTime(occurredAt, observation.ObservedAt)
		var transition store.RuntimeTransitionInput
		if observation.ActivationBundleID == bundleID {
			transition = runtimeTransitionFromObservation(
				runtimeIntentTransitionKey(intent, "abort:"+reason),
				state,
				reason,
				*observation,
				occurredAt,
				nil,
				intent,
			)
		} else {
			transition = runtimeTransitionWithoutObservation(
				runtimeIntentTransitionKey(intent, "abort:"+reason),
				state,
				reason,
				bundleID,
				occurredAt,
				nil,
				intent,
			)
		}
		commit.Transitions = append(commit.Transitions, transition)
		return &commit, nil
	}
	transition := runtimeTransitionWithoutObservation(
		runtimeIntentTransitionKey(intent, "abort:"+reason),
		state,
		reason,
		bundleID,
		occurredAt,
		nil,
		intent,
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
		store.RuntimeIntent{},
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

func (services *runtimeServices) runtimeCommitAfterFailedIntentStop(
	observation *store.RuntimeObservation,
	intent store.RuntimeIntent,
) (*store.RuntimeCommit, error) {
	if observation == nil {
		return nil, errors.Join(
			errRuntimeEvidenceUnavailable,
			errors.New("failed runtime stop has no captured observation fence"),
		)
	}
	exited, err := services.capturedObservationExited(observation)
	if err != nil {
		return nil, errors.Join(errRuntimeEvidenceUnavailable, err)
	}
	if !exited {
		// The exact captured incarnation is still present, so the failed Stop did
		// not change the durable runtime fact and needs no completion evidence.
		return nil, nil
	}
	uncertainSince := observation.ObservedAt
	transition := runtimeTransitionFromObservation(
		runtimeIntentTransitionKey(intent, "termination-result-uncertain"),
		store.RuntimeTransitionUnknown,
		"termination_result_uncertain",
		*observation,
		uncertainSince,
		&uncertainSince,
		intent,
	)
	commit := &store.RuntimeCommit{
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
