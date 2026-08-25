// SPDX-License-Identifier: GPL-3.0-or-later

package runtime

import (
	"context"
	"errors"
	"io"
	"path/filepath"
	"sync"
	"time"

	"github.com/rehuony/sing-box-panel/internal/coreartifact"
)

const (
	defaultShutdownGrace        = 10 * time.Second
	defaultProcessHealthWindow  = 500 * time.Millisecond
	defaultMaximumBinaryBytes   = 256 << 20
	defaultMaximumConfigBytes   = 16 << 20
	defaultMaximumCommandOutput = 64 << 10
)

// Manager owns the lifecycle of exactly one sing-box child process. Lifecycle
// operations are serialized, and every child has one owned reaper goroutine.
type Manager struct {
	options Options

	operationMu sync.Mutex
	mu          sync.Mutex
	status      Snapshot
	process     *managedProcess
	generation  uint64
	startCancel context.CancelFunc
	closing     bool
	closed      bool
	waitGroup   sync.WaitGroup
}

type managedProcess struct {
	child          ChildProcess
	generation     uint64
	done           chan struct{}
	desiredState   State
	desiredFailure *FailureStatus
	waitError      error
}

func NewManager(options Options) (*Manager, error) {
	if options.RuntimeDir == "" || !filepath.IsAbs(options.RuntimeDir) ||
		filepath.Clean(options.RuntimeDir) != options.RuntimeDir {
		return nil, ErrInvalidBundle
	}
	if options.Clock == nil {
		options.Clock = systemClock{}
	}
	if options.Executor == nil {
		executor, err := newDefaultCommandExecutor()
		if err != nil {
			return nil, err
		}
		options.Executor = executor
	}
	if options.Stdout == nil {
		options.Stdout = io.Discard
	}
	if options.Stderr == nil {
		options.Stderr = io.Discard
	}
	if options.ShutdownGrace == 0 {
		options.ShutdownGrace = defaultShutdownGrace
	}
	if options.ProcessHealthWindow == 0 {
		options.ProcessHealthWindow = defaultProcessHealthWindow
	}
	if options.MaximumBinaryBytes == 0 {
		options.MaximumBinaryBytes = defaultMaximumBinaryBytes
	}
	if options.MaximumConfigBytes == 0 {
		options.MaximumConfigBytes = defaultMaximumConfigBytes
	}
	if options.MaximumCommandOutput == 0 {
		options.MaximumCommandOutput = defaultMaximumCommandOutput
	}
	if options.ShutdownGrace < 0 || options.ProcessHealthWindow < 0 ||
		options.MaximumBinaryBytes <= 0 || options.MaximumConfigBytes <= 0 ||
		options.MaximumCommandOutput <= 0 {
		return nil, ErrInvalidBundle
	}
	if options.Probe == nil {
		options.Probe = processOnlyProbe{
			clock:  options.Clock,
			window: options.ProcessHealthWindow,
		}
	}
	if !validMonitoringLevel(options.Probe.Level()) {
		return nil, ErrInvalidBundle
	}

	now := options.Clock.Now().UTC()
	return &Manager{
		options: options,
		status: Snapshot{
			State:          StateStopped,
			TransitionedAt: now,
		},
	}, nil
}

// Start validates and runs the exact artifact and already-applied startup
// config supplied by bundle. It never loads, selects, or projects config.
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

