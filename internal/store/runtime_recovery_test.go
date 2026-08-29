// SPDX-License-Identifier: GPL-3.0-or-later

package store

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/rehuony/sing-box-panel/internal/configuration"
)

func TestRuntimeRecoveryPersistsEpisodeBackoffAndExhaustion(t *testing.T) {
	ctx := testContext(t)
	database := openTestStore(t, ctx)
	now := time.Date(2026, time.August, 29, 12, 0, 0, 0, time.UTC)
	bundle, observation := seedAppliedRuntime(t, ctx, database, now)

	first := requestRecovery(t, ctx, database, RuntimeRecoveryInput{
		TaskID: "recovery-1", NewEpisodeID: "episode-durable", ExpectedBundleID: bundle.ID,
		ExpectedGeneration: 1, ExpectedObservation: &observation, CreatedAt: now.Add(10 * time.Second),
	})
	assertRecoveryDecision(t, first, "episode-durable", 1, time.Second)
	completeRecoveryTask(t, ctx, database, first.Task, false)

	path := database.Path()
	if err := database.Close(); err != nil {
		t.Fatalf("Close() before recovery history reload error = %v", err)
	}
	database, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("Open() after recovery history reload error = %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })

	second := requestRecovery(t, ctx, database, RuntimeRecoveryInput{
		TaskID: "recovery-2", NewEpisodeID: "episode-ignored", ExpectedBundleID: bundle.ID,
		ExpectedGeneration: 2, CreatedAt: now.Add(20 * time.Second),
	})
	assertRecoveryDecision(t, second, "episode-durable", 2, 5*time.Second)
	completeRecoveryTask(t, ctx, database, second.Task, false)

	third := requestRecovery(t, ctx, database, RuntimeRecoveryInput{
		TaskID: "recovery-3", NewEpisodeID: "episode-still-ignored", ExpectedBundleID: bundle.ID,
		ExpectedGeneration: 3, CreatedAt: now.Add(30 * time.Second),
	})
	assertRecoveryDecision(t, third, "episode-durable", 3, 30*time.Second)
	completeRecoveryTask(t, ctx, database, third.Task, false)

	exhausted := requestRecovery(t, ctx, database, RuntimeRecoveryInput{
		TaskID: "recovery-4", NewEpisodeID: "episode-never-used", ExpectedBundleID: bundle.ID,
		ExpectedGeneration: 4, CreatedAt: now.Add(time.Minute),
	})
	if !exhausted.Exhausted || exhausted.Task != nil || exhausted.EpisodeID != "episode-durable" ||
		exhausted.Attempt != RuntimeRecoveryMaximumAttempts {
		t.Fatalf("exhausted decision = %+v", exhausted)
	}
	bootstrap, err := database.Bootstrap(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if bootstrap.Hub.TargetGeneration != 4 {
		t.Fatalf("generation after exhausted recovery = %d, want 4", bootstrap.Hub.TargetGeneration)
	}
}

func TestRuntimeRecoveryStableWindowStartsNewEpisode(t *testing.T) {
	tests := []struct {
		name            string
		uptime          time.Duration
		stableRunProven bool
		wantEpisode     string
		wantAttempt     int
	}{
		{name: "quick crash continues episode", uptime: RuntimeRecoveryStableWindow - time.Second, wantEpisode: "episode-first", wantAttempt: 2},
		{
			name:   "short lived process and long panel downtime continues episode",
			uptime: 24 * time.Hour, wantEpisode: "episode-first", wantAttempt: 2,
		},
		{
			name:   "continuously observed stable process resets episode",
			uptime: RuntimeRecoveryStableWindow, stableRunProven: true,
			wantEpisode: "episode-reset", wantAttempt: 1,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx := testContext(t)
			database := openTestStore(t, ctx)
			now := time.Date(2026, time.August, 29, 13, 0, 0, 0, time.UTC)
			bundle, observation := seedAppliedRuntime(t, ctx, database, now)
			first := requestRecovery(t, ctx, database, RuntimeRecoveryInput{
				TaskID: "recovery-first", NewEpisodeID: "episode-first", ExpectedBundleID: bundle.ID,
				ExpectedGeneration: 1, ExpectedObservation: &observation, CreatedAt: now.Add(10 * time.Second),
			})
			completeRecoveryTask(t, ctx, database, first.Task, true)

			startedAt := first.Task.NotBefore.Add(time.Second)
			failed := runtimeObservationFixture(bundle, 2202, "process-recovered", startedAt, startedAt.Add(time.Second))
			if _, err := database.RecordRuntimeObservation(ctx, failed); err != nil {
				t.Fatalf("RecordRuntimeObservation(recovered process) error = %v", err)
			}
			decision := requestRecovery(t, ctx, database, RuntimeRecoveryInput{
				TaskID: "recovery-next", NewEpisodeID: "episode-reset", ExpectedBundleID: bundle.ID,
				ExpectedGeneration: 2, ExpectedObservation: &failed,
				StableRunProven: test.stableRunProven, CreatedAt: startedAt.Add(test.uptime),
			})
			if decision.Task == nil || decision.EpisodeID != test.wantEpisode || decision.Attempt != test.wantAttempt {
				t.Fatalf("recovery after uptime %s = %+v", test.uptime, decision)
			}
		})
	}
}

func TestRuntimeRecoveryCleanRestartAfterSuccessStartsNewEpisode(t *testing.T) {
	ctx := testContext(t)
	database := openTestStore(t, ctx)
	now := time.Date(2026, time.August, 29, 13, 30, 0, 0, time.UTC)
	bundle, observation := seedAppliedRuntime(t, ctx, database, now)
	first := requestRecovery(t, ctx, database, RuntimeRecoveryInput{
		TaskID: "recovery-before-clean-restart", NewEpisodeID: "episode-before-clean-restart",
		ExpectedBundleID: bundle.ID, ExpectedGeneration: 1,
		ExpectedObservation: &observation, CreatedAt: now.Add(10 * time.Second),
	})
	completeRecoveryTask(t, ctx, database, first.Task, true)

	// Successful runtime tasks normally record a process observation; a clean
	// server Close removes that exact record. Absence at the next startup is a
	// clean episode boundary, not another failure attempt.
	afterRestart := requestRecovery(t, ctx, database, RuntimeRecoveryInput{
		TaskID: "recovery-after-clean-restart", NewEpisodeID: "episode-after-clean-restart",
		ExpectedBundleID: bundle.ID, ExpectedGeneration: 2,
		CleanBoundaryProven: true, CreatedAt: now.Add(20 * time.Second),
	})
	if afterRestart.Task == nil || afterRestart.EpisodeID != "episode-after-clean-restart" || afterRestart.Attempt != 1 {
		t.Fatalf("recovery after clean restart = %+v", afterRestart)
	}
}

func TestExplicitRuntimeIntentStartsFreshRecoveryEpisode(t *testing.T) {
	for _, kind := range []RuntimeIntentKind{RuntimeIntentStart, RuntimeIntentRestart, RuntimeIntentApply} {
		t.Run(string(kind), func(t *testing.T) {
			ctx := testContext(t)
			database := openTestStore(t, ctx)
			now := time.Date(2026, time.August, 29, 13, 45, 0, 0, time.UTC)
			bundle, observation := seedAppliedRuntime(t, ctx, database, now)
			first := requestRecovery(t, ctx, database, RuntimeRecoveryInput{
				TaskID: "recovery-before-user-intent", NewEpisodeID: "episode-before-user-intent",
				ExpectedBundleID: bundle.ID, ExpectedGeneration: 1,
				ExpectedObservation: &observation, CreatedAt: now.Add(10 * time.Second),
			})
			completeRecoveryTask(t, ctx, database, first.Task, false)

			input := RuntimeIntentInput{
				TaskID: "user-intent-between-episodes", Kind: kind, CreatedAt: now.Add(20 * time.Second),
			}
			if kind == RuntimeIntentApply {
				input.BundleID = bundle.ID
			}
			userTask, err := database.RequestRuntimeIntent(ctx, input)
			if err != nil {
				t.Fatal(err)
			}
			claimed, err := database.ClaimTask(ctx, ClaimTaskInput{
				Lane: TaskLaneRuntime, LeaseOwner: "user-intent-test",
				Now: now.Add(21 * time.Second), LeaseDuration: time.Minute,
			})
			if err != nil || claimed == nil || claimed.ID != userTask.ID {
				t.Fatalf("ClaimTask(user intent) = %+v, %v", claimed, err)
			}
			if _, err := database.CompleteTask(
				ctx, claimed.ID, claimed.LeaseOwner, now.Add(22*time.Second), TaskCompletion{Succeeded: true},
			); err != nil {
				t.Fatal(err)
			}
			afterUserIntent := runtimeObservationFixture(
				bundle, 4404, "process-after-user-intent", now.Add(23*time.Second), now.Add(24*time.Second),
			)
			if _, err := database.RecordRuntimeObservation(ctx, afterUserIntent); err != nil {
				t.Fatal(err)
			}
			decision := requestRecovery(t, ctx, database, RuntimeRecoveryInput{
				TaskID: "recovery-after-user-intent", NewEpisodeID: "episode-after-user-intent",
				ExpectedBundleID: bundle.ID, ExpectedGeneration: userTask.Generation,
				ExpectedObservation: &afterUserIntent, CreatedAt: now.Add(25 * time.Second),
			})
			if decision.Task == nil || decision.EpisodeID != "episode-after-user-intent" || decision.Attempt != 1 {
				t.Fatalf("recovery after explicit %s = %+v", kind, decision)
			}
		})
	}
}

func TestRuntimeRecoveryObservationFencePreservesNewIncarnation(t *testing.T) {
	ctx := testContext(t)
	database := openTestStore(t, ctx)
	now := time.Date(2026, time.August, 29, 14, 0, 0, 0, time.UTC)
	bundle, oldObservation := seedAppliedRuntime(t, ctx, database, now)
	newObservation := runtimeObservationFixture(bundle, 3303, "process-new", now.Add(time.Second), now.Add(2*time.Second))
	if _, err := database.RecordRuntimeObservation(ctx, newObservation); err != nil {
		t.Fatal(err)
	}

	decision := requestRecovery(t, ctx, database, RuntimeRecoveryInput{
		TaskID: "recovery-fenced", NewEpisodeID: "episode-fenced", ExpectedBundleID: bundle.ID,
		ExpectedGeneration: 1, ExpectedObservation: &oldObservation, CreatedAt: now.Add(3 * time.Second),
	})
	if decision != (RuntimeRecoveryDecision{}) {
		t.Fatalf("fenced recovery decision = %+v, want zero", decision)
	}
	stored, err := database.RuntimeObservation(ctx)
	if err != nil || stored.PID != newObservation.PID || stored.ProcessStartToken != newObservation.ProcessStartToken {
		t.Fatalf("new observation after stale cleanup = %+v, %v", stored, err)
	}
}

func TestRuntimeRecoveryRacesExplicitUserIntent(t *testing.T) {
	tests := []struct {
		name string
		kind RuntimeIntentKind
		want bool
	}{
		{name: "stop wins desired state", kind: RuntimeIntentStop, want: false},
		{name: "restart wins current generation", kind: RuntimeIntentRestart, want: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx := testContext(t)
			first := openTestStore(t, ctx)
			now := time.Date(2026, time.August, 29, 15, 0, 0, 0, time.UTC)
			bundle, observation := seedAppliedRuntime(t, ctx, first, now)
			second, err := Open(ctx, first.Path())
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = second.Close() })

			start := make(chan struct{})
			var recovery RuntimeRecoveryDecision
			var userTask Task
			var recoveryErr, userErr error
			var workers sync.WaitGroup
			workers.Add(2)
			go func() {
				defer workers.Done()
				<-start
				recovery, recoveryErr = first.RequestRuntimeRecovery(ctx, RuntimeRecoveryInput{
					TaskID: "recovery-race", NewEpisodeID: "episode-race", ExpectedBundleID: bundle.ID,
					ExpectedGeneration: 1, ExpectedObservation: &observation, CreatedAt: now.Add(time.Second),
				})
			}()
			go func() {
				defer workers.Done()
				<-start
				userTask, userErr = second.RequestRuntimeIntent(ctx, RuntimeIntentInput{
					TaskID: "user-intent", Kind: test.kind, CreatedAt: now.Add(2 * time.Second),
				})
			}()
			close(start)
			workers.Wait()
			if recoveryErr != nil || userErr != nil {
				t.Fatalf("race errors: recovery=%v user=%v", recoveryErr, userErr)
			}
			bootstrap, err := first.Bootstrap(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if bootstrap.Hub.DesiredRunning != test.want || bootstrap.Hub.TargetGeneration != userTask.Generation {
				t.Fatalf("hub after race = %+v; user task = %+v; recovery = %+v", bootstrap.Hub, userTask, recovery)
			}
			current, err := first.GetTask(ctx, userTask.ID)
			if err != nil {
				t.Fatal(err)
			}
			if current.Generation != bootstrap.Hub.TargetGeneration || current.Status != TaskStatusQueued {
				t.Fatalf("current explicit task = %+v; hub = %+v", current, bootstrap.Hub)
			}
		})
	}
}

