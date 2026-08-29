// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"context"
	"encoding/json"
	"errors"
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

func TestReconcileStartupPreservesObservationWhileUserIntentOwnsRuntimeLane(t *testing.T) {
	ctx := context.Background()
	database, commands, observation := seedRuntimeObservation(t, ctx)
	userTask, err := commands.QueueRuntimeRestart(ctx)
	if err != nil {
		t.Fatal(err)
	}
	services := &runtimeServices{
		database: database, commands: commands, manager: &fakeRuntimeManager{},
		identity: &fakeRuntimeIdentityResolver{startTokenErr: os.ErrNotExist},
	}
	if err := services.ReconcileStartup(ctx); err != nil {
		t.Fatalf("ReconcileStartup() error = %v", err)
	}
	stored, err := database.RuntimeObservation(ctx)
	if err != nil || stored.PID != observation.PID || stored.ProcessStartToken != observation.ProcessStartToken {
		t.Fatalf("observation while user task is active = %+v, %v", stored, err)
	}
	current, err := database.GetTask(ctx, userTask.ID)
	if err != nil || current.Status != store.TaskStatusQueued {
		t.Fatalf("explicit runtime task after startup reconciliation = %+v, %v", current, err)
	}
}

func TestReconcileStartupClearsExitedObservationWhenDesiredStopped(t *testing.T) {
	ctx := context.Background()
	database, commands, _ := seedRuntimeObservation(t, ctx)
	if _, err := commands.QueueRuntimeStop(ctx); err != nil {
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

func TestStopForTaskFailsBeforeStoppingProcessOnObservationReadError(t *testing.T) {
	ctx := context.Background()
	database, commands, _ := seedRuntimeObservation(t, ctx)
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}
	manager := &fakeRuntimeManager{}
	services := &runtimeServices{database: database, commands: commands, manager: manager}
	if _, err := services.stopForTask(ctx, successfulTaskControl{}); err == nil {
		t.Fatal("stopForTask() succeeded after observation read failed")
	}
	if manager.stopCalls != 0 {
		t.Fatalf("manager.Stop() calls = %d, want 0", manager.stopCalls)
	}
}

func TestStopForTaskClearsOnlyCapturedIncarnationProvenExited(t *testing.T) {
	tests := []struct {
		name          string
		startToken    string
		startTokenErr error
		wantCleared   bool
	}{
		{name: "process disappeared", startTokenErr: os.ErrNotExist, wantCleared: true},
		{name: "PID was reused", startToken: "replacement-incarnation", wantCleared: true},
		{name: "same incarnation is retained", startToken: "process-start-runtime-test"},
		{name: "inspection failure is retained", startTokenErr: errors.New("permission denied")},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			database, commands, observation := seedRuntimeObservation(t, ctx)
			manager := &fakeRuntimeManager{stopErr: errors.New("termination failed")}
			resolver := &fakeRuntimeIdentityResolver{startToken: test.startToken, startTokenErr: test.startTokenErr}
			services := &runtimeServices{database: database, commands: commands, manager: manager, identity: resolver}
			if _, err := services.stopForTask(ctx, successfulTaskControl{}); err == nil {
				t.Fatal("stopForTask() succeeded after termination failed")
			}
			stored, err := database.RuntimeObservation(ctx)
			if test.wantCleared {
				if !errors.Is(err, store.ErrRuntimeObservationNotFound) {
					t.Fatalf("proven exited observation was retained: %+v, %v", stored, err)
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
	if err != nil || !beforeStable.ObservedAt.Equal(observation.ObservedAt) {
		t.Fatalf("observation before stable window = %+v, %v", beforeStable, err)
	}
	clock.Advance(time.Second)
	reconciler.reconcile(ctx)
	stable, err := database.RuntimeObservation(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !runtimeObservationProvesStableRun(&stable) || !stable.ObservedAt.Equal(clock.Now()) {
		t.Fatalf("stable runtime observation = %+v; clock = %v", stable, clock.Now())
	}
	if resolver.startTokenCalls != 3 {
		t.Fatalf("ProcessStartToken() calls = %d, want one per reconciliation", resolver.startTokenCalls)
	}
}

func TestRuntimeReconcilerLogsUnexpectedExitAndRecoveryQueueOnce(t *testing.T) {
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
	assertSingleRuntimeLog(t, ctx, database, "runtime.recovery_queued", observation.ActivationBundleID, 2, 1)
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
	revision, err := database.SaveCanonicalRevisionAndTask(ctx, "", store.NewCanonicalRevision{
		ID: "revision-runtime-test", SchemaVersion: configuration.SchemaVersion,
		Document: configuration.Empty().CanonicalJSON(), CommandID: "command-runtime-test", CreatedAt: now,
	}, store.NewTask{
		ID: "task-runtime-canonical", IdempotencyKey: "canonical:runtime-test",
		Lane: store.TaskLaneMaintenance, Kind: store.TaskKindCanonicalSaved,
		Payload: json.RawMessage(`{}`), CreatedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	core := store.CoreArtifact{
		ID: "core-runtime-test", ExactVersion: "1.13.19", OperatingSystem: "linux", Architecture: "arm64", Variant: "plain",
		SourceKind: store.CoreArtifactSourceUserVerified, UserSource: "runtime state test",
		ArchiveSHA256: strings.Repeat("a", 64), BinarySHA256: strings.Repeat("b", 64),
		BinaryPath: "/opt/sing-box-panel/core-runtime-test/sing-box", ReportedVersion: "1.13.19",
		FeatureFingerprint: json.RawMessage(`{}`), VerificationState: store.CoreArtifactVerified, CreatedAt: now,
	}
	if _, err := database.UpsertCoreArtifact(ctx, core); err != nil {
		t.Fatal(err)
	}
	startup, err := database.CreateStartupArtifact(ctx, store.StartupArtifact{
		ID: "startup-runtime-test", CanonicalRevisionID: revision.ID, ExactCoreVersion: core.ExactVersion,
		AdapterID: "test-adapter", AdapterRevision: "1", CoreArtifactID: core.ID,
		ConfigBytes: []byte(`{}`), Diagnostics: json.RawMessage(`[]`), CreatedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	startup, err = database.CompleteStartupArtifactCheck(ctx, startup.ID, true, json.RawMessage(`[]`), now.Add(time.Second))
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
	apply, err := database.RequestRuntimeIntent(ctx, store.RuntimeIntentInput{
		TaskID: "task-runtime-apply", Kind: store.RuntimeIntentApply,
		BundleID: bundle.ID, CreatedAt: now.Add(3 * time.Second),
	})
	if err != nil {
		t.Fatal(err)
	}
	claimed, err := database.ClaimTask(ctx, store.ClaimTaskInput{
		Lane: store.TaskLaneRuntime, LeaseOwner: "runtime-test",
		Now: now.Add(4 * time.Second), LeaseDuration: time.Minute,
	})
	if err != nil || claimed == nil || claimed.ID != apply.ID {
		t.Fatalf("ClaimTask(runtime apply) = %+v, %v", claimed, err)
	}
	if _, err := database.CompleteTask(
		ctx, claimed.ID, claimed.LeaseOwner, now.Add(5*time.Second), store.TaskCompletion{Succeeded: true},
	); err != nil {
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
	return database, application.FromStore(database), observation
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
	stopCalls int
	stopErr   error
	closeErr  error
	waitErr   error
	closeHook func()
	live      coreruntime.LiveIdentity
}

func (*fakeRuntimeManager) Check(context.Context, coreruntime.AppliedBundle) error { return nil }
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

type successfulTaskControl struct{}

func (successfulTaskControl) SafePoint(context.Context) error { return nil }
