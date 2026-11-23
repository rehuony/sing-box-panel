// SPDX-License-Identifier: GPL-3.0-or-later

package runtime

import (
	"errors"
	"reflect"
	"testing"
)

func TestManagerCheckVerifiesWithoutStartingProcess(t *testing.T) {
	t.Parallel()
	fixture := newRuntimeFixture(t, "1.13.19", []byte(`{"route":{}}`))
	executor := newFakeExecutor(newFakeProcess(4401, true))
	executor.versions[fixture.bundle.BinaryPath] = fixture.bundle.ExactVersion.String()
	manager := newTestManager(t, fixture.runtimeDir, executor, newFakeClock(), immediateProbe())

	if err := manager.Check(testContext(t), fixture.bundle); err != nil {
		t.Fatalf("Check: %v", err)
	}
	commands := executor.Commands()
	if len(commands) != 2 || !reflect.DeepEqual(commands[0].Args, []string{"version"}) ||
		len(commands[1].Args) != 3 || commands[1].Args[0] != "check" {
		t.Fatalf("commands = %+v, want version then check", commands)
	}
	if executor.StartCalls() != 0 || manager.Status().State != StateStopped {
		t.Fatalf("Check started or changed runtime: starts=%d status=%+v", executor.StartCalls(), manager.Status())
	}
	closeManager(t, manager)
}

func TestManagerCheckRejectsConfigAndVersion(t *testing.T) {
	t.Parallel()
	fixture := newRuntimeFixture(t, "1.13.19", []byte(`{"route":{}}`))
	executor := newFakeExecutor(newFakeProcess(4402, true))
	executor.versions[fixture.bundle.BinaryPath] = "1.12.8"
	manager := newTestManager(t, fixture.runtimeDir, executor, newFakeClock(), immediateProbe())
	if err := manager.Check(testContext(t), fixture.bundle); !errors.Is(err, ErrVersionMismatch) {
		t.Fatalf("version mismatch error = %v", err)
	}
	if manager.Status().State != StateStopped {
		t.Fatalf("failed Check changed manager status: %+v", manager.Status())
	}
	closeManager(t, manager)
}
