// SPDX-License-Identifier: GPL-3.0-or-later

package runtime

import (
	"context"
	"errors"
)

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

	// Every subprocess uses RuntimeDir as its working directory. A ready startup
	// artifact may outlive that directory, so recreate and validate it before
	// the first version command instead of relying on later config materialization.
	if err := ensurePrivateDirectory(manager.options.RuntimeDir); err != nil {
		return manager.startFailure("prepare_runtime", "directory", ErrMaterialization, err)
	}
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
