// SPDX-License-Identifier: GPL-3.0-or-later

package systemd

import (
	"context"
	"errors"
	"slices"
	"sync"
	"testing"
	"time"
)

// scriptedRunner answers each systemctl show with the next active state.
type scriptedRunner struct {
	mu     sync.Mutex
	states []string
}

func (runner *scriptedRunner) Run(_ context.Context, name string, args ...string) (CommandResult, error) {
	if name != "systemctl" || !slices.Contains(args, "show") {
		return CommandResult{}, nil
	}
	runner.mu.Lock()
	defer runner.mu.Unlock()
	state := runner.states[0]
	if len(runner.states) > 1 {
		runner.states = runner.states[1:]
	}
	return CommandResult{Stdout: []byte("LoadState=loaded\nActiveState=" + state + "\nSubState=running\nUnitFileState=enabled\nMainPID=7\nFragmentPath=/etc/systemd/system/example.service\nDropInPaths=\nNeedDaemonReload=no\n")}, nil
}

func TestRestartAndWaitLetsASettleThatBeganInTimeFinish(t *testing.T) {
	previous := restartPoll
	restartPoll = 20 * time.Millisecond
	t.Cleanup(func() { restartPoll = previous })
	const timeout, settle = 200 * time.Millisecond, 250 * time.Millisecond
	for _, test := range []struct {
		name   string
		states []string
		want   error
	}{
		// Active before the timeout, settled after it.
		{name: "late but in time", states: []string{"activating", "activating", "active"}},
		{name: "never active", states: []string{"activating"}, want: ErrNotActive},
		{name: "stops during settle", states: []string{"active", "active", "activating"}, want: ErrNotActive},
	} {
		t.Run(test.name, func(t *testing.T) {
			manager, err := New(Definition[struct{}]{Name: "example", SystemStatePath: "/var/lib/example"}, Options{
				GOOS: "linux", EUID: func() int { return 0 }, Runner: &scriptedRunner{states: test.states},
			})
			if err != nil {
				t.Fatal(err)
			}
			err = manager.RestartAndWait(t.Context(), ScopeSystem, timeout, settle)
			if test.want == nil && err != nil || test.want != nil && !errors.Is(err, test.want) {
				t.Fatalf("RestartAndWait = %v, want %v", err, test.want)
			}
		})
	}
}
