// SPDX-License-Identifier: GPL-3.0-or-later

package store

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"
)

func TestCompleteTaskAtomicallyCommitsRuntimeEvidenceAndIntent(t *testing.T) {
	ctx := context.Background()
	database := openTestStore(t, ctx)
	initialized, err := database.LatestRuntimeTransition(ctx)
	if err != nil {
		t.Fatal(err)
	}
	now := initialized.OccurredAt.Add(time.Minute)
	oldBundle, oldObservation := seedAppliedRuntime(t, ctx, database, now)
	newBundle, err := database.SaveActivationBundle(ctx, ActivationBundle{
		ID:                "bundle-runtime-atomic",
		StartupArtifactID: oldBundle.StartupArtifactID,
		MonitoringTier:    MonitoringLimited,
		CreatedAt:         oldObservation.ObservedAt.Add(time.Second),
	})
	if err != nil {
		t.Fatal(err)
	}
	queued, err := database.RequestRuntimeIntent(ctx, RuntimeIntentInput{
		TaskID: "runtime-atomic-apply", Kind: RuntimeIntentApply,
		BundleID: newBundle.ID, CreatedAt: oldObservation.ObservedAt.Add(2 * time.Second),
	})
	if err != nil {
		t.Fatal(err)
	}
	claimed, err := database.ClaimTask(ctx, ClaimTaskInput{
		Lane: TaskLaneRuntime, LeaseOwner: "runtime-atomic-worker",
		Now: oldObservation.ObservedAt.Add(3 * time.Second), LeaseDuration: time.Minute,
	})
	if err != nil || claimed == nil || claimed.ID != queued.ID {
		t.Fatalf("ClaimTask(runtime atomic apply) = %+v, %v", claimed, err)
	}

	preliminary := oldObservation
	preliminary.PID++
	preliminary.ProcessStartToken = "process-runtime-atomic"
	preliminary.ActivationBundleID = newBundle.ID
	preliminary.StartedAt = oldObservation.ObservedAt.Add(4 * time.Second)
	preliminary.ObservedAt = preliminary.StartedAt.Add(time.Second)
	preliminary.StableObservedAt = nil
	if _, err := database.RecordRuntimeObservationAndTransitions(
		ctx, &oldObservation, preliminary, nil,
	); err != nil {
		t.Fatal(err)
	}
	finalObservation := preliminary
	finalObservation.ObservedAt = preliminary.ObservedAt.Add(time.Second)
	oldStartedAt, newStartedAt := oldObservation.StartedAt, finalObservation.StartedAt
	completion := TaskCompletion{
		Succeeded: true,
		Result:    json.RawMessage(`{"healthy":true}`),
		Runtime: &RuntimeTaskCommit{
			ExpectedObservation: &preliminary,
			Observation:         &finalObservation,
			Transitions: []RuntimeTransitionInput{
				{
					DedupeKey: "runtime-atomic-boundary", State: RuntimeTransitionStopped,
					Reason: "apply_boundary", ActivationBundleID: oldBundle.ID,
					Generation: claimed.Generation, TaskID: claimed.ID,
					PID: oldObservation.PID, ProcessStartToken: oldObservation.ProcessStartToken,
					ProcessStartedAt: &oldStartedAt, OccurredAt: finalObservation.StartedAt,
				},
				{
					DedupeKey: "runtime-atomic-running", State: RuntimeTransitionRunning,
					Reason: "apply_succeeded", ActivationBundleID: newBundle.ID,
					Generation: claimed.Generation, TaskID: claimed.ID,
					PID: finalObservation.PID, ProcessStartToken: finalObservation.ProcessStartToken,
					ProcessStartedAt: &newStartedAt, OccurredAt: finalObservation.ObservedAt,
				},
			},
		},
	}

	if _, err := database.db.ExecContext(ctx, `
		CREATE TRIGGER fail_runtime_atomic_transition
		BEFORE INSERT ON runtime_transitions
		WHEN NEW.task_id = 'runtime-atomic-apply'
		BEGIN
			SELECT RAISE(ABORT, 'injected runtime transition failure');
		END`); err != nil {
		t.Fatal(err)
	}
	completedAt := finalObservation.ObservedAt.Add(time.Second)
	if _, err := database.CompleteTask(
		ctx, claimed.ID, claimed.LeaseOwner, completedAt, completion,
	); err == nil {
		t.Fatal("CompleteTask() succeeded despite injected transition failure")
	}
	assertAtomicRuntimeCompletionRolledBack(
		t, ctx, database, *claimed, oldBundle.ID, preliminary,
	)

	if _, err := database.db.ExecContext(ctx, `DROP TRIGGER fail_runtime_atomic_transition`); err != nil {
		t.Fatal(err)
	}
	completed, err := database.CompleteTask(
		ctx, claimed.ID, claimed.LeaseOwner, completedAt, completion,
	)
	if err != nil || completed.Status != TaskStatusSucceeded {
		t.Fatalf("CompleteTask(retry) = %+v, %v", completed, err)
	}
	bootstrap, err := database.Bootstrap(ctx)
	if err != nil || bootstrap.Hub.AppliedBundleID != newBundle.ID {
		t.Fatalf("hub after runtime atomic completion = %+v, %v", bootstrap.Hub, err)
	}
	storedObservation, err := database.RuntimeObservation(ctx)
	if err != nil || !storedObservation.ObservedAt.Equal(finalObservation.ObservedAt) ||
		storedObservation.ActivationBundleID != newBundle.ID {
		t.Fatalf("final runtime observation = %+v, %v", storedObservation, err)
	}
	for _, reason := range []string{"apply_boundary", "apply_succeeded"} {
		page, err := database.ListRuntimeTransitions(ctx, RuntimeHistoryFilter{Reason: reason, Limit: 10})
		if err != nil || len(page.Items) != 1 || page.Items[0].TaskID != claimed.ID {
			t.Fatalf("runtime history %q = %+v, %v", reason, page, err)
		}
	}
}

