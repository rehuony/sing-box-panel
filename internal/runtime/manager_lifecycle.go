// SPDX-License-Identifier: GPL-3.0-or-later

package runtime

import "context"

func (manager *Manager) Start(ctx context.Context, bundle AppliedBundle) error {
	if ctx == nil {
		return fail("start", "invalid_context", ErrInvalidBundle, nil)
	}
	prepared, err := cloneAndValidateBundle(bundle, manager.options.MaximumConfigBytes)
	if err != nil {
		return fail("start", "invalid_bundle", ErrInvalidBundle, err)
	}

	manager.operationMu.Lock()
	defer manager.operationMu.Unlock()
	manager.mu.Lock()
	blocked := manager.closing || manager.closed
	active := manager.process != nil || manager.status.State == StateStarting
	manager.mu.Unlock()
	if blocked {
		return ErrClosed
	}
	if active {
		return ErrAlreadyRunning
	}
	return manager.startLocked(ctx, prepared)
}

// Stop terminates the process group. A concurrent startup is canceled before
// waiting for the serialized lifecycle operation.
func (manager *Manager) Stop(ctx context.Context) error {
	if ctx == nil {
		return fail("stop", "invalid_context", ErrTermination, nil)
	}
	manager.cancelStartup()
	manager.operationMu.Lock()
	defer manager.operationMu.Unlock()
	return manager.stopLocked(ctx)
}

// CheckHealth refreshes health evidence for the active child using the
// configured probe. A failed probe transitions the process to failed and
// terminates it; callers can then submit an explicit Restart bundle.
func (manager *Manager) CheckHealth(ctx context.Context) (HealthStatus, error) {
	if ctx == nil {
		return HealthStatus{}, fail("health_check", "invalid_context", ErrHealthFailed, nil)
	}
	manager.operationMu.Lock()
	defer manager.operationMu.Unlock()
	manager.mu.Lock()
	if manager.closing || manager.closed {
		manager.mu.Unlock()
		return HealthStatus{}, ErrClosed
	}
	process := manager.process
	startedAt := manager.status.StartedAt
	manager.mu.Unlock()
	if process == nil {
		return HealthStatus{}, ErrNotRunning
	}

	observation, err := manager.options.Probe.AwaitHealthy(ctx, ProcessInfo{
		PID:       process.child.PID(),
		StartedAt: startedAt,
		Exited:    process.done,
	})
	if err != nil {
		if ctx.Err() != nil {
			return HealthStatus{}, fail("health_check", "cancelled", ctx.Err(), err)
		}
		return HealthStatus{}, manager.startedProcessFailure(
			ctx,
			process,
			"health_check",
			"probe_error",
			ErrHealthFailed,
			err,
		)
	}
	if err := validateObservation(observation); err != nil {
		return HealthStatus{}, manager.startedProcessFailure(
			ctx,
			process,
			"health_check",
			"unhealthy",
			ErrHealthFailed,
			err,
		)
	}

	health := HealthStatus{
		Level:     manager.options.Probe.Level(),
		Healthy:   true,
		Code:      observation.Code,
		CheckedAt: manager.options.Clock.Now().UTC(),
	}
	manager.mu.Lock()
	if manager.process != process {
		manager.mu.Unlock()
		return HealthStatus{}, fail("health_check", "process_exited", ErrProcessExited, process.waitError)
	}
	manager.status.State = StateRunning
	manager.status.Health = &health
	manager.status.Failure = nil
	manager.mu.Unlock()
	return health, nil
}

// Restart always requires a fresh applied bundle; it cannot silently reuse or
// derive a prior configuration.
func (manager *Manager) Restart(ctx context.Context, bundle AppliedBundle) error {
	if ctx == nil {
		return fail("restart", "invalid_context", ErrInvalidBundle, nil)
	}
	prepared, err := cloneAndValidateBundle(bundle, manager.options.MaximumConfigBytes)
	if err != nil {
		return fail("restart", "invalid_bundle", ErrInvalidBundle, err)
	}
	manager.cancelStartup()
	manager.operationMu.Lock()
	defer manager.operationMu.Unlock()
	manager.mu.Lock()
	blocked := manager.closing || manager.closed
	manager.mu.Unlock()
	if blocked {
		return ErrClosed
	}
	if err := manager.stopLocked(ctx); err != nil {
		return err
	}
	return manager.startLocked(ctx, prepared)
}

// Close prevents new starts and initiates a bounded stop. Wait may be called
// after Close returns, including when Close returned a context error.
func (manager *Manager) Close(ctx context.Context) error {
	if ctx == nil {
		return fail("close", "invalid_context", ErrTermination, nil)
	}
	manager.mu.Lock()
	manager.closing = true
	cancel := manager.startCancel
	manager.mu.Unlock()
	if cancel != nil {
		cancel()
	}

	manager.operationMu.Lock()
	err := manager.stopLocked(ctx)
	manager.mu.Lock()
	manager.closed = true
	manager.mu.Unlock()
	manager.operationMu.Unlock()
	return err
}

// Wait joins all child reaper goroutines. Calling Wait before Close could race
// a future Start, so it is explicitly rejected.
func (manager *Manager) Wait() error {
	manager.mu.Lock()
	closed := manager.closed
	manager.mu.Unlock()
	if !closed {
		return ErrNotClosed
	}
	manager.waitGroup.Wait()
	return nil
}

func (manager *Manager) Status() Snapshot {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	status := manager.status
	if status.Health != nil {
		health := *status.Health
		status.Health = &health
	}
	if status.Failure != nil {
		failure := *status.Failure
		status.Failure = &failure
	}
	return status
}

// MonitoringLevel is immutable for the manager lifetime and reports the
// evidence level the configured probe can actually establish.
func (manager *Manager) MonitoringLevel() MonitoringLevel {
	return manager.options.Probe.Level()
}

// ObserveLiveIdentity returns the identity of the actually verified child,
// not a desired or selected version. Running is true only after the health
// gate succeeds and while the owned child remains present.
func (manager *Manager) ObserveLiveIdentity() LiveIdentity {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	return LiveIdentity{
		Running:        manager.process != nil && manager.status.State == StateRunning,
		State:          manager.status.State,
		PID:            manager.status.PID,
		ExactVersion:   manager.status.ActualExactVersion,
		ArtifactDigest: manager.status.ActualArtifactDigest,
		ArtifactID:     manager.status.ActualArtifactID,
		BundleID:       manager.status.BundleID,
		StartedAt:      manager.status.StartedAt,
	}
}
