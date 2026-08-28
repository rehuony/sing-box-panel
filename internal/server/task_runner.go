package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/rehuony/sing-box-panel/internal/store"
)

var (
	errTaskRunnerAlreadyStarted = errors.New("task runner already started")
	errTaskRunnerClosed         = errors.New("task runner is closed")
	errTaskRunnerNotStarted     = errors.New("task runner is not started")
)

// taskHandler performs one claimed task. Calls are serial within a lane and may
// overlap across the runtime and maintenance lanes. Handlers must honor ctx for
// lease loss and process shutdown, and call control.SafePoint at safe external
// cancellation boundaries.
type taskHandler interface {
	Handle(context.Context, store.Task, taskExecutionControl) (json.RawMessage, error)
}

// taskHandlerFunc adapts a function to taskHandler.
type taskHandlerFunc func(context.Context, store.Task, taskExecutionControl) (json.RawMessage, error)

func (f taskHandlerFunc) Handle(
	ctx context.Context,
	task store.Task,
	control taskExecutionControl,
) (json.RawMessage, error) {
	return f(ctx, task, control)
}

// taskStore is the durable boundary the composition root supplies to taskRunner.
type taskStore interface {
	ClaimTask(context.Context, store.ClaimTaskInput) (*store.Task, error)
	HeartbeatTask(
		context.Context,
		string,
		string,
		time.Time,
		time.Duration,
	) (store.TaskLeaseState, error)
	CompleteTask(
		context.Context,
		string,
		string,
		time.Time,
		store.TaskCompletion,
	) (store.Task, error)
}

// taskRunnerOptions defines the worker identity and bounded polling/lease policy.
type taskRunnerOptions struct {
	WorkerID          string
	LeaseDuration     time.Duration
	HeartbeatInterval time.Duration
	PollInterval      time.Duration
	taskClock         taskClock
}

// taskRunner owns exactly one goroutine for each durable lane. A short-lived
// heartbeat goroutine is additionally owned while a handler is running.
type taskRunner struct {
	store    taskStore
	handlers map[string]taskHandler
	options  taskRunnerOptions
	wake     map[store.TaskLane]chan struct{}

	mu      sync.Mutex
	started bool
	closed  bool
	cancel  context.CancelFunc
	runErr  error
	wg      sync.WaitGroup
}

// New constructs a stopped runner. taskHandler registration is immutable after
// construction so lane goroutines never race on the map.
func newTaskRunner(taskStore taskStore, handlers map[string]taskHandler, options taskRunnerOptions) (*taskRunner, error) {
	if taskStore == nil {
		return nil, errors.New("task store is nil")
	}
	if strings.TrimSpace(options.WorkerID) == "" {
		return nil, errors.New("task runner worker id is empty")
	}
	if options.LeaseDuration == 0 {
		options.LeaseDuration = 30 * time.Second
	}
	if options.HeartbeatInterval == 0 {
		options.HeartbeatInterval = 10 * time.Second
	}
	if options.PollInterval == 0 {
		options.PollInterval = time.Second
	}
	if options.LeaseDuration <= 0 || options.HeartbeatInterval <= 0 || options.PollInterval <= 0 {
		return nil, errors.New("task runner durations must be positive")
	}
	if options.HeartbeatInterval >= options.LeaseDuration {
		return nil, errors.New("heartbeat interval must be shorter than lease duration")
	}
	if options.taskClock == nil {
		options.taskClock = systemClock{}
	}

	registered := make(map[string]taskHandler, len(handlers))
	for kind, handler := range handlers {
		if strings.TrimSpace(kind) == "" {
			return nil, errors.New("task handler kind is empty")
		}
		if handler == nil {
			return nil, fmt.Errorf("task handler %q is nil", kind)
		}
		registered[kind] = handler
	}

	return &taskRunner{
		store:    taskStore,
		handlers: registered,
		options:  options,
		wake: map[store.TaskLane]chan struct{}{
			store.TaskLaneRuntime:     make(chan struct{}, 1),
			store.TaskLaneMaintenance: make(chan struct{}, 1),
		},
	}, nil
}