func assertAtomicRuntimeCompletionRolledBack(
	t *testing.T,
	ctx context.Context,
	database *Store,
	claimed Task,
	appliedBundleID string,
	preliminary RuntimeObservation,
) {
	t.Helper()
	task, err := database.GetTask(ctx, claimed.ID)
	if err != nil || task.Status != TaskStatusRunning || task.LeaseOwner != claimed.LeaseOwner {
		t.Fatalf("task after rolled-back completion = %+v, %v", task, err)
	}
	bootstrap, err := database.Bootstrap(ctx)
	if err != nil || bootstrap.Hub.AppliedBundleID != appliedBundleID {
		t.Fatalf("hub after rolled-back completion = %+v, %v", bootstrap.Hub, err)
	}
	observation, err := database.RuntimeObservation(ctx)
	if err != nil || !observation.ObservedAt.Equal(preliminary.ObservedAt) ||
		observation.PID != preliminary.PID {
		t.Fatalf("observation after rolled-back completion = %+v, %v", observation, err)
	}
	page, err := database.ListRuntimeTransitions(ctx, RuntimeHistoryFilter{
		Reason: "apply_succeeded", Limit: 10,
	})
	if err != nil || len(page.Items) != 0 {
		t.Fatalf("history after rolled-back completion = %+v, %v", page, err)
	}
}

