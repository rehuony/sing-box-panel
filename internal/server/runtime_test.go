// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/rehuony/sing-box-panel/internal/testutil"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/rehuony/sing-box-panel/internal/application"
	"github.com/rehuony/sing-box-panel/internal/configuration"
	coreruntime "github.com/rehuony/sing-box-panel/internal/runtime"
	"github.com/rehuony/sing-box-panel/internal/store"
)

func TestReconcileStartupClearsOnlyProvenStaleObservation(t *testing.T) {
	tests := []struct {
		name          string
		startToken    string
		startTokenErr error
		wantErr       error
		wantCleared   bool
	}{
		{
			name:          "process disappeared",
			startTokenErr: os.ErrNotExist,
			wantCleared:   true,
		},
		{name: "PID was reused", startToken: "replacement-incarnation", wantCleared: true},
		{
			name:          "inspection unavailable",
			startTokenErr: application.ErrInspectionUnavailable,
			wantErr:       application.ErrInspectionUnavailable,
		},
		{
			name:          "unknown inspection failure",
			startTokenErr: errors.New("identity inspection failed"),
		},
		{
			name:       "same PID incarnation remains live",
			startToken: "process-start-runtime-test",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			database, commands, observation := seedRuntimeObservation(t, ctx)
			resolver := &fakeRuntimeIdentityResolver{startToken: test.startToken, startTokenErr: test.startTokenErr}
			services := &runtimeServices{
				database: database, commands: commands, manager: &fakeRuntimeManager{}, identity: resolver,
			}
			err := services.ReconcileStartup(ctx)
			switch {
			case test.wantCleared:
				if err != nil {
					t.Fatalf("ReconcileStartup() error = %v", err)
				}
				if _, err := database.RuntimeObservation(ctx); !errors.Is(err, store.ErrRuntimeObservationNotFound) {
					t.Fatalf("stale observation was not cleared: %v", err)
				}
			case test.wantErr != nil:
				if !errors.Is(err, test.wantErr) {
					t.Fatalf("ReconcileStartup() error = %v, want %v", err, test.wantErr)
				}
			default:
				if err == nil {
					t.Fatal("ReconcileStartup() unexpectedly succeeded")
				}
			}
			if !test.wantCleared {
				stored, observationErr := database.RuntimeObservation(ctx)
				if observationErr != nil || stored.PID != observation.PID || stored.ProcessStartToken != observation.ProcessStartToken {
					t.Fatalf("ambiguous observation was changed: observation=%+v err=%v", stored, observationErr)
				}
			}
			if resolver.startTokenCalls != 1 {
				t.Fatalf("ProcessStartToken() calls = %d, want 1", resolver.startTokenCalls)
			}
		})
	}
}

func TestReconcileStartupFailsClosedOnObservationReadError(t *testing.T) {
	ctx := context.Background()
	database, commands, _ := seedRuntimeObservation(t, ctx)
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}
	resolver := &fakeRuntimeIdentityResolver{startTokenErr: os.ErrNotExist}
	services := &runtimeServices{
		database: database, commands: commands, manager: &fakeRuntimeManager{}, identity: resolver,
	}
	if err := services.ReconcileStartup(ctx); err == nil {
		t.Fatal("ReconcileStartup() succeeded after the database was closed")
	}
	if resolver.startTokenCalls != 0 {
		t.Fatalf("ProcessStartToken() calls = %d, want 0", resolver.startTokenCalls)
	}
}

