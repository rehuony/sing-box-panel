package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestTaskClaimsAreSerialPerLaneAndIndependentAcrossLanes(t *testing.T) {
	ctx := testContext(t)
	store := openTestStore(t, ctx)
	now := time.Date(2026, time.August, 27, 8, 0, 0, 0, time.UTC)

	enqueueTask(t, ctx, store, EnqueueTaskInput{
		ID: "maintenance-1", Lane: TaskLaneMaintenance, Kind: TaskKindCoreInstall, CreatedAt: now,
	})
	enqueueTask(t, ctx, store, EnqueueTaskInput{
		ID: "maintenance-2", Lane: TaskLaneMaintenance, Kind: TaskKindCatalogRefresh, CreatedAt: now.Add(time.Second),
	})
	enqueueTask(t, ctx, store, EnqueueTaskInput{
		ID: "runtime-1", Lane: TaskLaneRuntime, Kind: TaskKindRuntimeApply, Generation: 1, CreatedAt: now,
	})

	maintenance, err := store.ClaimTask(ctx, ClaimTaskInput{
		Lane: TaskLaneMaintenance, LeaseOwner: "worker/maintenance", Now: now, LeaseDuration: time.Minute,
	})
	if err != nil {
		t.Fatalf("ClaimTask(maintenance) error = %v", err)
	}
	if maintenance == nil || maintenance.ID != "maintenance-1" || maintenance.Attempt != 1 {
		t.Fatalf("maintenance claim = %+v, want maintenance-1 attempt 1", maintenance)
	}

	blocked, err := store.ClaimTask(ctx, ClaimTaskInput{
		Lane: TaskLaneMaintenance, LeaseOwner: "other", Now: now, LeaseDuration: time.Minute,
	})
	if err != nil {
		t.Fatalf("second maintenance ClaimTask() error = %v", err)
	}
	if blocked != nil {
		t.Fatalf("second maintenance claim = %+v, want nil while lane is running", blocked)
	}

	runtimeTask, err := store.ClaimTask(ctx, ClaimTaskInput{
		Lane: TaskLaneRuntime, LeaseOwner: "worker/runtime", Now: now, LeaseDuration: time.Minute,
	})
	if err != nil {
		t.Fatalf("ClaimTask(runtime) error = %v", err)
	}
	if runtimeTask == nil || runtimeTask.ID != "runtime-1" {
		t.Fatalf("runtime claim = %+v, want runtime-1", runtimeTask)
	}

	if _, err := store.CompleteTask(
		ctx,
		maintenance.ID,
		maintenance.LeaseOwner,
		now.Add(time.Second),
		TaskCompletion{Succeeded: true, Result: json.RawMessage(`{"ok":true}`)},
	); err != nil {
		t.Fatalf("CompleteTask(maintenance-1) error = %v", err)
	}
	next, err := store.ClaimTask(ctx, ClaimTaskInput{
		Lane: TaskLaneMaintenance, LeaseOwner: "worker/maintenance", Now: now.Add(2 * time.Second), LeaseDuration: time.Minute,
	})
	if err != nil {
		t.Fatalf("ClaimTask(next maintenance) error = %v", err)
	}
	if next == nil || next.ID != "maintenance-2" {
		t.Fatalf("next maintenance claim = %+v, want maintenance-2", next)
	}
}

