package taskrunner

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/rehuony/sing-box-panel/internal/store"
)

func TestRunnerProcessesLanesIndependentlyAndSerially(t *testing.T) {
	ctx := runnerTestContext(t)
	taskStore := openRunnerStore(t, ctx)
	now := time.Date(2026, time.August, 27, 12, 0, 0, 0, time.UTC)
	clock := newFakeClock(now)

	enqueueRunnerTask(t, ctx, taskStore, store.EnqueueTaskInput{
		ID: "maintenance-1", Lane: store.TaskLaneMaintenance, Kind: "maintenance", CreatedAt: now,
	})
	enqueueRunnerTask(t, ctx, taskStore, store.EnqueueTaskInput{
		ID: "maintenance-2", Lane: store.TaskLaneMaintenance, Kind: "maintenance", CreatedAt: now.Add(time.Second),
	})
	enqueueRunnerTask(t, ctx, taskStore, store.EnqueueTaskInput{
		ID: "runtime-1", Lane: store.TaskLaneRuntime, Kind: "runtime", Generation: 1, CreatedAt: now,
	})

	started := make(chan string, 3)
	releases := map[string]chan struct{}{
		"maintenance-1": make(chan struct{}),
		"maintenance-2": make(chan struct{}),
		"runtime-1":     make(chan struct{}),
	}
	handler := HandlerFunc(func(
		ctx context.Context,
		task store.Task,
		_ Control,
	) (json.RawMessage, error) {
		started <- task.ID
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-releases[task.ID]:
			return json.RawMessage(`{"done":true}`), nil
		}
	})
	runner := newTestRunner(t, taskStore, clock, map[string]Handler{
		"maintenance": handler,
		"runtime":     handler,
	})
	if err := runner.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	firstTwo := map[string]bool{}
	firstTwo[receiveString(t, ctx, started)] = true
	firstTwo[receiveString(t, ctx, started)] = true
	if !firstTwo["maintenance-1"] || !firstTwo["runtime-1"] {
		t.Fatalf("initial handlers = %v, want maintenance-1 and runtime-1", firstTwo)
	}
	select {
	case unexpected := <-started:
		t.Fatalf("same-lane handler %q started while maintenance-1 was running", unexpected)
	default:
	}

	close(releases["runtime-1"])
	close(releases["maintenance-1"])
	if next := receiveString(t, ctx, started); next != "maintenance-2" {
		t.Fatalf("next handler = %q, want maintenance-2", next)
	}
	close(releases["maintenance-2"])

	waitTaskStatus(t, ctx, taskStore, "maintenance-1", store.TaskStatusSucceeded)
	waitTaskStatus(t, ctx, taskStore, "maintenance-2", store.TaskStatusSucceeded)
	waitTaskStatus(t, ctx, taskStore, "runtime-1", store.TaskStatusSucceeded)
	runner.Close()
	if err := runner.Wait(); err != nil {
		t.Fatalf("Wait() error = %v", err)
	}
}

func TestRunnerCancellationOccursAtExplicitSafePoint(t *testing.T) {
	ctx := runnerTestContext(t)
	taskStore := openRunnerStore(t, ctx)
	now := time.Date(2026, time.August, 27, 13, 0, 0, 0, time.UTC)
	clock := newFakeClock(now)
	enqueueRunnerTask(t, ctx, taskStore, store.EnqueueTaskInput{
		ID: "cancel-at-safe-point", Lane: store.TaskLaneMaintenance, Kind: "cancellable", CreatedAt: now,
	})

	started := make(chan struct{})
	checkNow := make(chan struct{})
	checkpointError := make(chan error, 1)
	runner := newTestRunner(t, taskStore, clock, map[string]Handler{
		"cancellable": HandlerFunc(func(
			ctx context.Context,
			_ store.Task,
			control Control,
		) (json.RawMessage, error) {
			close(started)
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-checkNow:
			}
			err := control.SafePoint(ctx)
			checkpointError <- err
			return nil, err
		}),
	})
	if err := runner.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	waitSignal(t, ctx, started)

	clock.Advance(time.Second)
	if _, err := taskStore.RequestTaskCancellation(ctx, "cancel-at-safe-point", clock.Now()); err != nil {
		t.Fatalf("RequestTaskCancellation() error = %v", err)
	}
	close(checkNow)
	select {
	case err := <-checkpointError:
		if !errors.Is(err, ErrCancellationRequested) {
			t.Fatalf("SafePoint() error = %v, want ErrCancellationRequested", err)
		}
	case <-ctx.Done():
		t.Fatalf("waiting for SafePoint(): %v", ctx.Err())
	}

	waitTaskStatus(t, ctx, taskStore, "cancel-at-safe-point", store.TaskStatusCanceled)
	runner.Close()
	if err := runner.Wait(); err != nil {
		t.Fatalf("Wait() error = %v", err)
	}
}

