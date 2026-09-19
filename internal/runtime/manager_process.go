// SPDX-License-Identifier: GPL-3.0-or-later

package runtime

import (
	"errors"
	"io"

	"github.com/rehuony/sing-box-panel/internal/coreartifact"
)

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
	if process.output != nil {
		waitError = errors.Join(waitError, process.output.Close())
	}
	// Wait drains the child's pipes. Complete buffered lines before a new
	// incarnation can reuse the output streams.
	for _, output := range []io.Writer{manager.options.Stdout, manager.options.Stderr} {
		if buffered, ok := output.(interface{ Flush() error }); ok {
			waitError = errors.Join(waitError, buffered.Flush())
		}
	}
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