func TestTaskHeartbeatAndExpiredLeaseReclaim(t *testing.T) {
	ctx := testContext(t)
	store := openTestStore(t, ctx)
	now := time.Date(2026, time.August, 27, 9, 0, 0, 0, time.UTC)
	enqueueTask(t, ctx, store, EnqueueTaskInput{
		ID: "recoverable", Lane: TaskLaneMaintenance, Kind: TaskKindCoreInstall, CreatedAt: now,
	})

	claimed, err := store.ClaimTask(ctx, ClaimTaskInput{
		Lane: TaskLaneMaintenance, LeaseOwner: "worker-a", Now: now, LeaseDuration: 10 * time.Second,
	})
	if err != nil || claimed == nil {
		t.Fatalf("first ClaimTask() = %+v, %v", claimed, err)
	}
	lease, err := store.HeartbeatTask(
		ctx,
		claimed.ID,
		claimed.LeaseOwner,
		now.Add(5*time.Second),
		10*time.Second,
	)
	if err != nil {
		t.Fatalf("HeartbeatTask() error = %v", err)
	}
	if want := now.Add(15 * time.Second); !lease.LeaseExpiresAt.Equal(want) {
		t.Fatalf("heartbeat expiry = %s, want %s", lease.LeaseExpiresAt, want)
	}

	tooEarly, err := store.ClaimTask(ctx, ClaimTaskInput{
		Lane: TaskLaneMaintenance, LeaseOwner: "worker-b", Now: now.Add(11 * time.Second), LeaseDuration: time.Minute,
	})
	if err != nil {
		t.Fatalf("ClaimTask() before expiry error = %v", err)
	}
	if tooEarly != nil {
		t.Fatalf("claim before renewed lease expiry = %+v, want nil", tooEarly)
	}

	reclaimed, err := store.ClaimTask(ctx, ClaimTaskInput{
		Lane: TaskLaneMaintenance, LeaseOwner: "worker-b", Now: now.Add(16 * time.Second), LeaseDuration: time.Minute,
	})
	if err != nil {
		t.Fatalf("ClaimTask() after expiry error = %v", err)
	}
	if reclaimed == nil || reclaimed.ID != claimed.ID || reclaimed.Attempt != 2 {
		t.Fatalf("reclaimed task = %+v, want same task attempt 2", reclaimed)
	}

	_, err = store.CompleteTask(
		ctx,
		claimed.ID,
		claimed.LeaseOwner,
		now.Add(17*time.Second),
		TaskCompletion{Succeeded: true},
	)
	if !errors.Is(err, ErrTaskLeaseLost) {
		t.Fatalf("old owner CompleteTask() error = %v, want ErrTaskLeaseLost", err)
	}
	completed, err := store.CompleteTask(
		ctx,
		reclaimed.ID,
		reclaimed.LeaseOwner,
		now.Add(17*time.Second),
		TaskCompletion{Succeeded: true},
	)
	if err != nil {
		t.Fatalf("new owner CompleteTask() error = %v", err)
	}
	if completed.Status != TaskStatusSucceeded || completed.LeaseOwner != "" {
		t.Fatalf("completed reclaimed task = %+v, want succeeded without lease", completed)
	}
}