func TestRunnerHeartbeatPreventsPrematureReclaim(t *testing.T) {
	ctx := runnerTestContext(t)
	taskStore := openRunnerStore(t, ctx)
	now := time.Date(2026, time.August, 27, 14, 0, 0, 0, time.UTC)
	clock := newFakeClock(now)
	enqueueRunnerTask(t, ctx, taskStore, store.EnqueueTaskInput{
		ID: "heartbeat", Lane: store.TaskLaneMaintenance, Kind: "blocking", CreatedAt: now,
	})

	started := make(chan struct{})
	release := make(chan struct{})
	runner := newTestRunner(t, taskStore, clock, map[string]Handler{
		"blocking": HandlerFunc(func(
			ctx context.Context,
			_ store.Task,
			_ Control,
		) (json.RawMessage, error) {
			close(started)
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-release:
				return nil, nil
			}
		}),
	})
	if err := runner.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	waitSignal(t, ctx, started)

	clock.Advance(10 * time.Second)
	wantExpiry := now.Add(40 * time.Second)
	waitTaskLease(t, ctx, taskStore, "heartbeat", wantExpiry)
	claimed, err := taskStore.ClaimTask(ctx, store.ClaimTaskInput{
		Lane:          store.TaskLaneMaintenance,
		LeaseOwner:    "other-worker",
		Now:           now.Add(31 * time.Second),
		LeaseDuration: time.Minute,
	})
	if err != nil {
		t.Fatalf("competing ClaimTask() error = %v", err)
	}
	if claimed != nil {
		t.Fatalf("competing claim = %+v, want nil while heartbeat lease is valid", claimed)
	}

	close(release)
	waitTaskStatus(t, ctx, taskStore, "heartbeat", store.TaskStatusSucceeded)
	runner.Close()
	if err := runner.Wait(); err != nil {
		t.Fatalf("Wait() error = %v", err)
	}
}

func TestRunnerShutdownLeavesRunningTaskForCrashReclaim(t *testing.T) {
	ctx := runnerTestContext(t)
	taskStore := openRunnerStore(t, ctx)
	now := time.Date(2026, time.August, 27, 15, 0, 0, 0, time.UTC)
	clock := newFakeClock(now)
	enqueueRunnerTask(t, ctx, taskStore, store.EnqueueTaskInput{
		ID: "shutdown", Lane: store.TaskLaneMaintenance, Kind: "until-shutdown", CreatedAt: now,
	})

	started := make(chan struct{})
	handlerStopped := make(chan struct{})
	runner := newTestRunner(t, taskStore, clock, map[string]Handler{
		"until-shutdown": HandlerFunc(func(
			ctx context.Context,
			_ store.Task,
			_ Control,
		) (json.RawMessage, error) {
			close(started)
			<-ctx.Done()
			close(handlerStopped)
			return nil, ctx.Err()
		}),
	})
	if err := runner.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	waitSignal(t, ctx, started)
	runner.Close()
	waitSignal(t, ctx, handlerStopped)
	if err := runner.Wait(); err != nil {
		t.Fatalf("Wait() error = %v", err)
	}

	unfinished, err := taskStore.GetTask(ctx, "shutdown")
	if err != nil {
		t.Fatalf("GetTask(shutdown) error = %v", err)
	}
	if unfinished.Status != store.TaskStatusRunning {
		t.Fatalf("shutdown task status = %q, want running until lease reclaim", unfinished.Status)
	}

	clock.Advance(31 * time.Second)
	reclaimed, err := taskStore.ClaimTask(ctx, store.ClaimTaskInput{
		Lane:          store.TaskLaneMaintenance,
		LeaseOwner:    "replacement/maintenance",
		Now:           clock.Now(),
		LeaseDuration: time.Minute,
	})
	if err != nil {
		t.Fatalf("ClaimTask() after shutdown lease expiry error = %v", err)
	}
	if reclaimed == nil || reclaimed.ID != "shutdown" || reclaimed.Attempt != 2 {
		t.Fatalf("reclaimed shutdown task = %+v, want shutdown attempt 2", reclaimed)
	}
}