func TestReconcileStartupEstablishesKnownStoppedBoundaryAfterHistoryInitialization(t *testing.T) {
	ctx := context.Background()
	database, err := store.Open(ctx, filepath.Join(t.TempDir(), "panel.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	commands := application.FromStore(database)
	services := &runtimeServices{
		database: database,
		commands: commands,
		manager:  &fakeRuntimeManager{},
		identity: &fakeRuntimeIdentityResolver{},
	}
	if err := services.ReconcileStartup(ctx); err != nil {
		t.Fatal(err)
	}
	history, err := database.ListRuntimeTransitions(ctx, store.RuntimeHistoryFilter{Limit: 10})
	if err != nil || len(history.Items) != 2 ||
		history.Items[0].State != store.RuntimeTransitionStopped ||
		history.Items[0].Reason != "startup_reconciled_stopped" ||
		history.Items[1].Reason != "history_initialized" {
		t.Fatalf("startup runtime history = %+v, %v", history, err)
	}
}

func TestReconcileStartupClearsExitedObservationWhenDesiredStopped(t *testing.T) {
	ctx := context.Background()
	database, commands, _ := seedRuntimeObservation(t, ctx)
	if _, err := commands.PrepareRuntimeIntent(ctx, store.RuntimeIntentStop, ""); err != nil {
		t.Fatal(err)
	}
	services := &runtimeServices{
		database: database, commands: commands, manager: &fakeRuntimeManager{},
		identity: &fakeRuntimeIdentityResolver{startTokenErr: os.ErrNotExist},
	}
	if err := services.ReconcileStartup(ctx); err != nil {
		t.Fatalf("ReconcileStartup() error = %v", err)
	}
	if observation, err := database.RuntimeObservation(ctx); !errors.Is(err, store.ErrRuntimeObservationNotFound) {
		t.Fatalf("exited observation with desired stopped = %+v, %v", observation, err)
	}
}

func TestStopForIntentFailsBeforeStoppingProcessOnObservationReadError(t *testing.T) {
	ctx := context.Background()
	database, commands, _ := seedRuntimeObservation(t, ctx)
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}
	manager := &fakeRuntimeManager{}
	services := &runtimeServices{database: database, commands: commands, manager: manager}
	if _, err := services.stopForIntent(ctx, store.RuntimeIntent{}, successfulRuntimeGuard{}); err == nil {
		t.Fatal("stopForIntent() succeeded after observation read failed")
	}
	if manager.stopCalls != 0 {
		t.Fatalf("manager.Stop() calls = %d, want 0", manager.stopCalls)
	}
}

func TestStopForIntentClearsOnlyCapturedIncarnationProvenExited(t *testing.T) {
	tests := []struct {
		name            string
		startToken      string
		startTokenErr   error
		wantCleared     bool
		wantRecoverable bool
	}{
		{name: "process disappeared", startTokenErr: os.ErrNotExist, wantCleared: true},
		{name: "PID was reused", startToken: "replacement-incarnation", wantCleared: true},
		{name: "same incarnation is retained", startToken: "process-start-runtime-test"},
		{
			name: "inspection failure is retained", startTokenErr: errors.New("permission denied"),
			wantRecoverable: true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			database, commands, observation := seedRuntimeObservation(t, ctx)
			queued, err := commands.PrepareRuntimeIntent(ctx, store.RuntimeIntentStop, "")
			if err != nil {
				t.Fatal(err)
			}
			intent := queued
			manager := &fakeRuntimeManager{stopErr: errors.New("termination failed")}
			resolver := &fakeRuntimeIdentityResolver{startToken: test.startToken, startTokenErr: test.startTokenErr}
			services := &runtimeServices{database: database, commands: commands, manager: manager, identity: resolver}
			result, handlerErr := services.stopForIntent(ctx, intent, successfulRuntimeGuard{})
			if handlerErr == nil {
				t.Fatal("stopForIntent() succeeded after termination failed")
			}
			stored, err := database.RuntimeObservation(ctx)
			if err != nil || stored.PID != observation.PID || stored.ProcessStartToken != observation.ProcessStartToken {
				t.Fatalf("observation changed before terminal completion: %+v, %v", stored, err)
			}
			if test.wantRecoverable {
				if !errors.Is(handlerErr, errRuntimeEvidenceUnavailable) || result.Runtime != nil {
					t.Fatalf("uncertain failed stop = %v; commit = %+v", handlerErr, result.Runtime)
				}
				return
			}
			if err := database.CompleteRuntimeIntent(ctx, intent, false, result.Runtime, time.Now().UTC()); err != nil {
				t.Fatalf("CompleteTask(failed stop) error = %v", err)
			}
			stored, err = database.RuntimeObservation(ctx)
			if test.wantCleared {
				if !errors.Is(err, store.ErrRuntimeObservationNotFound) {
					t.Fatalf("proven exited observation was retained: %+v, %v", stored, err)
				}
				history, historyErr := database.ListRuntimeTransitions(ctx, store.RuntimeHistoryFilter{
					Reason: "termination_result_uncertain", Limit: 10,
				})
				if historyErr != nil || len(history.Items) != 1 || history.Items[0].Generation != intent.Generation {
					t.Fatalf("failed-stop runtime history = %+v, %v", history, historyErr)
				}
			} else if err != nil || stored.PID != observation.PID || stored.ProcessStartToken != observation.ProcessStartToken {
				t.Fatalf("uncertain observation was changed: %+v, %v", stored, err)
			}
		})
	}
}

func TestRuntimeCloseCannotClearNewerObservation(t *testing.T) {
	ctx := context.Background()
	database, commands, oldObservation := seedRuntimeObservation(t, ctx)
	newObservation := oldObservation
	newObservation.PID = 5252
	newObservation.ProcessStartToken = "new-process-incarnation"
	newObservation.StartedAt = oldObservation.StartedAt.Add(time.Minute)
	newObservation.ObservedAt = newObservation.StartedAt.Add(time.Second)
	manager := &fakeRuntimeManager{closeHook: func() {
		if _, err := database.RecordRuntimeObservation(ctx, newObservation); err != nil {
			t.Errorf("RecordRuntimeObservation(new incarnation) error = %v", err)
		}
	}}
	services := &runtimeServices{database: database, commands: commands, manager: manager, identity: &fakeRuntimeIdentityResolver{}}
	if err := services.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	stored, err := database.RuntimeObservation(ctx)
	if err != nil || stored.PID != newObservation.PID || stored.ProcessStartToken != newObservation.ProcessStartToken {
		t.Fatalf("newer observation after Close = %+v, %v", stored, err)
	}
}

func TestRuntimeCloseFailureWithSuccessfulWaitRetainsSameIncarnation(t *testing.T) {
	ctx := context.Background()
	database, commands, observation := seedRuntimeObservation(t, ctx)
	services := &runtimeServices{
		database: database, commands: commands,
		manager:  &fakeRuntimeManager{closeErr: errors.New("termination timed out")},
		identity: &fakeRuntimeIdentityResolver{startToken: observation.ProcessStartToken},
	}
	if err := services.Close(); err == nil {
		t.Fatal("Close() succeeded after termination failed")
	}
	stored, err := database.RuntimeObservation(ctx)
	if err != nil || stored.PID != observation.PID || stored.ProcessStartToken != observation.ProcessStartToken {
		t.Fatalf("observation after uncertain Close = %+v, %v", stored, err)
	}
}