func TestTaskCancellationAndRuntimeGenerationSupersession(t *testing.T) {
	ctx := testContext(t)
	store := openTestStore(t, ctx)
	now := time.Date(2026, time.August, 27, 10, 0, 0, 0, time.UTC)

	enqueueTask(t, ctx, store, EnqueueTaskInput{
		ID: "queued-cancel", Lane: TaskLaneMaintenance, Kind: TaskKindCatalogRefresh, CreatedAt: now,
	})
	canceled, transitioned, err := store.RequestTaskCancellation(ctx, "queued-cancel", now.Add(time.Second))
	if err != nil {
		t.Fatalf("RequestTaskCancellation(queued) error = %v", err)
	}
	if canceled.Status != TaskStatusCanceled || !canceled.CancelRequested || !transitioned {
		t.Fatalf("queued canceled task = %+v, want canceled with request flag", canceled)
	}

	enqueueTask(t, ctx, store, EnqueueTaskInput{
		ID: "runtime-1", Lane: TaskLaneRuntime, Kind: TaskKindRuntimeApply, Generation: 1, CreatedAt: now,
	})
	first, err := store.ClaimTask(ctx, ClaimTaskInput{
		Lane: TaskLaneRuntime, LeaseOwner: "runtime-worker", Now: now, LeaseDuration: time.Minute,
	})
	if err != nil || first == nil {
		t.Fatalf("ClaimTask(runtime-1) = %+v, %v", first, err)
	}
	enqueueTask(t, ctx, store, EnqueueTaskInput{
		ID: "runtime-2", Lane: TaskLaneRuntime, Kind: TaskKindRuntimeApply, Generation: 2, CreatedAt: now.Add(time.Second),
	})

	lease, err := store.HeartbeatTask(
		ctx,
		first.ID,
		first.LeaseOwner,
		now.Add(2*time.Second),
		time.Minute,
	)
	if err != nil {
		t.Fatalf("HeartbeatTask(superseded) error = %v", err)
	}
	if !lease.CancelRequested || !lease.Superseded {
		t.Fatalf("superseded heartbeat = %+v, want both flags", lease)
	}
	firstCompleted, err := store.CompleteTask(
		ctx,
		first.ID,
		first.LeaseOwner,
		now.Add(3*time.Second),
		TaskCompletion{Succeeded: true, Result: json.RawMessage(`{"ignored":true}`)},
	)
	if err != nil {
		t.Fatalf("CompleteTask(superseded) error = %v", err)
	}
	if firstCompleted.Status != TaskStatusSuperseded || len(firstCompleted.Result) != 0 {
		t.Fatalf("superseded completion = %+v, want superseded without result", firstCompleted)
	}

	enqueueTask(t, ctx, store, EnqueueTaskInput{
		ID: "runtime-3", Lane: TaskLaneRuntime, Kind: TaskKindRuntimeApply, Generation: 3, CreatedAt: now.Add(4 * time.Second),
	})
	second, err := store.GetTask(ctx, "runtime-2")
	if err != nil {
		t.Fatalf("GetTask(runtime-2) error = %v", err)
	}
	if second.Status != TaskStatusSuperseded {
		t.Fatalf("runtime-2 status = %q, want superseded", second.Status)
	}

	third, err := store.ClaimTask(ctx, ClaimTaskInput{
		Lane: TaskLaneRuntime, LeaseOwner: "runtime-worker", Now: now.Add(5 * time.Second), LeaseDuration: time.Minute,
	})
	if err != nil || third == nil || third.ID != "runtime-3" {
		t.Fatalf("ClaimTask(runtime-3) = %+v, %v", third, err)
	}
	if _, _, err := store.RequestTaskCancellation(ctx, third.ID, now.Add(6*time.Second)); err != nil {
		t.Fatalf("RequestTaskCancellation(running) error = %v", err)
	}
	lease, err = store.HeartbeatTask(
		ctx,
		third.ID,
		third.LeaseOwner,
		now.Add(7*time.Second),
		time.Minute,
	)
	if err != nil || !lease.CancelRequested || lease.Superseded {
		t.Fatalf("canceled heartbeat = %+v, %v; want cancel only", lease, err)
	}
	thirdCompleted, err := store.CompleteTask(
		ctx,
		third.ID,
		third.LeaseOwner,
		now.Add(8*time.Second),
		TaskCompletion{Succeeded: true},
	)
	if err != nil {
		t.Fatalf("CompleteTask(canceled) error = %v", err)
	}
	if thirdCompleted.Status != TaskStatusCanceled {
		t.Fatalf("canceled completion status = %q, want canceled", thirdCompleted.Status)
	}

	_, err = store.EnqueueTask(ctx, EnqueueTaskInput{
		ID: "stale-generation", Lane: TaskLaneRuntime, Kind: TaskKindRuntimeApply, Generation: 2,
	})
	if !errors.Is(err, ErrTaskGenerationConflict) {
		t.Fatalf("stale runtime enqueue error = %v, want ErrTaskGenerationConflict", err)
	}
}

func TestExpiredRuntimeLeaseIsSupersededInsteadOfRetried(t *testing.T) {
	ctx := testContext(t)
	store := openTestStore(t, ctx)
	now := time.Date(2026, time.August, 27, 10, 30, 0, 0, time.UTC)
	enqueueTask(t, ctx, store, EnqueueTaskInput{
		ID: "old-runtime", Lane: TaskLaneRuntime, Kind: TaskKindRuntimeApply, Generation: 1, CreatedAt: now,
	})
	old, err := store.ClaimTask(ctx, ClaimTaskInput{
		Lane: TaskLaneRuntime, LeaseOwner: "crashed-worker", Now: now, LeaseDuration: 10 * time.Second,
	})
	if err != nil || old == nil {
		t.Fatalf("ClaimTask(old runtime) = %+v, %v", old, err)
	}
	enqueueTask(t, ctx, store, EnqueueTaskInput{
		ID: "new-runtime", Lane: TaskLaneRuntime, Kind: TaskKindRuntimeApply, Generation: 2, CreatedAt: now.Add(time.Second),
	})

	claimed, err := store.ClaimTask(ctx, ClaimTaskInput{
		Lane:          TaskLaneRuntime,
		LeaseOwner:    "replacement-worker",
		Now:           now.Add(11 * time.Second),
		LeaseDuration: time.Minute,
	})
	if err != nil {
		t.Fatalf("ClaimTask(new runtime) error = %v", err)
	}
	if claimed == nil || claimed.ID != "new-runtime" {
		t.Fatalf("claimed runtime = %+v, want new-runtime", claimed)
	}
	oldAfterReclaim, err := store.GetTask(ctx, "old-runtime")
	if err != nil {
		t.Fatalf("GetTask(old-runtime) error = %v", err)
	}
	if oldAfterReclaim.Status != TaskStatusSuperseded || oldAfterReclaim.Attempt != 1 {
		t.Fatalf("expired old runtime = %+v, want superseded without retry", oldAfterReclaim)
	}
}

