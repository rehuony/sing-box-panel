// SPDX-License-Identifier: GPL-3.0-or-later

package runtime

import (
	"bytes"
	"io"
	"sync/atomic"
	"testing"
)

type outputHandle struct{ closed atomic.Int32 }

func (o *outputHandle) Close() error { o.closed.Add(1); return nil }

func TestManagerOwnsConfiguredOutputForChildLifetime(t *testing.T) {
	fixture := newRuntimeFixture(t, "1.13.19", []byte(`{"log":{"output":"native.log"}}`))
	process := newFakeProcess(8101, true)
	executor := newFakeExecutor(process)
	executor.versions[fixture.bundle.BinaryPath] = fixture.bundle.ExactVersion.String()
	handle := &outputHandle{}
	observations := 0
	manager := newTestManagerWithOptions(t, Options{RuntimeDir: fixture.runtimeDir, Executor: executor, Clock: newFakeClock(), Probe: immediateProbe(), ObserveOutput: func(config []byte, dir string) (io.Closer, error) {
		observations++
		if !bytes.Equal(config, fixture.bundle.StartupConfig) || dir != fixture.runtimeDir {
			t.Error("observer did not receive exact runtime inputs")
		}
		return handle, nil
	}})
	if err := manager.Start(testContext(t), fixture.bundle); err != nil {
		t.Fatal(err)
	}
	if observations != 1 || handle.closed.Load() != 0 {
		t.Fatal("incorrect output lifetime")
	}
	closeManager(t, manager)
	if handle.closed.Load() != 1 || process.WaitCalls() != 1 {
		t.Fatal("output was not joined with child")
	}
}
