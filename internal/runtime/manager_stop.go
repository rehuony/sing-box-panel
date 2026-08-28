// SPDX-License-Identifier: GPL-3.0-or-later

package runtime

import (
	"context"
	"errors"
)

func (manager *Manager) stopLocked(ctx context.Context) error {
	manager.mu.Lock()
	process := manager.process
	if process == nil {
		manager.status.State = StateStopped
		manager.status.PID = 0
		manager.status.Health = nil
		manager.status.Failure = nil
		manager.status.TransitionedAt = manager.options.Clock.Now().UTC()
		manager.mu.Unlock()
		if err := contextError(ctx); err != nil {
			return fail("stop", "cancelled", ErrTermination, err)
		}
		return nil
	}
	process.desiredState = StateStopped
	process.desiredFailure = nil
	manager.mu.Unlock()

	if err := manager.terminateProcess(ctx, process); err != nil {
		return fail("stop", "termination", ErrTermination, err)
	}
	return nil
}

func (manager *Manager) terminateProcess(ctx context.Context, process *managedProcess) error {
	terminateError := process.child.Terminate()
	if errors.Is(terminateError, ErrProcessExited) {
		terminateError = nil
	}
	timer := manager.options.Clock.NewTimer(manager.options.ShutdownGrace)
	defer timer.Stop()
	select {
	case <-process.done:
		return terminateError
	case <-ctx.Done():
		killError := process.child.Kill()
		if errors.Is(killError, ErrProcessExited) {
			killError = nil
		}
		return errors.Join(ctx.Err(), terminateError, killError)
	case <-timer.C():
		killError := process.child.Kill()
		if errors.Is(killError, ErrProcessExited) {
			killError = nil
		}
		if killError != nil {
			return errors.Join(terminateError, killError)
		}
	}

	select {
	case <-process.done:
		return terminateError
	case <-ctx.Done():
		return errors.Join(ctx.Err(), terminateError)
	}
}

func (manager *Manager) startedProcessFailure(
	ctx context.Context,
	process *managedProcess,
	operation string,
	code string,
	kind error,
	cause error,
) error {
	now := manager.options.Clock.Now().UTC()
	manager.mu.Lock()
	if manager.process == process {
		failure := &FailureStatus{Operation: operation, Code: code, FailedAt: now}
		process.desiredState = StateFailed
		process.desiredFailure = failure
		manager.status.State = StateFailed
		manager.status.Health = nil
		manager.status.Failure = failure
		manager.status.TransitionedAt = now
	}
	manager.mu.Unlock()
	terminationError := manager.terminateProcess(ctx, process)
	return fail(operation, code, kind, errors.Join(cause, terminationError))
}
