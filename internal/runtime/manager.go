// SPDX-License-Identifier: GPL-3.0-or-later

package runtime

import (
	"context"
	"io"
	"path/filepath"
	"sync"
	"time"
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