func (manager *Manager) startLocked(ctx context.Context, bundle AppliedBundle) error {
	operationContext, cancel := context.WithCancel(ctx)
	manager.mu.Lock()
	manager.generation++
	generation := manager.generation
	manager.startCancel = cancel
	manager.status = Snapshot{
		State:                   StateStarting,
		BundleID:                bundle.ID,
		RequestedArtifactID:     bundle.ArtifactID,
		RequestedExactVersion:   bundle.ExactVersion,
		RequestedArtifactDigest: bundle.ArtifactDigest,
		StartupConfigDigest:     bundle.StartupConfigDigest,
		TransitionedAt:          manager.options.Clock.Now().UTC(),
	}
	manager.mu.Unlock()
	defer func() {
		cancel()
		manager.mu.Lock()
		if manager.generation == generation {
			manager.startCancel = nil
		}
		manager.mu.Unlock()
	}()

	if err := contextError(operationContext); err != nil {
		return manager.startFailure("start", "cancelled", err, err)
	}
	if err := verifyStartupConfigDigest(bundle.StartupConfig, bundle.StartupConfigDigest); err != nil {
		return manager.startFailure("verify_config", "digest_mismatch", ErrStartupConfigDigest, err)
	}
	actualDigest, err := verifyBinaryDigest(
		operationContext,
		bundle.BinaryPath,
		bundle.ArtifactDigest,
		manager.options.MaximumBinaryBytes,
	)
	if err != nil {
		if operationContext.Err() != nil {
			return manager.startFailure("start", "cancelled", operationContext.Err(), err)
		}
		if !actualDigest.IsZero() {
			manager.recordActualArtifact(generation, "", actualDigest)
		}
		return manager.startFailure("verify_artifact", "digest_mismatch", ErrArtifactDigest, err)
	}
	manager.recordActualArtifact(generation, bundle.ArtifactID, actualDigest)

	versionOutput, err := manager.options.Executor.Run(
		operationContext,
		manager.command(bundle.BinaryPath, "version"),
		manager.options.MaximumCommandOutput,
	)
	if err != nil {
		if operationContext.Err() != nil {
			return manager.startFailure("start", "cancelled", operationContext.Err(), err)
		}
		return manager.startFailure("verify_version", "execution", ErrVersionMismatch, err)
	}
	actualVersion, err := parseVersionOutput(versionOutput)
	if err != nil {
		return manager.startFailure("verify_version", "invalid_output", ErrVersionMismatch, err)
	}
	manager.recordActualVersion(generation, actualVersion)
	if actualVersion != bundle.ExactVersion {
		return manager.startFailure("verify_version", "mismatch", ErrVersionMismatch, nil)
	}
	actualDigest, err = verifyBinaryDigest(
		operationContext,
		bundle.BinaryPath,
		bundle.ArtifactDigest,
		manager.options.MaximumBinaryBytes,
	)
	if err != nil {
		if operationContext.Err() != nil {
			return manager.startFailure("start", "cancelled", operationContext.Err(), err)
		}
		if !actualDigest.IsZero() {
			manager.recordActualArtifact(generation, "", actualDigest)
		}
		return manager.startFailure("verify_artifact", "changed_after_version", ErrArtifactDigest, err)
	}

	if err := contextError(operationContext); err != nil {
		return manager.startFailure("start", "cancelled", err, err)
	}
	configPath, err := materializeStartupConfig(
		manager.options.RuntimeDir,
		bundle.StartupConfigDigest,
		bundle.StartupConfig,
	)
	if err != nil {
		return manager.startFailure("materialize_config", "write", ErrMaterialization, err)
	}
	if _, err := manager.options.Executor.Run(
		operationContext,
		manager.command(bundle.BinaryPath, "check", "-c", configPath),
		manager.options.MaximumCommandOutput,
	); err != nil {
		if operationContext.Err() != nil {
			return manager.startFailure("start", "cancelled", operationContext.Err(), err)
		}
		return manager.startFailure("check_config", "rejected", ErrCheckFailed, err)
	}
	actualDigest, err = verifyBinaryDigest(
		operationContext,
		bundle.BinaryPath,
		bundle.ArtifactDigest,
		manager.options.MaximumBinaryBytes,
	)
	if err != nil {
		if operationContext.Err() != nil {
			return manager.startFailure("start", "cancelled", operationContext.Err(), err)
		}
		if !actualDigest.IsZero() {
			manager.recordActualArtifact(generation, "", actualDigest)
		}
		return manager.startFailure("verify_artifact", "changed_after_check", ErrArtifactDigest, err)
	}
	if err := contextError(operationContext); err != nil {
		return manager.startFailure("start", "cancelled", err, err)
	}

	child, err := manager.options.Executor.Start(manager.command(bundle.BinaryPath, "run", "-c", configPath))
	if err != nil {
		return manager.startFailure("start_process", "execution", ErrProcessExited, err)
	}
	if child == nil || child.PID() <= 0 {
		if child != nil {
			_ = child.Kill()
		}
		return manager.startFailure("start_process", "invalid_process", ErrProcessExited, nil)
	}

	startedAt := manager.options.Clock.Now().UTC()
	process := &managedProcess{
		child:        child,
		generation:   generation,
		done:         make(chan struct{}),
		desiredState: StateFailed,
		desiredFailure: &FailureStatus{
			Operation: "process",
			Code:      "unexpected_exit",
			FailedAt:  startedAt,
		},
	}
	manager.mu.Lock()
	manager.process = process
	manager.status.PID = child.PID()
	manager.status.StartedAt = startedAt
	manager.mu.Unlock()
	manager.waitGroup.Add(1)
	go manager.reap(process)

	observation, err := manager.options.Probe.AwaitHealthy(operationContext, ProcessInfo{
		PID:       child.PID(),
		StartedAt: startedAt,
		Exited:    process.done,
	})
	if err != nil {
		if operationContext.Err() != nil {
			return manager.startedProcessFailure(
				operationContext,
				process,
				"start",
				"cancelled",
				errors.Join(ErrHealthFailed, operationContext.Err()),
				err,
			)
		}
		return manager.startedProcessFailure(
			operationContext,
			process,
			"health_check",
			"probe_error",
			ErrHealthFailed,
			err,
		)
	}
	if err := validateObservation(observation); err != nil {
		return manager.startedProcessFailure(
			operationContext,
			process,
			"health_check",
			"unhealthy",
			ErrHealthFailed,
			err,
		)
	}

	manager.mu.Lock()
	if manager.process != process {
		manager.mu.Unlock()
		return fail("health_check", "process_exited", ErrProcessExited, process.waitError)
	}
	now := manager.options.Clock.Now().UTC()
	manager.status.State = StateRunning
	manager.status.TransitionedAt = now
	manager.status.Health = &HealthStatus{
		Level:     manager.options.Probe.Level(),
		Healthy:   true,
		Code:      observation.Code,
		CheckedAt: now,
	}
	manager.status.Failure = nil
	manager.mu.Unlock()
	return nil
}

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

