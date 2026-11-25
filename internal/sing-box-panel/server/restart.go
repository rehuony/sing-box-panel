// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"

	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/releases"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/store"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/telemetry"
)

// errRestartRequested is why the server stops when the panel restarts.
var errRestartRequested = errors.New("panel restart requested")

// execFunc replaces the running program with path, run with args and env.
type execFunc func(path string, args, env []string) error

// restart re-executes the panel in place once the server has shut down the
// way a stop does: the same process, executable path, arguments and
// environment. The path is resolved when the panel starts, so a binary that
// an update replaced is the one that runs next.
type restart struct {
	path string
	args []string
	env  []string
	exec execFunc
	stop func()

	mu        sync.Mutex
	requested bool
	releases  *releases.Service
	records   *telemetry.Service
	version   string
}

func newRestart(exec execFunc, stop func()) (*restart, error) {
	path, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("resolve the panel executable: %w", err)
	}
	return &restart{path: path, args: os.Args, env: os.Environ(), exec: exec, stop: stop}, nil
}

// bind gives the restart the services it consults once they exist.
func (restart *restart) bind(panelReleases *releases.Service, records *telemetry.Service, version string) {
	restart.mu.Lock()
	defer restart.mu.Unlock()
	restart.releases, restart.records, restart.version = panelReleases, records, version
}

// request records the restart and stops the server once the request that
// asked, whose context is ctx, has been answered. A restart that is already
// requested is not requested again. An update that runs or is staged refuses
// it: the update restarts the panel itself.
func (restart *restart) request(ctx context.Context) error {
	restart.mu.Lock()
	defer restart.mu.Unlock()
	if restart.releases == nil || restart.records == nil {
		return errors.New("the panel is not ready to restart")
	}
	switch restart.releases.Status().State {
	case releases.StateUpdating, releases.StateStaged, releases.StateRestarting:
		return releases.ErrUpdateRunning
	}
	if restart.requested {
		return nil
	}
	restart.requested = true
	restart.records.RecordEvent(telemetry.LogRecordRequest{
		Source: store.LogSourcePanel, Level: store.LogLevelInfo, Code: "panel.restart_requested",
		Message: "Panel restart requested", Metadata: telemetry.LogMetadata(map[string]any{"version": restart.version}),
	})
	context.AfterFunc(ctx, restart.stop)
	return nil
}

// run replaces the stopped panel with a new one. It returns only when that
// fails, so that the caller exits with an error a supervisor acts on.
func (restart *restart) run() error {
	if err := restart.exec(restart.path, restart.args, restart.env); err != nil {
		return fmt.Errorf("restart the panel by running %s again: %w", restart.path, err)
	}
	return nil
}