func TestCompleteTaskRejectsStaleRuntimeObservationFence(t *testing.T) {
	ctx := context.Background()
	database := openTestStore(t, ctx)
	initialized, err := database.LatestRuntimeTransition(ctx)
	if err != nil {
		t.Fatal(err)
	}
	_, observation := seedAppliedRuntime(t, ctx, database, initialized.OccurredAt.Add(time.Minute))
	queued, err := database.RequestRuntimeIntent(ctx, RuntimeIntentInput{
		TaskID: "runtime-stale-stop", Kind: RuntimeIntentStop,
		CreatedAt: observation.ObservedAt.Add(time.Second),
	})
	if err != nil {
		t.Fatal(err)
	}
	claimed, err := database.ClaimTask(ctx, ClaimTaskInput{
		Lane: TaskLaneRuntime, LeaseOwner: "runtime-stale-worker",
		Now: observation.ObservedAt.Add(2 * time.Second), LeaseDuration: time.Minute,
	})
	if err != nil || claimed == nil || claimed.ID != queued.ID {
		t.Fatalf("ClaimTask(runtime stale stop) = %+v, %v", claimed, err)
	}
	stale := observation
	newer := observation
	newer.PID++
	newer.ProcessStartToken = "newer-runtime-incarnation"
	newer.StartedAt = observation.ObservedAt.Add(3 * time.Second)
	newer.ObservedAt = newer.StartedAt.Add(time.Second)
	if _, err := database.RecordRuntimeObservation(ctx, newer); err != nil {
		t.Fatal(err)
	}
	startedAt := stale.StartedAt
	_, err = database.CompleteTask(
		ctx, claimed.ID, claimed.LeaseOwner, newer.ObservedAt.Add(time.Second),
		TaskCompletion{Succeeded: true, Runtime: &RuntimeTaskCommit{
			ExpectedObservation: &stale,
			ClearObservation:    true,
			Transitions: []RuntimeTransitionInput{{
				DedupeKey: "runtime-stale-stop", State: RuntimeTransitionStopped,
				Reason: "stop_succeeded", ActivationBundleID: stale.ActivationBundleID,
				Generation: claimed.Generation, TaskID: claimed.ID,
				PID: stale.PID, ProcessStartToken: stale.ProcessStartToken,
				ProcessStartedAt: &startedAt, OccurredAt: newer.ObservedAt,
			}},
		}},
	)
	if !errors.Is(err, ErrRuntimeIdentityMismatch) {
		t.Fatalf("CompleteTask(stale observation) error = %v", err)
	}
	stored, err := database.RuntimeObservation(ctx)
	if err != nil || stored.PID != newer.PID {
		t.Fatalf("newer runtime observation after stale completion = %+v, %v", stored, err)
	}
	task, err := database.GetTask(ctx, claimed.ID)
	if err != nil || task.Status != TaskStatusRunning {
		t.Fatalf("task after stale completion = %+v, %v", task, err)
	}
}

func TestCompleteTaskAtomicallyCommitsRuntimeEvidenceForTerminalOutcomes(t *testing.T) {
	tests := []struct {
		name      string
		state     RuntimeTransitionState
		want      TaskStatus
		interrupt func(context.Context, *testing.T, *Store, Task, time.Time)
	}{
		{name: "failed", state: RuntimeTransitionFailed, want: TaskStatusFailed},
		{
			name: "canceled", state: RuntimeTransitionStopped, want: TaskStatusCanceled,
			interrupt: func(ctx context.Context, t *testing.T, database *Store, task Task, at time.Time) {
				t.Helper()
				if _, _, err := database.RequestTaskCancellation(ctx, task.ID, at); err != nil {
					t.Fatalf("RequestTaskCancellation() error = %v", err)
				}
			},
		},
		{
			name: "superseded", state: RuntimeTransitionStopped, want: TaskStatusSuperseded,
			interrupt: func(ctx context.Context, t *testing.T, database *Store, _ Task, at time.Time) {
				t.Helper()
				if _, err := database.RequestRuntimeIntent(ctx, RuntimeIntentInput{
					TaskID: "runtime-after-superseded-stop", Kind: RuntimeIntentStart, CreatedAt: at,
				}); err != nil {
					t.Fatalf("RequestRuntimeIntent(newer start) error = %v", err)
				}
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			database, observation, claimed, completionAt := seedClaimedRuntimeStop(t, ctx, test.name)
			if test.interrupt != nil {
				test.interrupt(ctx, t, database, claimed, completionAt.Add(-time.Second))
			}
			completed, err := database.CompleteTask(
				ctx, claimed.ID, claimed.LeaseOwner, completionAt,
				TaskCompletion{
					Failure: json.RawMessage(`{"code":"runtime_terminal_test"}`),
					Runtime: failedRuntimeTaskCommit(
						claimed, observation, test.state, "runtime_"+test.name, completionAt,
					),
				},
			)
			if err != nil || completed.Status != test.want {
				t.Fatalf("CompleteTask(%s) = %+v, %v", test.name, completed, err)
			}
			if _, err := database.RuntimeObservation(ctx); !errors.Is(err, ErrRuntimeObservationNotFound) {
				t.Fatalf("runtime observation after %s completion = %v", test.name, err)
			}
			history, err := database.ListRuntimeTransitions(ctx, RuntimeHistoryFilter{
				Reason: "runtime_" + test.name, Limit: 10,
			})
			if err != nil || len(history.Items) != 1 || history.Items[0].TaskID != claimed.ID ||
				history.Items[0].Generation != claimed.Generation || history.Items[0].State != test.state {
				t.Fatalf("runtime %s history = %+v, %v", test.name, history, err)
			}
		})
	}
}