func TestTaskClaimIsAtomicAcrossStores(t *testing.T) {
	ctx := testContext(t)
	path := filepath.Join(t.TempDir(), "panel.db")
	first, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("first Open() error = %v", err)
	}
	t.Cleanup(func() { _ = first.Close() })
	second, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("second Open() error = %v", err)
	}
	t.Cleanup(func() { _ = second.Close() })

	now := time.Date(2026, time.August, 27, 11, 0, 0, 0, time.UTC)
	enqueueTask(t, ctx, first, EnqueueTaskInput{
		ID: "contended", Lane: TaskLaneMaintenance, Kind: TaskKindCoreInstall, CreatedAt: now,
	})

	start := make(chan struct{})
	claimed := make([]*Task, 2)
	errs := make([]error, 2)
	var workers sync.WaitGroup
	for i, candidate := range []*Store{first, second} {
		workers.Add(1)
		go func(i int, candidate *Store) {
			defer workers.Done()
			<-start
			claimed[i], errs[i] = candidate.ClaimTask(ctx, ClaimTaskInput{
				Lane:          TaskLaneMaintenance,
				LeaseOwner:    string(rune('a' + i)),
				Now:           now,
				LeaseDuration: time.Minute,
			})
		}(i, candidate)
	}
	close(start)
	workers.Wait()

	claimCount := 0
	for i := range claimed {
		if errs[i] != nil {
			t.Fatalf("claimant %d error = %v", i, errs[i])
		}
		if claimed[i] != nil {
			claimCount++
		}
	}
	if claimCount != 1 {
		t.Fatalf("successful claims = %d, want 1", claimCount)
	}
}