func seedAppliedRuntime(
	t *testing.T,
	ctx context.Context,
	database *Store,
	now time.Time,
) (ActivationBundle, RuntimeObservation) {
	t.Helper()
	revision, err := database.SaveCanonicalRevisionAndTask(ctx, "", NewCanonicalRevision{
		ID: "revision-runtime-recovery", SchemaVersion: configuration.SchemaVersion,
		Document: configuration.Empty().CanonicalJSON(), CommandID: "command-runtime-recovery", CreatedAt: now,
	}, NewTask{
		ID: "canonical-runtime-recovery", IdempotencyKey: "canonical:runtime-recovery",
		Lane: TaskLaneMaintenance, Kind: TaskKindCanonicalSaved, Payload: json.RawMessage(`{}`), CreatedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	core := testCoreArtifact("core-runtime-recovery", 909, 'a', "amd64", now)
	if _, err := database.UpsertCoreArtifact(ctx, core); err != nil {
		t.Fatal(err)
	}
	startup, err := database.CreateStartupArtifact(ctx, StartupArtifact{
		ID: "startup-runtime-recovery", CanonicalRevisionID: revision.ID, ExactCoreVersion: core.ExactVersion,
		AdapterID: "test-adapter", AdapterRevision: "1", CoreArtifactID: core.ID,
		ConfigBytes: []byte(`{}`), Diagnostics: json.RawMessage(`[]`), CreatedAt: now.Add(time.Second),
	})
	if err != nil {
		t.Fatal(err)
	}
	startup, err = database.CompleteStartupArtifactCheck(ctx, startup.ID, true, json.RawMessage(`[]`), now.Add(2*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	bundle, err := database.SaveActivationBundle(ctx, ActivationBundle{
		ID: "bundle-runtime-recovery", StartupArtifactID: startup.ID,
		MonitoringTier: MonitoringProcessOnly, CreatedAt: now.Add(3 * time.Second),
	})
	if err != nil {
		t.Fatal(err)
	}
	apply, err := database.RequestRuntimeIntent(ctx, RuntimeIntentInput{
		TaskID: "runtime-apply-recovery", Kind: RuntimeIntentApply, BundleID: bundle.ID, CreatedAt: now.Add(4 * time.Second),
	})
	if err != nil {
		t.Fatal(err)
	}
	claimed, err := database.ClaimTask(ctx, ClaimTaskInput{
		Lane: TaskLaneRuntime, LeaseOwner: "runtime-recovery-seed", Now: now.Add(5 * time.Second), LeaseDuration: time.Minute,
	})
	if err != nil || claimed == nil || claimed.ID != apply.ID {
		t.Fatalf("ClaimTask(seed apply) = %+v, %v", claimed, err)
	}
	if _, err := database.CompleteTask(
		ctx, claimed.ID, claimed.LeaseOwner, now.Add(6*time.Second), TaskCompletion{Succeeded: true},
	); err != nil {
		t.Fatal(err)
	}
	observation := runtimeObservationFixture(bundle, 1101, "process-original", now.Add(7*time.Second), now.Add(8*time.Second))
	if _, err := database.RecordRuntimeObservation(ctx, observation); err != nil {
		t.Fatal(err)
	}
	return bundle, observation
}

func runtimeObservationFixture(
	bundle ActivationBundle,
	pid int,
	startToken string,
	startedAt time.Time,
	observedAt time.Time,
) RuntimeObservation {
	return RuntimeObservation{
		PID: pid, ProcessStartToken: startToken, CoreArtifactID: "core-runtime-recovery",
		ActivationBundleID: bundle.ID, ExactCoreVersion: "1.13.19",
		ArchiveSHA256: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		BinarySHA256:  "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		StartedAt:     startedAt, ObservedAt: observedAt,
	}
}

func requestRecovery(
	t *testing.T,
	ctx context.Context,
	database *Store,
	input RuntimeRecoveryInput,
) RuntimeRecoveryDecision {
	t.Helper()
	decision, err := database.RequestRuntimeRecovery(ctx, input)
	if err != nil {
		t.Fatalf("RequestRuntimeRecovery() error = %v", err)
	}
	return decision
}

func assertRecoveryDecision(
	t *testing.T,
	decision RuntimeRecoveryDecision,
	episode string,
	attempt int,
	delay time.Duration,
) {
	t.Helper()
	if decision.Task == nil || decision.Exhausted || decision.EpisodeID != episode || decision.Attempt != attempt {
		t.Fatalf("recovery decision = %+v", decision)
	}
	wantNotBefore := decision.Task.CreatedAt.Add(delay)
	if decision.Task.NotBefore == nil || !decision.Task.NotBefore.Equal(wantNotBefore) {
		t.Fatalf("recovery not_before = %v, want %v", decision.Task.NotBefore, wantNotBefore)
	}
	var payload runtimeRecoveryPayload
	if err := json.Unmarshal(decision.Task.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Recovery == nil || payload.Origin != "auto_recovery" ||
		payload.RecoveryEpisodeGeneration != payload.Recovery.EpisodeGeneration || payload.RecoveryAttempt != attempt ||
		payload.Recovery.EpisodeID != episode || payload.Recovery.Attempt != attempt {
		t.Fatalf("recovery payload = %+v", payload)
	}
}

func completeRecoveryTask(
	t *testing.T,
	ctx context.Context,
	database *Store,
	task *Task,
	succeeded bool,
) {
	t.Helper()
	if task == nil || task.NotBefore == nil {
		t.Fatal("recovery task is unavailable")
	}
	claimed, err := database.ClaimTask(ctx, ClaimTaskInput{
		Lane: TaskLaneRuntime, LeaseOwner: "recovery-test", Now: *task.NotBefore, LeaseDuration: time.Minute,
	})
	if err != nil || claimed == nil || claimed.ID != task.ID {
		t.Fatalf("ClaimTask(recovery) = %+v, %v", claimed, err)
	}
	completed, err := database.CompleteTask(
		ctx, claimed.ID, claimed.LeaseOwner, task.NotBefore.Add(time.Second),
		TaskCompletion{Succeeded: succeeded, Failure: json.RawMessage(`{"code":"test_failure"}`)},
	)
	if err != nil {
		t.Fatalf("CompleteTask(recovery) error = %v", err)
	}
	want := TaskStatusFailed
	if succeeded {
		want = TaskStatusSucceeded
	}
	if completed.Status != want {
		t.Fatalf("completed recovery status = %q, want %q", completed.Status, want)
	}
}