func TestRuntimeReconcilerDoesNotTreatIdentityMismatchAsProcessExit(t *testing.T) {
	ctx := context.Background()
	database, commands, observation := seedRuntimeObservation(t, ctx)
	resolver := &fakeRuntimeIdentityResolver{
		err: application.ErrStaleObservation, startToken: observation.ProcessStartToken,
	}
	services := &runtimeServices{
		database: database, commands: commands,
		manager:  &fakeRuntimeManager{live: coreruntime.LiveIdentity{State: coreruntime.StateFailed}},
		identity: resolver,
	}
	if _, err := services.reconcileFailedRuntime(ctx); err == nil {
		t.Fatal("reconcileFailedRuntime() accepted a still-live PID incarnation")
	}
	stored, err := database.RuntimeObservation(ctx)
	if err != nil || stored.PID != observation.PID || stored.ProcessStartToken != observation.ProcessStartToken {
		t.Fatalf("still-live observation was changed: %+v, %v", stored, err)
	}
	bootstrap, err := database.Bootstrap(ctx)
	if err != nil || bootstrap.Hub.TargetGeneration != 1 {
		t.Fatalf("hub after refused recovery = %+v, %v", bootstrap.Hub, err)
	}
	if resolver.resolveCalls != 0 {
		t.Fatalf("Resolve() calls = %d, want start-token proof only", resolver.resolveCalls)
	}
}