func TestTerminalTasksDoNotConsumeIdempotencyKeys(t *testing.T) {
	ctx := testContext(t)
	taskStore := openTestStore(t, ctx)
	now := time.Date(2026, time.August, 29, 9, 0, 0, 0, time.UTC)

	for index, test := range []struct {
		name       string
		completion *TaskCompletion
	}{
		{name: "succeeded", completion: &TaskCompletion{Succeeded: true}},
		{name: "failed", completion: &TaskCompletion{Failure: json.RawMessage(`{"code":"expected"}`)}},
		{name: "canceled"},
	} {
		key := "terminal:" + test.name
		createdAt := now.Add(time.Duration(index) * time.Minute)
		first := enqueueTask(t, ctx, taskStore, EnqueueTaskInput{
			ID: "first-" + test.name, IdempotencyKey: key, Lane: TaskLaneMaintenance,
			Kind: TaskKindCatalogRefresh, Payload: json.RawMessage(`{"force":true}`), CreatedAt: createdAt,
		})
		if test.completion == nil {
			if _, _, err := taskStore.RequestTaskCancellation(ctx, first.ID, createdAt.Add(time.Second)); err != nil {
				t.Fatalf("%s RequestTaskCancellation() error = %v", test.name, err)
			}
		} else {
			claimed, err := taskStore.ClaimTask(ctx, ClaimTaskInput{
				Lane: TaskLaneMaintenance, LeaseOwner: "worker-" + test.name,
				Now: createdAt, LeaseDuration: time.Minute,
			})
			if err != nil || claimed == nil || claimed.ID != first.ID {
				t.Fatalf("%s ClaimTask() = %+v, %v", test.name, claimed, err)
			}
			if _, err := taskStore.CompleteTask(
				ctx, claimed.ID, claimed.LeaseOwner, createdAt.Add(time.Second), *test.completion,
			); err != nil {
				t.Fatalf("%s CompleteTask() error = %v", test.name, err)
			}
		}

		retry := enqueueTask(t, ctx, taskStore, EnqueueTaskInput{
			ID: "retry-" + test.name, IdempotencyKey: key, Lane: TaskLaneMaintenance,
			Kind: TaskKindCatalogRefresh, Payload: first.Payload, CreatedAt: createdAt.Add(2 * time.Second),
		})
		if retry.ID == first.ID {
			t.Fatalf("%s terminal task was returned instead of a new task", test.name)
		}
		if _, _, err := taskStore.RequestTaskCancellation(ctx, retry.ID, createdAt.Add(3*time.Second)); err != nil {
			t.Fatalf("cancel %s retry: %v", test.name, err)
		}
	}

	firstRuntime := enqueueTask(t, ctx, taskStore, EnqueueTaskInput{
		ID: "runtime-first", IdempotencyKey: "terminal:superseded", Lane: TaskLaneRuntime,
		Kind: TaskKindRuntimeStop, Generation: 1, CreatedAt: now.Add(10 * time.Minute),
	})
	enqueueTask(t, ctx, taskStore, EnqueueTaskInput{
		ID: "runtime-newer", IdempotencyKey: "runtime:newer", Lane: TaskLaneRuntime,
		Kind: TaskKindRuntimeStop, Generation: 2, CreatedAt: now.Add(10*time.Minute + time.Second),
	})
	preserved, err := taskStore.GetTask(ctx, firstRuntime.ID)
	if err != nil || preserved.Status != TaskStatusSuperseded {
		t.Fatalf("superseded task = %+v, %v", preserved, err)
	}
	runtimeRetry := enqueueTask(t, ctx, taskStore, EnqueueTaskInput{
		ID: "runtime-retry", IdempotencyKey: firstRuntime.IdempotencyKey, Lane: TaskLaneRuntime,
		Kind: firstRuntime.Kind, Generation: 3, Payload: firstRuntime.Payload,
		CreatedAt: now.Add(10*time.Minute + 2*time.Second),
	})
	if runtimeRetry.ID == firstRuntime.ID {
		t.Fatal("superseded runtime task consumed its idempotency key")
	}
}

func TestConcurrentIdempotentEnqueueCreatesOneActiveTask(t *testing.T) {
	ctx := testContext(t)
	path := filepath.Join(t.TempDir(), "panel.db")
	first, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("first Open() error = %v", err)
	}
	t.Cleanup(func() { _ = first.Close() })
	second, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("second Open() error = %v", err)
	}
	t.Cleanup(func() { _ = second.Close() })
	now := time.Date(2026, time.August, 29, 10, 0, 0, 0, time.UTC)

	start := make(chan struct{})
	results := make([]Task, 2)
	errs := make([]error, 2)
	var workers sync.WaitGroup
	for index, candidate := range []*Store{first, second} {
		workers.Add(1)
		go func(index int, candidate *Store) {
			defer workers.Done()
			<-start
			results[index], errs[index] = candidate.EnqueueTask(ctx, EnqueueTaskInput{
				ID: fmt.Sprintf("contender-%d", index), IdempotencyKey: "shared-active-key",
				Lane: TaskLaneMaintenance, Kind: TaskKindCatalogRefresh,
				Payload: json.RawMessage(`{"force":true}`), CreatedAt: now,
			})
		}(index, candidate)
	}
	close(start)
	workers.Wait()
	for index, err := range errs {
		if err != nil {
			t.Fatalf("contender %d EnqueueTask() error = %v", index, err)
		}
	}
	if results[0].ID != results[1].ID {
		t.Fatalf("concurrent results = %q and %q, want one active task", results[0].ID, results[1].ID)
	}
	page, err := first.ListTasks(ctx, TaskListFilter{Status: TaskStatusQueued, Limit: 10})
	if err != nil || len(page.Items) != 1 {
		t.Fatalf("active task page = %+v, %v; want one task", page, err)
	}
}

func enqueueTask(
	t *testing.T,
	ctx context.Context,
	store *Store,
	input EnqueueTaskInput,
) Task {
	t.Helper()
	queued, err := store.EnqueueTask(ctx, input)
	if err != nil {
		t.Fatalf("EnqueueTask(%q) error = %v", input.ID, err)
	}
	return queued
}