// Start transfers goroutine ownership to the runner until Close or parent
// cancellation. A taskRunner is intentionally single-use.
func (r *taskRunner) Start(parent context.Context) error {
	if parent == nil {
		return errors.New("task runner parent context is nil")
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if r.started {
		return errTaskRunnerAlreadyStarted
	}
	if r.closed {
		return errTaskRunnerClosed
	}

	ctx, cancel := context.WithCancel(parent)
	r.started = true
	r.cancel = cancel
	for _, lane := range []store.TaskLane{store.TaskLaneRuntime, store.TaskLaneMaintenance} {
		r.wg.Add(1)
		go r.runLane(ctx, lane)
	}
	return nil
}

// Wake asks a lane to poll immediately. The signal is coalesced; durable tasks
// remain in SQLite, and periodic polling covers tasks enqueued by another CLI
// process that cannot call Wake.
func (r *taskRunner) Wake(lane store.TaskLane) error {
	wake, ok := r.wake[lane]
	if !ok {
		return fmt.Errorf("invalid task lane %q", lane)
	}
	select {
	case wake <- struct{}{}:
	default:
	}
	return nil
}

// Close requests component shutdown. It does not mark a running durable task
// terminal; unfinished work is reclaimed after its lease expires.
func (r *taskRunner) Close() {
	r.mu.Lock()
	r.closed = true
	cancel := r.cancel
	r.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

// Wait waits for both lane owners and any current handler heartbeat to stop.
func (r *taskRunner) Wait() error {
	r.mu.Lock()
	started := r.started
	r.mu.Unlock()
	if !started {
		return errTaskRunnerNotStarted
	}
	r.wg.Wait()
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.runErr
}

func (r *taskRunner) runLane(ctx context.Context, lane store.TaskLane) {
	defer r.wg.Done()
	ticker := r.options.taskClock.NewTicker(r.options.PollInterval)
	defer ticker.Stop()

	for {
		if err := ctx.Err(); err != nil {
			return
		}

		task, err := r.store.ClaimTask(ctx, store.ClaimTaskInput{
			Lane:          lane,
			LeaseOwner:    r.options.WorkerID + "/" + string(lane),
			Now:           r.options.taskClock.Now(),
			LeaseDuration: r.options.LeaseDuration,
		})
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			r.fail(fmt.Errorf("claim %s task: %w", lane, err))
			return
		}
		if task != nil {
			if err := r.runTask(ctx, *task); err != nil {
				if ctx.Err() != nil {
					return
				}
				r.fail(fmt.Errorf("run task %q: %w", task.ID, err))
				return
			}
			continue
		}

		select {
		case <-ctx.Done():
			return
		case <-r.wake[lane]:
		case <-ticker.C():
		}
	}
}

func (r *taskRunner) runTask(ctx context.Context, task store.Task) error {
	handler, ok := r.handlers[task.Kind]
	if !ok {
		failure := encodeTaskFailure("handler_not_registered", fmt.Errorf("no handler for kind %q", task.Kind))
		_, err := r.store.CompleteTask(
			ctx,
			task.ID,
			task.LeaseOwner,
			r.options.taskClock.Now(),
			store.TaskCompletion{Failure: failure},
		)
		if errors.Is(err, store.ErrTaskLeaseLost) {
			return nil
		}
		return err
	}

	handlerCtx, cancelHandler := context.WithCancelCause(ctx)
	defer cancelHandler(context.Canceled)
	control := &taskControl{
		store:         r.store,
		taskID:        task.ID,
		leaseOwner:    task.LeaseOwner,
		leaseDuration: r.options.LeaseDuration,
		clock:         r.options.taskClock,
	}

	heartbeatCtx, stopHeartbeat := context.WithCancel(ctx)
	heartbeatDone := make(chan struct{})
	heartbeatTicker := r.options.taskClock.NewTicker(r.options.HeartbeatInterval)
	go func() {
		defer close(heartbeatDone)
		defer heartbeatTicker.Stop()
		for {
			select {
			case <-heartbeatCtx.Done():
				return
			case <-heartbeatTicker.C():
				if _, err := control.heartbeat(handlerCtx); err != nil {
					cancelHandler(err)
					return
				}
			}
		}
	}()

	result, handlerErr := handler.Handle(handlerCtx, task, control)
	stopHeartbeat()
	<-heartbeatDone

	if ctx.Err() != nil {
		// Shutdown leaves the lease to expire; a new process can reclaim it.
		return nil
	}
	if cause := context.Cause(handlerCtx); cause != nil {
		if errors.Is(cause, store.ErrTaskLeaseLost) {
			return nil
		}
		return cause
	}

	completion := store.TaskCompletion{Succeeded: handlerErr == nil, Result: result}
	if handlerErr != nil {
		completion.Failure = encodeTaskFailure("handler_failed", handlerErr)
	}
	_, err := r.store.CompleteTask(
		ctx,
		task.ID,
		task.LeaseOwner,
		r.options.taskClock.Now(),
		completion,
	)
	if errors.Is(err, store.ErrTaskLeaseLost) {
		return nil
	}
	return err
}

func (r *taskRunner) fail(err error) {
	r.mu.Lock()
	if r.runErr == nil {
		r.runErr = err
	}
	cancel := r.cancel
	r.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

func encodeTaskFailure(code string, err error) json.RawMessage {
	encoded, marshalErr := json.Marshal(map[string]string{
		"code":    code,
		"message": err.Error(),
	})
	if marshalErr != nil {
		return json.RawMessage(`{"code":"failure_encoding_failed"}`)
	}
	return encoded
}