func TestRuntimeReconcilerPersistsStableRunOnlyAfterContinuousIdentityProof(t *testing.T) {
	ctx := context.Background()
	database, commands, observation := seedRuntimeObservation(t, ctx)
	clock := newFakeClock(observation.ObservedAt)
	manager := &fakeRuntimeManager{live: coreruntime.LiveIdentity{
		Running: true, State: coreruntime.StateRunning, PID: observation.PID,
		BundleID: observation.ActivationBundleID, StartedAt: observation.StartedAt,
	}}
	resolver := &fakeRuntimeIdentityResolver{startToken: observation.ProcessStartToken}
	reconciler := &runtimeReconciler{
		services: &runtimeServices{database: database, commands: commands, manager: manager, identity: resolver},
		clock:    clock,
	}

	reconciler.reconcile(ctx)
	clock.Advance(store.RuntimeRecoveryStableWindow - time.Second)
	reconciler.reconcile(ctx)
	beforeStable, err := database.RuntimeObservation(ctx)
	if err != nil || runtimeObservationProvesStableRun(&beforeStable) ||
		!beforeStable.ObservedAt.Equal(clock.Now()) {
		t.Fatalf("observation before stable window = %+v, %v", beforeStable, err)
	}
	clock.Advance(time.Second)
	reconciler.reconcile(ctx)
	stable, err := database.RuntimeObservation(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !runtimeObservationProvesStableRun(&stable) || !stable.ObservedAt.Equal(clock.Now()) ||
		stable.StableObservedAt == nil || !stable.StableObservedAt.Equal(clock.Now()) {
		t.Fatalf("stable runtime observation = %+v; clock = %v", stable, clock.Now())
	}
	if resolver.startTokenCalls != 3 {
		t.Fatalf("ProcessStartToken() calls = %d, want one per reconciliation", resolver.startTokenCalls)
	}
}

func TestRuntimeReconcilerLogsUnexpectedExitAndRecoveryScheduleOnce(t *testing.T) {
	ctx := context.Background()
	database, commands, observation := seedRuntimeObservation(t, ctx)
	manager := &fakeRuntimeManager{live: coreruntime.LiveIdentity{
		State: coreruntime.StateFailed, BundleID: observation.ActivationBundleID,
		StartedAt: observation.StartedAt,
	}}
	reconciler := &runtimeReconciler{
		services: &runtimeServices{
			database: database, commands: commands, manager: manager,
			identity: &fakeRuntimeIdentityResolver{startTokenErr: os.ErrNotExist},
		},
		clock: newFakeClock(observation.ObservedAt),
	}
	reconciler.reconcile(ctx)
	reconciler.reconcile(ctx)

	assertSingleRuntimeLog(t, ctx, database, "runtime.unexpected_exit", observation.ActivationBundleID, 1, 1)
	assertSingleRuntimeLog(t, ctx, database, "runtime.recovery_scheduled", observation.ActivationBundleID, 1, 1)
	history, err := database.ListRuntimeTransitions(ctx, store.RuntimeHistoryFilter{
		Reason: "unexpected_exit",
		Limit:  10,
	})
	if err != nil || len(history.Items) != 1 || history.Items[0].State != store.RuntimeTransitionFailed ||
		history.Items[0].PID != observation.PID {
		t.Fatalf("unexpected-exit runtime history = %+v, %v", history, err)
	}
}

func TestRuntimeReconcilerRecordsFailureWhenRecoveryIsNoLongerDesired(t *testing.T) {
	ctx := context.Background()
	database, commands, observation := seedRuntimeObservation(t, ctx)
	if _, err := commands.PrepareRuntimeIntent(ctx, store.RuntimeIntentStop, ""); err != nil {
		t.Fatal(err)
	}
	services := &runtimeServices{
		database: database,
		commands: commands,
		manager: &fakeRuntimeManager{live: coreruntime.LiveIdentity{
			State: coreruntime.StateFailed, BundleID: observation.ActivationBundleID,
			StartedAt: observation.StartedAt, TransitionedAt: observation.ObservedAt.Add(time.Second),
			Failure: &coreruntime.FailureStatus{
				Operation: "process", Code: "unexpected_exit", FailedAt: observation.ObservedAt.Add(time.Second),
			},
		}},
		identity: &fakeRuntimeIdentityResolver{startTokenErr: os.ErrNotExist},
	}
	result, err := services.reconcileFailedRuntime(ctx)
	if err != nil || result != (application.RuntimeRecoveryResult{}) {
		t.Fatalf("reconcileFailedRuntime() = %+v, %v", result, err)
	}
	if _, err := database.RuntimeObservation(ctx); !errors.Is(err, store.ErrRuntimeObservationNotFound) {
		t.Fatalf("runtime observation after non-recoverable failure = %v", err)
	}
	history, err := database.ListRuntimeTransitions(ctx, store.RuntimeHistoryFilter{
		Reason: "unexpected_exit",
		Limit:  10,
	})
	if err != nil || len(history.Items) != 1 || history.Items[0].Generation != 0 {
		t.Fatalf("non-recoverable failure history = %+v, %v", history, err)
	}
}

func TestStopForIntentClearsObservationAndAppendsStoppedTransition(t *testing.T) {
	ctx := context.Background()
	database, commands, observation := seedRuntimeObservation(t, ctx)
	queued, err := commands.PrepareRuntimeIntent(ctx, store.RuntimeIntentStop, "")
	if err != nil {
		t.Fatal(err)
	}
	intent := queued
	manager := &fakeRuntimeManager{live: coreruntime.LiveIdentity{
		State:          coreruntime.StateStopped,
		TransitionedAt: observation.ObservedAt.Add(time.Second),
	}}
	services := &runtimeServices{database: database, commands: commands, manager: manager}
	result, err := services.stopForIntent(ctx, intent, successfulRuntimeGuard{})
	if err != nil {
		t.Fatalf("stopForIntent() error = %v", err)
	}
	if stored, err := database.RuntimeObservation(ctx); err != nil || stored.PID != observation.PID {
		t.Fatalf("observation changed before terminal commit = %+v, %v", stored, err)
	}
	if err := database.CompleteRuntimeIntent(ctx, intent, true, result.Runtime, time.Now().UTC()); err != nil {
		t.Fatalf("CompleteTask(runtime stop) error = %v", err)
	}
	if _, err := database.RuntimeObservation(ctx); !errors.Is(err, store.ErrRuntimeObservationNotFound) {
		t.Fatalf("runtime observation after controlled stop = %v", err)
	}
	history, err := database.ListRuntimeTransitions(ctx, store.RuntimeHistoryFilter{
		Reason: "stop_succeeded",
		Limit:  10,
	})
	if err != nil || len(history.Items) != 1 || history.Items[0].State != store.RuntimeTransitionStopped ||
		history.Items[0].Generation != intent.Generation || history.Items[0].PID != observation.PID {
		t.Fatalf("controlled-stop runtime history = %+v, %v", history, err)
	}
}

func TestStopForIntentReturnsEvidenceBeforePostStopSupersession(t *testing.T) {
	ctx := context.Background()
	database, commands, observation := seedRuntimeObservation(t, ctx)
	queued, err := commands.PrepareRuntimeIntent(ctx, store.RuntimeIntentStop, "")
	if err != nil {
		t.Fatal(err)
	}
	intent := queued
	manager := &fakeRuntimeManager{live: coreruntime.LiveIdentity{
		State: coreruntime.StateStopped, TransitionedAt: observation.ObservedAt.Add(time.Second),
	}}
	services := &runtimeServices{database: database, commands: commands, manager: manager}
	safePoints := 0
	control := runtimeGuardFunc(func(checkCtx context.Context) error {
		safePoints++
		if safePoints == 1 {
			return nil
		}
		if _, queueErr := commands.PrepareRuntimeIntent(checkCtx, store.RuntimeIntentStart, ""); queueErr != nil {
			return queueErr
		}
		return store.ErrRuntimeIntentStale
	})
	result, err := services.stopForIntent(ctx, intent, control)
	if !errors.Is(err, store.ErrRuntimeIntentStale) {
		t.Fatalf("stopForIntent() error = %v, want superseded", err)
	}
	if manager.stopCalls != 1 || safePoints != 2 || result.Runtime == nil {
		t.Fatalf("post-stop state: stop calls=%d safe points=%d commit=%+v", manager.stopCalls, safePoints, result.Runtime)
	}
	if stored, err := database.RuntimeObservation(ctx); err != nil || stored.PID != observation.PID {
		t.Fatalf("observation changed before superseded completion = %+v, %v", stored, err)
	}
	err = database.CompleteRuntimeIntent(ctx, intent, false, result.Runtime, time.Now().UTC())
	if !errors.Is(err, store.ErrRuntimeIntentStale) {
		t.Fatalf("stale completion accepted: %v", err)
	}
	stored, err := database.RuntimeObservation(ctx)
	if err != nil || stored.PID != observation.PID {
		t.Fatalf("stale completion changed observation: %+v %v", stored, err)
	}
	history, err := database.ListRuntimeTransitions(ctx, store.RuntimeHistoryFilter{Reason: "stop_succeeded", Limit: 10})
	if err != nil || len(history.Items) != 0 {
		t.Fatalf("stale history: %+v %v", history, err)
	}

}

func TestStopForIntentCompletionFenceFailureLeavesEvidenceIntact(t *testing.T) {
	ctx := context.Background()
	database, commands, observation := seedRuntimeObservation(t, ctx)
	queued, err := commands.PrepareRuntimeIntent(ctx, store.RuntimeIntentStop, "")
	if err != nil {
		t.Fatal(err)
	}
	intent := queued
	manager := &fakeRuntimeManager{live: coreruntime.LiveIdentity{
		State: coreruntime.StateStopped, TransitionedAt: observation.ObservedAt.Add(time.Second),
	}}
	services := &runtimeServices{database: database, commands: commands, manager: manager}
	result, err := services.stopForIntent(ctx, intent, successfulRuntimeGuard{})
	if err != nil {
		t.Fatalf("stopForIntent() error = %v", err)
	}

	newer := observation
	newer.PID++
	newer.ProcessStartToken = "newer-stop-completion-fence"
	newer.StartedAt = observation.ObservedAt.Add(2 * time.Second)
	newer.ObservedAt = newer.StartedAt.Add(time.Second)
	if _, err := database.RecordRuntimeObservation(ctx, newer); err != nil {
		t.Fatal(err)
	}
	err = database.CompleteRuntimeIntent(ctx, intent, true, result.Runtime, time.Now().UTC())
	if !errors.Is(err, store.ErrRuntimeIdentityMismatch) {
		t.Fatalf("CompleteTask(stale post-stop evidence) error = %v", err)
	}
	stored, err := database.RuntimeObservation(ctx)
	if err != nil || stored.PID != newer.PID || stored.ProcessStartToken != newer.ProcessStartToken {
		t.Fatalf("newer observation after failed post-stop completion = %+v, %v", stored, err)
	}
	history, err := database.ListRuntimeTransitions(ctx, store.RuntimeHistoryFilter{
		Reason: "stop_succeeded", Limit: 10,
	})
	if err != nil || len(history.Items) != 0 {
		t.Fatalf("history after failed post-stop completion = %+v, %v", history, err)
	}
	if manager.stopCalls != 1 {
		t.Fatalf("manager.Stop() calls = %d, want 1", manager.stopCalls)
	}
}

func TestRuntimeIntentHealthFailureClearsObservationAndAppendsFailedTransition(t *testing.T) {
	ctx := context.Background()
	database, commands, observation := seedRuntimeObservation(t, ctx)
	queued, err := commands.PrepareRuntimeIntent(ctx, store.RuntimeIntentRestart, "")
	if err != nil {
		t.Fatal(err)
	}
	intent := queued
	activation, err := database.GetActivationBundle(ctx, observation.ActivationBundleID)
	if err != nil {
		t.Fatal(err)
	}
	startup, err := database.GetStartupArtifact(ctx, activation.StartupArtifactID)
	if err != nil {
		t.Fatal(err)
	}
	core, err := database.GetCoreArtifact(ctx, startup.CoreArtifactID)
	if err != nil {
		t.Fatal(err)
	}
	failedAt := observation.ObservedAt.Add(time.Second)
	services := &runtimeServices{
		database: database,
		commands: commands,
		manager: &fakeRuntimeManager{live: coreruntime.LiveIdentity{
			State: coreruntime.StateFailed, BundleID: activation.ID,
			StartedAt: observation.StartedAt, TransitionedAt: failedAt,
			Failure: &coreruntime.FailureStatus{
				Operation: "health_check", Code: "unhealthy", FailedAt: failedAt,
			},
		}},
		identity: &fakeRuntimeIdentityResolver{startTokenErr: os.ErrNotExist},
	}
	commit, err := services.runtimeIntentFailureCommit(intent, application.RuntimeMaterial{
		Activation: activation, Startup: startup, Core: core,
	}, &observation)
	if err != nil {
		t.Fatal(err)
	}
	if stored, err := database.RuntimeObservation(ctx); err != nil || stored.PID != observation.PID {
		t.Fatalf("runtime observation changed before terminal completion = %+v, %v", stored, err)
	}
	if err := database.CompleteRuntimeIntent(ctx, intent, false, commit, time.Now().UTC()); err != nil {
		t.Fatalf("CompleteTask(health failure) error = %v", err)
	}
	if _, err := database.RuntimeObservation(ctx); !errors.Is(err, store.ErrRuntimeObservationNotFound) {
		t.Fatalf("runtime observation after atomic health failure = %v", err)
	}
	history, err := database.ListRuntimeTransitions(ctx, store.RuntimeHistoryFilter{
		Reason: "health_check_failed",
		Limit:  10,
	})
	if err != nil || len(history.Items) != 1 || history.Items[0].State != store.RuntimeTransitionFailed ||
		history.Items[0].Generation != intent.Generation {
		t.Fatalf("health-failure runtime history = %+v, %v", history, err)
	}
}

func TestMonitoringHandshakeFailureAppendsFailedTransition(t *testing.T) {
	ctx := context.Background()
	database, commands, observation := seedRuntimeObservation(t, ctx)
	queued, err := commands.PrepareRuntimeIntent(ctx, store.RuntimeIntentRestart, "")
	if err != nil {
		t.Fatal(err)
	}
	intent := queued
	services := &runtimeServices{
		database: database,
		commands: commands,
		manager: &fakeRuntimeManager{live: coreruntime.LiveIdentity{
			State: coreruntime.StateStopped, TransitionedAt: observation.ObservedAt.Add(time.Second),
		}},
	}
	commit, err := services.stopAfterLostIntent(
		&observation,
		intent,
		observation.ActivationBundleID,
		store.RuntimeTransitionFailed,
		"monitoring_handshake_failed",
	)
	if err != nil {
		t.Fatal(err)
	}
	if stored, err := database.RuntimeObservation(ctx); err != nil || stored.PID != observation.PID {
		t.Fatalf("runtime observation changed before terminal completion = %+v, %v", stored, err)
	}
	if err := database.CompleteRuntimeIntent(ctx, intent, false, commit, time.Now().UTC()); err != nil {
		t.Fatalf("CompleteTask(handshake failure) error = %v", err)
	}
	history, err := database.ListRuntimeTransitions(ctx, store.RuntimeHistoryFilter{
		Reason: "monitoring_handshake_failed",
		Limit:  10,
	})
	if err != nil || len(history.Items) != 1 || history.Items[0].State != store.RuntimeTransitionFailed ||
		history.Items[0].Generation != intent.Generation {
		t.Fatalf("handshake-failure runtime history = %+v, %v", history, err)
	}
}

func TestPrepareRuntimeIntentRunningCommitDefersIntentSpecificTransitions(t *testing.T) {
	tests := []struct {
		kind       store.RuntimeIntentKind
		recovery   *store.RuntimeRecoveryMetadata
		wantReason string
	}{
		{kind: store.RuntimeIntentApply, wantReason: "apply_succeeded"},
		{kind: store.RuntimeIntentStart, wantReason: "start_succeeded"},
		{kind: store.RuntimeIntentRestart, wantReason: "restart_succeeded"},
		{kind: store.RuntimeIntentRollback, wantReason: "rollback_succeeded"},
		{
			kind: store.RuntimeIntentStart, recovery: &store.RuntimeRecoveryMetadata{EpisodeID: "test"},
			wantReason: "recovery_succeeded",
		},
	}
	for _, test := range tests {
		t.Run(test.wantReason, func(t *testing.T) {
			ctx := context.Background()
			database, commands, previous := seedRuntimeObservation(t, ctx)
			activation, err := database.GetActivationBundle(ctx, previous.ActivationBundleID)
			if err != nil {
				t.Fatal(err)
			}
			startup, err := database.GetStartupArtifact(ctx, activation.StartupArtifactID)
			if err != nil {
				t.Fatal(err)
			}
			core, err := database.GetCoreArtifact(ctx, startup.CoreArtifactID)
			if err != nil {
				t.Fatal(err)
			}
			startedAt := previous.ObservedAt.Add(time.Minute)
			manager := &fakeRuntimeManager{live: coreruntime.LiveIdentity{
				Running: true, State: coreruntime.StateRunning, PID: previous.PID + 1,
				BundleID: activation.ID, StartedAt: startedAt,
				TransitionedAt: startedAt.Add(time.Second),
			}}
			services := &runtimeServices{
				database: database, commands: commands, manager: manager,
				identity: &fakeRuntimeIdentityResolver{startToken: "intent-specific-incarnation"},
			}
			intent := store.RuntimeIntent{Kind: test.kind, Generation: 2, Recovery: test.recovery}
			observation, err := services.recordLiveObservation(
				ctx,
				application.RuntimeMaterial{Activation: activation, Startup: startup, Core: core},
				&previous,
			)
			if err != nil {
				t.Fatal(err)
			}
			premature, err := database.ListRuntimeTransitions(ctx, store.RuntimeHistoryFilter{
				Reason: test.wantReason,
				Limit:  10,
			})
			if err != nil || len(premature.Items) != 0 {
				t.Fatalf("premature %s runtime history = %+v, %v", test.wantReason, premature, err)
			}
			observation, commit, err := services.prepareRuntimeIntentRunningCommit(ctx, intent, observation, &previous)
			if err != nil {
				t.Fatal(err)
			}
			if observation.PID != previous.PID+1 {
				t.Fatalf("recorded runtime observation = %+v", observation)
			}
			history, err := database.ListRuntimeTransitions(ctx, store.RuntimeHistoryFilter{
				Reason: test.wantReason,
				Limit:  10,
			})
			if err != nil || len(history.Items) != 0 {
				t.Fatalf("%s history was written before intent completion = %+v, %v", test.wantReason, history, err)
			}
			if len(commit.Transitions) != 2 || commit.Transitions[0].State != store.RuntimeTransitionStopped ||
				commit.Transitions[0].Reason != runtimeIntentReason(intent, "boundary") ||
				commit.Transitions[1].State != store.RuntimeTransitionRunning ||
				commit.Transitions[1].Reason != test.wantReason {
				t.Fatalf("prepared runtime intent commit = %+v", commit)
			}
		})
	}
}

func TestRuntimeReconcilerLogsExhaustedEpisodeOnce(t *testing.T) {
	ctx := context.Background()
	database, commands, observation := seedRuntimeObservation(t, ctx)
	reconciler := &runtimeReconciler{
		services: &runtimeServices{database: database, commands: commands},
	}
	result := application.RuntimeRecoveryResult{
		BundleID: observation.ActivationBundleID, Generation: 4,
		EpisodeID: "episode-exhausted", Attempt: store.RuntimeRecoveryMaximumAttempts, Exhausted: true,
	}
	reconciler.recordRecoveryDecision(result)
	reconciler.recordRecoveryDecision(result)
	assertSingleRuntimeLog(
		t, ctx, database, "runtime.recovery_exhausted", observation.ActivationBundleID,
		4, store.RuntimeRecoveryMaximumAttempts,
	)
}

func assertSingleRuntimeLog(
	t *testing.T,
	ctx context.Context,
	database *store.Store,
	code string,
	bundleID string,
	generation int64,
	attempt int,
) {
	t.Helper()
	page, err := database.ListLogEntries(ctx, store.LogListFilter{Code: code, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 {
		t.Fatalf("%s log entries = %d, want 1: %+v", code, len(page.Items), page.Items)
	}
	var metadata struct {
		BundleID   string `json:"bundle_id"`
		Generation int64  `json:"generation"`
		Attempt    int    `json:"attempt"`
	}
	if err := json.Unmarshal(page.Items[0].Metadata, &metadata); err != nil {
		t.Fatal(err)
	}
	if metadata.BundleID != bundleID || metadata.Generation != generation || metadata.Attempt != attempt {
		t.Fatalf("%s metadata = %+v", code, metadata)
	}
}

func seedRuntimeObservation(
	t *testing.T,
	ctx context.Context,
) (*store.Store, *application.Application, store.RuntimeObservation) {
	t.Helper()
	database, err := store.Open(ctx, filepath.Join(t.TempDir(), "panel.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	now := time.Date(2026, time.August, 29, 9, 0, 0, 0, time.UTC)
	revision, err := testutil.SaveConfiguration(ctx, database, 0, store.NewCanonicalRevision{
		ID: "revision-runtime-test", SchemaVersion: configuration.SchemaVersion,
		Document: configuration.Empty().CanonicalJSON(), CommandID: "command-runtime-test", CreatedAt: now,
	},
	)
	if err != nil {
		t.Fatal(err)
	}
	core := store.CoreArtifact{
		ID: "core-runtime-test", ExactVersion: "1.13.19", OperatingSystem: "linux", Architecture: "arm64", Variant: "musl",
		SourceKind: store.CoreArtifactSourceUserVerified, UserSource: "runtime state test",
		ArchiveSHA256: strings.Repeat("a", 64), BinarySHA256: strings.Repeat("b", 64),
		BinaryPath: "/opt/sing-box-panel/core-runtime-test/sing-box", ReportedVersion: "1.13.19",
		FeatureFingerprint: json.RawMessage(`{}`), CreatedAt: now,
	}
	if _, err := database.UpsertCoreArtifact(ctx, core); err != nil {
		t.Fatal(err)
	}
	startup, err := database.CreateStartupArtifact(ctx, store.StartupArtifact{
		ID: "startup-runtime-test", CanonicalRevisionID: revision.ID, ExactCoreVersion: core.ExactVersion,
		CoreArtifactID: core.ID, ConfigBytes: []byte(`{}`), CreatedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	startup, err = database.CompleteStartupArtifactCheck(ctx, startup.ID, true, now.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	bundle, err := database.SaveActivationBundle(ctx, store.ActivationBundle{
		ID: "bundle-runtime-test", StartupArtifactID: startup.ID,
		MonitoringTier: store.MonitoringProcessOnly, CreatedAt: now.Add(2 * time.Second),
	})
	if err != nil {
		t.Fatal(err)
	}
	intent, err := database.RequestRuntimeIntent(ctx, store.RuntimeIntentInput{Kind: store.RuntimeIntentApply, BundleID: bundle.ID, CreatedAt: now.Add(3 * time.Second)})
	if err != nil {
		t.Fatal(err)
	}
	observation, err := database.RecordRuntimeObservation(ctx, store.RuntimeObservation{
		PID: 4242, ProcessStartToken: "process-start-runtime-test", CoreArtifactID: core.ID,
		ActivationBundleID: bundle.ID, ExactCoreVersion: core.ExactVersion,
		ArchiveSHA256: core.ArchiveSHA256, BinarySHA256: core.BinarySHA256,
		StartedAt: now.Add(6 * time.Second), ObservedAt: now.Add(7 * time.Second),
	})
	if err != nil {
		t.Fatal(err)
	}
	started := observation.StartedAt
	if err := database.CompleteRuntimeIntent(ctx, intent, true, &store.RuntimeCommit{ExpectedObservation: &observation, Observation: &observation, Transitions: []store.RuntimeTransitionInput{{DedupeKey: "fixture-running", State: store.RuntimeTransitionRunning, Reason: "fixture_running", ActivationBundleID: bundle.ID, Generation: intent.Generation, PID: observation.PID, ProcessStartToken: observation.ProcessStartToken, ProcessStartedAt: &started, OccurredAt: observation.ObservedAt}}}, observation.ObservedAt); err != nil {
		t.Fatal(err)
	}
	return database, application.FromStore(database), observation
}

func TestStartupCheckUsesRawRevisionWithExactBinaryWithoutSchema(t *testing.T) {
	ctx := context.Background()
	database, err := store.Open(ctx, filepath.Join(t.TempDir(), "panel.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	now := time.Date(2026, time.August, 31, 13, 0, 0, 0, time.UTC)
	raw := json.RawMessage(`{"future_option":{"enabled":true}}`)
	revision, err := testutil.SaveConfiguration(ctx, database, 0, store.NewCanonicalRevision{
		ID: "revision-raw-startup-check", SchemaVersion: configuration.SchemaVersion,
		Document: raw, CommandID: "command-raw-startup-check", CreatedAt: now,
	},
	)
	if err != nil {
		t.Fatal(err)
	}
	core := store.CoreArtifact{
		ID: "core-raw-startup-check", ExactVersion: "1.13.19", OperatingSystem: "linux", Architecture: "arm64", Variant: "musl",
		SourceKind: store.CoreArtifactSourceUserVerified, UserSource: "raw startup check",
		ArchiveSHA256: strings.Repeat("a", 64), BinarySHA256: strings.Repeat("b", 64),
		BinaryPath: "/opt/sing-box-panel/core-raw-startup-check/sing-box", ReportedVersion: "1.13.19",
		FeatureFingerprint: json.RawMessage(`{"status":"not_reported"}`), CreatedAt: now,
	}
	if _, err := database.UpsertCoreArtifact(ctx, core); err != nil {
		t.Fatal(err)
	}
	startup, err := database.CreateStartupArtifact(ctx, store.StartupArtifact{
		ID: "startup-raw-check", CanonicalRevisionID: revision.ID,
		ExactCoreVersion: core.ExactVersion, CoreArtifactID: core.ID,
		ConfigBytes: raw, CreatedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	manager := &fakeRuntimeManager{}
	services := &runtimeServices{commands: application.FromStore(database), database: database, manager: manager}
	result, err := services.checkStartup(ctx, startup.ID)
	if err != nil {
		t.Fatal(err)
	}
	if manager.checkedBundle == nil || string(manager.checkedBundle.StartupConfig) != string(raw) ||
		manager.checkedBundle.BinaryPath != core.BinaryPath || result.State != store.StartupArtifactReady {
		t.Fatalf("checked bundle = %+v, result = %+v", manager.checkedBundle, result)
	}
	stored, err := database.GetStartupArtifact(ctx, startup.ID)
	if err != nil || stored.State != store.StartupArtifactReady {
		t.Fatalf("stored startup = %+v, error = %v", stored, err)
	}
}

type fakeRuntimeIdentityResolver struct {
	identity        application.RuntimeIdentity
	err             error
	resolveCalls    int
	startToken      string
	startTokenErr   error
	startTokenCalls int
}

func (resolver *fakeRuntimeIdentityResolver) Resolve(context.Context) (application.RuntimeIdentity, error) {
	resolver.resolveCalls++
	return resolver.identity, resolver.err
}

func (resolver *fakeRuntimeIdentityResolver) ProcessStartToken(context.Context, int) (string, error) {
	resolver.startTokenCalls++
	return resolver.startToken, resolver.startTokenErr
}

type fakeRuntimeManager struct {
	stopCalls     int
	stopErr       error
	closeErr      error
	waitErr       error
	closeHook     func()
	live          coreruntime.LiveIdentity
	checkedBundle *coreruntime.AppliedBundle
}

func (manager *fakeRuntimeManager) Check(_ context.Context, bundle coreruntime.AppliedBundle) error {
	cloned := bundle
	cloned.StartupConfig = append([]byte(nil), bundle.StartupConfig...)
	manager.checkedBundle = &cloned
	return nil
}
func (*fakeRuntimeManager) Start(context.Context, coreruntime.AppliedBundle) error { return nil }
func (manager *fakeRuntimeManager) Stop(context.Context) error {
	manager.stopCalls++
	return manager.stopErr
}
func (*fakeRuntimeManager) Restart(context.Context, coreruntime.AppliedBundle) error { return nil }
func (manager *fakeRuntimeManager) Close(context.Context) error {
	if manager.closeHook != nil {
		manager.closeHook()
	}
	return manager.closeErr
}
func (manager *fakeRuntimeManager) Wait() error { return manager.waitErr }
func (*fakeRuntimeManager) MonitoringLevel() coreruntime.MonitoringLevel {
	return coreruntime.MonitoringProcessOnly
}
func (manager *fakeRuntimeManager) ObserveLiveIdentity() coreruntime.LiveIdentity {
	return manager.live
}

type successfulRuntimeGuard struct{}

func (successfulRuntimeGuard) SafePoint(context.Context) error { return nil }

type runtimeGuardFunc func(context.Context) error

func (control runtimeGuardFunc) SafePoint(ctx context.Context) error { return control(ctx) }