func newTestRunner(
	t *testing.T,
	taskStore TaskStore,
	clock Clock,
	handlers map[string]Handler,
) *Runner {
	t.Helper()
	runner, err := New(taskStore, handlers, Options{
		WorkerID:          "test-worker",
		LeaseDuration:     30 * time.Second,
		HeartbeatInterval: 10 * time.Second,
		PollInterval:      time.Second,
		Clock:             clock,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	t.Cleanup(func() {
		runner.Close()
		_ = runner.Wait()
	})
	return runner
}

func openRunnerStore(t *testing.T, ctx context.Context) *store.Store {
	t.Helper()
	taskStore, err := store.Open(ctx, filepath.Join(t.TempDir(), "panel.db"))
	if err != nil {
		t.Fatalf("store.Open() error = %v", err)
	}
	t.Cleanup(func() { _ = taskStore.Close() })
	return taskStore
}

func enqueueRunnerTask(
	t *testing.T,
	ctx context.Context,
	taskStore *store.Store,
	input store.EnqueueTaskInput,
) {
	t.Helper()
	if _, err := taskStore.EnqueueTask(ctx, input); err != nil {
		t.Fatalf("EnqueueTask(%q) error = %v", input.ID, err)
	}
}

func runnerTestContext(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	return ctx
}

func receiveString(t *testing.T, ctx context.Context, values <-chan string) string {
	t.Helper()
	select {
	case value := <-values:
		return value
	case <-ctx.Done():
		t.Fatalf("waiting for value: %v", ctx.Err())
		return ""
	}
}

func waitSignal(t *testing.T, ctx context.Context, signal <-chan struct{}) {
	t.Helper()
	select {
	case <-signal:
	case <-ctx.Done():
		t.Fatalf("waiting for signal: %v", ctx.Err())
	}
}

func waitTaskStatus(
	t *testing.T,
	ctx context.Context,
	taskStore *store.Store,
	taskID string,
	want store.TaskStatus,
) {
	t.Helper()
	for {
		task, err := taskStore.GetTask(ctx, taskID)
		if err != nil {
			t.Fatalf("GetTask(%q) error = %v", taskID, err)
		}
		if task.Status == want {
			return
		}
		select {
		case <-ctx.Done():
			t.Fatalf("task %q status = %q, want %q: %v", taskID, task.Status, want, ctx.Err())
		default:
			runtime.Gosched()
		}
	}
}

func waitTaskLease(
	t *testing.T,
	ctx context.Context,
	taskStore *store.Store,
	taskID string,
	want time.Time,
) {
	t.Helper()
	for {
		task, err := taskStore.GetTask(ctx, taskID)
		if err != nil {
			t.Fatalf("GetTask(%q) error = %v", taskID, err)
		}
		if task.LeaseExpiresAt != nil && task.LeaseExpiresAt.Equal(want) {
			return
		}
		select {
		case <-ctx.Done():
			t.Fatalf("task %q lease = %v, want %v: %v", taskID, task.LeaseExpiresAt, want, ctx.Err())
		default:
			runtime.Gosched()
		}
	}
}

type fakeClock struct {
	mu      sync.Mutex
	now     time.Time
	tickers map[*fakeTicker]struct{}
}

func newFakeClock(now time.Time) *fakeClock {
	return &fakeClock{now: now.UTC(), tickers: make(map[*fakeTicker]struct{})}
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) NewTicker(interval time.Duration) Ticker {
	c.mu.Lock()
	defer c.mu.Unlock()
	ticker := &fakeTicker{
		clock:    c,
		interval: interval,
		next:     c.now.Add(interval),
		values:   make(chan time.Time, 1),
	}
	c.tickers[ticker] = struct{}{}
	return ticker
}

func (c *fakeClock) Advance(delta time.Duration) {
	c.mu.Lock()
	c.now = c.now.Add(delta)
	for ticker := range c.tickers {
		if ticker.stopped || c.now.Before(ticker.next) {
			continue
		}
		select {
		case ticker.values <- c.now:
		default:
		}
		for !ticker.next.After(c.now) {
			ticker.next = ticker.next.Add(ticker.interval)
		}
	}
	c.mu.Unlock()
}

type fakeTicker struct {
	clock    *fakeClock
	interval time.Duration
	next     time.Time
	values   chan time.Time
	stopped  bool
}

func (t *fakeTicker) C() <-chan time.Time {
	return t.values
}

func (t *fakeTicker) Stop() {
	t.clock.mu.Lock()
	defer t.clock.mu.Unlock()
	if t.stopped {
		return
	}
	t.stopped = true
	delete(t.clock.tickers, t)
}