func TestCompleteTaskRollsBackRuntimeEvidenceWhenTaskWriteFails(t *testing.T) {
	ctx := context.Background()
	database, observation, claimed, completionAt := seedClaimedRuntimeStop(t, ctx, "task-write-failure")
	commit := failedRuntimeTaskCommit(
		claimed, observation, RuntimeTransitionFailed, "runtime_task_write_failure", completionAt,
	)
	if _, err := database.db.ExecContext(ctx, `
		CREATE TRIGGER fail_runtime_task_terminal
		BEFORE UPDATE OF status ON tasks
		WHEN OLD.id = 'runtime-task-write-failure' AND NEW.status = 'failed'
		BEGIN
			SELECT RAISE(ABORT, 'injected task terminal failure');
		END`); err != nil {
		t.Fatal(err)
	}
	completion := TaskCompletion{
		Failure: json.RawMessage(`{"code":"runtime_task_write_failure"}`), Runtime: commit,
	}
	if _, err := database.CompleteTask(
		ctx, claimed.ID, claimed.LeaseOwner, completionAt, completion,
	); err == nil {
		t.Fatal("CompleteTask() succeeded despite injected task update failure")
	}
	assertFailedRuntimeCompletionRolledBack(t, ctx, database, claimed, observation, "runtime_task_write_failure")

	if _, err := database.db.ExecContext(ctx, `DROP TRIGGER fail_runtime_task_terminal`); err != nil {
		t.Fatal(err)
	}
	completed, err := database.CompleteTask(
		ctx, claimed.ID, claimed.LeaseOwner, completionAt, completion,
	)
	if err != nil || completed.Status != TaskStatusFailed {
		t.Fatalf("CompleteTask(retry) = %+v, %v", completed, err)
	}
	if _, err := database.RuntimeObservation(ctx); !errors.Is(err, ErrRuntimeObservationNotFound) {
		t.Fatalf("runtime observation after retry = %v", err)
	}
}

func TestCompleteTaskRejectsRuntimeCommitOwnershipFences(t *testing.T) {
	tests := []struct {
		name   string
		reason string
		mutate func(*Task, *RuntimeTaskCommit, *time.Time)
		lease  func(Task) string
		want   error
	}{
		{
			name:   "lease owner",
			reason: "runtime_fence_lease_owner",
			lease:  func(Task) string { return "not-the-lease-owner" },
			want:   ErrTaskLeaseLost,
		},
		{
			name:   "expired lease",
			reason: "runtime_fence_expired_lease",
			mutate: func(task *Task, _ *RuntimeTaskCommit, completedAt *time.Time) {
				*completedAt = task.LeaseExpiresAt.Add(time.Second)
			},
			want: ErrTaskLeaseLost,
		},
		{
			name:   "transition task ID",
			reason: "runtime_fence_task_id",
			mutate: func(_ *Task, commit *RuntimeTaskCommit, _ *time.Time) {
				commit.Transitions[0].TaskID = "different-runtime-task"
			},
		},
		{
			name:   "transition generation",
			reason: "runtime_fence_generation",
			mutate: func(_ *Task, commit *RuntimeTaskCommit, _ *time.Time) {
				commit.Transitions[0].Generation++
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			database, observation, claimed, completionAt := seedClaimedRuntimeStop(t, ctx, test.reason)
			commit := failedRuntimeTaskCommit(
				claimed, observation, RuntimeTransitionFailed, test.reason, completionAt,
			)
			if test.mutate != nil {
				test.mutate(&claimed, commit, &completionAt)
			}
			leaseOwner := claimed.LeaseOwner
			if test.lease != nil {
				leaseOwner = test.lease(claimed)
			}
			_, err := database.CompleteTask(
				ctx, claimed.ID, leaseOwner, completionAt,
				TaskCompletion{Failure: json.RawMessage(`{"code":"fence_test"}`), Runtime: commit},
			)
			if err == nil || test.want != nil && !errors.Is(err, test.want) {
				t.Fatalf("CompleteTask(%s) error = %v, want %v", test.name, err, test.want)
			}
			assertFailedRuntimeCompletionRolledBack(
				t, ctx, database, claimed, observation, test.reason,
			)
		})
	}
}