func (manager *Manager) startFailure(operation, code string, kind, cause error) error {
	now := manager.options.Clock.Now().UTC()
	manager.mu.Lock()
	manager.status.State = StateFailed
	manager.status.PID = 0
	manager.status.Health = nil
	manager.status.Failure = &FailureStatus{
		Operation: operation,
		Code:      code,
		FailedAt:  now,
	}
	manager.status.TransitionedAt = now
	manager.mu.Unlock()
	return fail(operation, code, kind, cause)
}

func (manager *Manager) recordActualArtifact(
	generation uint64,
	artifactID string,
	digest coreartifact.SHA256,
) {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	if manager.generation == generation {
		manager.status.ActualArtifactID = artifactID
		manager.status.ActualArtifactDigest = digest
	}
}

func (manager *Manager) recordActualVersion(generation uint64, version coreartifact.ExactVersion) {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	if manager.generation == generation {
		manager.status.ActualExactVersion = version
	}
}

func (manager *Manager) reap(process *managedProcess) {
	defer manager.waitGroup.Done()
	waitError := process.child.Wait()
	manager.mu.Lock()
	process.waitError = waitError
	if manager.process == process {
		now := manager.options.Clock.Now().UTC()
		manager.status.State = process.desiredState
		manager.status.PID = 0
		manager.status.Health = nil
		manager.status.TransitionedAt = now
		if process.desiredState == StateFailed {
			failure := process.desiredFailure
			if failure == nil {
				failure = &FailureStatus{
					Operation: "process",
					Code:      "unexpected_exit",
					FailedAt:  now,
				}
			}
			manager.status.Failure = failure
		} else {
			manager.status.Failure = nil
		}
		manager.process = nil
	}
	manager.mu.Unlock()
	close(process.done)
}

func (manager *Manager) command(path string, arguments ...string) Command {
	return Command{
		Path:   path,
		Args:   append([]string(nil), arguments...),
		Dir:    manager.options.RuntimeDir,
		Env:    append([]string(nil), fixedCommandEnvironment...),
		Stdout: manager.options.Stdout,
		Stderr: manager.options.Stderr,
	}
}

func (manager *Manager) cancelStartup() {
	manager.mu.Lock()
	cancel := manager.startCancel
	manager.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}
