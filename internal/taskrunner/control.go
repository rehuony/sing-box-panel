package taskrunner

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/rehuony/sing-box-panel/internal/store"
)

var (
	ErrCancellationRequested = errors.New("task cancellation requested")
	ErrSuperseded            = errors.New("task superseded by a newer runtime generation")
)

// Control exposes an explicit safe cancellation boundary to a task handler.
// A handler must call SafePoint before beginning another externally visible or
// non-idempotent step. The runner does not asynchronously abort such a step.
type Control interface {
	SafePoint(context.Context) error
}

type leaseStore interface {
	HeartbeatTask(
		context.Context,
		string,
		string,
		time.Time,
		time.Duration,
	) (store.TaskLeaseState, error)
}

type taskControl struct {
	store         leaseStore
	taskID        string
	leaseOwner    string
	leaseDuration time.Duration
	clock         Clock

	mu sync.Mutex
}

func (c *taskControl) SafePoint(ctx context.Context) error {
	state, err := c.heartbeat(ctx)
	if err != nil {
		return err
	}
	return cancellationError(state)
}

func (c *taskControl) heartbeat(ctx context.Context) (store.TaskLeaseState, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	state, err := c.store.HeartbeatTask(
		ctx,
		c.taskID,
		c.leaseOwner,
		c.clock.Now(),
		c.leaseDuration,
	)
	if err != nil {
		return store.TaskLeaseState{}, err
	}
	return state, nil
}

func cancellationError(state store.TaskLeaseState) error {
	if state.Superseded {
		return ErrSuperseded
	}
	if state.CancelRequested {
		return ErrCancellationRequested
	}
	return nil
}