func TestCompleteTaskRejectsRuntimeCommitForMaintenanceLane(t *testing.T) {
	ctx := context.Background()
	database := openTestStore(t, ctx)
	initialized, err := database.LatestRuntimeTransition(ctx)
	if err != nil {
		t.Fatal(err)
	}
	now := initialized.OccurredAt.Add(time.Minute)
	_, observation := seedAppliedRuntime(t, ctx, database, now)
	claimed, err := database.ClaimTask(ctx, ClaimTaskInput{
		Lane: TaskLaneMaintenance, LeaseOwner: "maintenance-worker",
		Now: observation.ObservedAt.Add(2 * time.Second), LeaseDuration: time.Minute,
	})
	if err != nil || claimed == nil || claimed.Lane != TaskLaneMaintenance {
		t.Fatalf("ClaimTask(maintenance) = %+v, %v", claimed, err)
	}
	completionAt := observation.ObservedAt.Add(3 * time.Second)
	_, err = database.CompleteTask(
		ctx, claimed.ID, claimed.LeaseOwner, completionAt,
		TaskCompletion{
			Failure: json.RawMessage(`{"code":"wrong_lane"}`),
			Runtime: failedRuntimeTaskCommit(
				*claimed, observation, RuntimeTransitionFailed, "wrong_lane", completionAt,
			),
		},
	)
	if err == nil {
		t.Fatal("CompleteTask() accepted runtime evidence for a maintenance task")
	}
	assertFailedRuntimeCompletionRolledBack(t, ctx, database, *claimed, observation, "wrong_lane")
}

func seedClaimedRuntimeStop(
	t *testing.T,
	ctx context.Context,
	suffix string,
) (*Store, RuntimeObservation, Task, time.Time) {
	t.Helper()
	database := openTestStore(t, ctx)
	initialized, err := database.LatestRuntimeTransition(ctx)
	if err != nil {
		t.Fatal(err)
	}
	_, observation := seedAppliedRuntime(t, ctx, database, initialized.OccurredAt.Add(time.Minute))
	queued, err := database.RequestRuntimeIntent(ctx, RuntimeIntentInput{
		TaskID: "runtime-" + suffix, Kind: RuntimeIntentStop,
		CreatedAt: observation.ObservedAt.Add(time.Second),
	})
	if err != nil {
		t.Fatal(err)
	}
	claimed, err := database.ClaimTask(ctx, ClaimTaskInput{
		Lane: TaskLaneRuntime, LeaseOwner: "runtime-" + suffix + "-worker",
		Now: observation.ObservedAt.Add(2 * time.Second), LeaseDuration: time.Minute,
	})
	if err != nil || claimed == nil || claimed.ID != queued.ID {
		t.Fatalf("ClaimTask(runtime stop) = %+v, %v", claimed, err)
	}
	return database, observation, *claimed, observation.ObservedAt.Add(4 * time.Second)
}

func failedRuntimeTaskCommit(
	task Task,
	observation RuntimeObservation,
	state RuntimeTransitionState,
	reason string,
	occurredAt time.Time,
) *RuntimeTaskCommit {
	startedAt := observation.StartedAt
	return &RuntimeTaskCommit{
		ExpectedObservation: &observation,
		ClearObservation:    true,
		Transitions: []RuntimeTransitionInput{{
			DedupeKey: reason, State: state, Reason: reason,
			ActivationBundleID: observation.ActivationBundleID,
			Generation:         task.Generation, TaskID: task.ID,
			PID: observation.PID, ProcessStartToken: observation.ProcessStartToken,
			ProcessStartedAt: &startedAt, OccurredAt: occurredAt,
		}},
	}
}

func assertFailedRuntimeCompletionRolledBack(
	t *testing.T,
	ctx context.Context,
	database *Store,
	claimed Task,
	observation RuntimeObservation,
	reason string,
) {
	t.Helper()
	task, err := database.GetTask(ctx, claimed.ID)
	if err != nil || task.Status != TaskStatusRunning || task.LeaseOwner != claimed.LeaseOwner {
		t.Fatalf("task after rejected completion = %+v, %v", task, err)
	}
	stored, err := database.RuntimeObservation(ctx)
	if err != nil || stored.PID != observation.PID || stored.ProcessStartToken != observation.ProcessStartToken {
		t.Fatalf("observation after rejected completion = %+v, %v", stored, err)
	}
	history, err := database.ListRuntimeTransitions(ctx, RuntimeHistoryFilter{Reason: reason, Limit: 10})
	if err != nil || len(history.Items) != 0 {
		t.Fatalf("history after rejected completion = %+v, %v", history, err)
	}
}
