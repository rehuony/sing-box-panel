// SPDX-License-Identifier: GPL-3.0-or-later

package runtime

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestStartupConfigFilesShareAndRebuildExactBytes(t *testing.T) {
	t.Parallel()
	files := &startupConfigFiles{runtimeDir: filepath.Join(t.TempDir(), "runtime"), users: make(map[string]int)}
	data := []byte("{\n  \"outbounds\": []\n}\n")
	first, err := files.acquire(digestOf(data), data)
	if err != nil {
		t.Fatal(err)
	}
	second, err := files.acquire(digestOf(data), data)
	if err != nil {
		t.Fatal(err)
	}
	if first.path != second.path {
		t.Fatal("same bytes did not share a path")
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	assertConfigContents(t, second.path, data)
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	assertConfigContents(t, second.path, data)
	if err := second.Close(); err != nil {
		t.Fatal(err)
	}
	assertConfigAbsent(t, first.path)
	rebuilt, err := files.acquire(digestOf(data), data)
	if err != nil {
		t.Fatal(err)
	}
	assertConfigContents(t, rebuilt.path, data)
	if err := rebuilt.Close(); err != nil {
		t.Fatal(err)
	}
	assertConfigAbsent(t, rebuilt.path)
}

func TestManagerCheckReleasesSnapshotOnEveryOutcome(t *testing.T) {
	for _, outcome := range []string{"success", "rejected", "canceled"} {
		t.Run(outcome, func(t *testing.T) {
			fixture := newRuntimeFixture(t, "1.13.19", []byte(`{"route":{}}`))
			executor := newFakeExecutor()
			executor.versions[fixture.bundle.BinaryPath] = fixture.bundle.ExactVersion.String()
			manager := newTestManager(t, fixture.runtimeDir, executor, newFakeClock(), immediateProbe())
			ctx, cancel := context.WithCancel(testContext(t))
			defer cancel()
			executor.runHook = func(command Command) {
				if command.Args[0] == "check" {
					assertConfigContents(t, command.Args[2], fixture.bundle.StartupConfig)
					if outcome == "canceled" {
						cancel()
					}
				}
			}
			if outcome == "rejected" {
				executor.checkError = errors.New("check rejected")
			}
			err := manager.Check(ctx, fixture.bundle)
			if (err != nil) != (outcome != "success") {
				t.Fatalf("check %s: %v", outcome, err)
			}
			assertConfigAbsent(t, snapshotPath(fixture))
			closeManager(t, manager)
		})
	}
}

func TestManagerFailedStartReleasesSnapshot(t *testing.T) {
	for _, outcome := range []string{"check", "start", "output", "health", "invalid-pid"} {
		t.Run(outcome, func(t *testing.T) {
			fixture := newRuntimeFixture(t, "1.13.19", []byte(`{"route":{}}`))
			process := newFakeProcess(8701, true)
			if outcome == "invalid-pid" {
				process.pid = 0
			}
			executor := newFakeExecutor(process)
			executor.versions[fixture.bundle.BinaryPath] = fixture.bundle.ExactVersion.String()
			probe := immediateProbe()
			options := Options{RuntimeDir: fixture.runtimeDir, Executor: executor, Clock: newFakeClock(), Probe: probe}
			switch outcome {
			case "check":
				executor.checkError = errors.New("check rejected")
			case "start":
				executor.processes = nil
			case "output":
				options.ObserveOutput = func([]byte, string) (io.Closer, error) { return nil, errors.New("output unavailable") }
			case "health":
				probe.err = errors.New("unhealthy")
			}
			manager := newTestManagerWithOptions(t, options)
			if err := manager.Start(testContext(t), fixture.bundle); err == nil {
				t.Fatal("failed start succeeded")
			}
			assertConfigAbsent(t, snapshotPath(fixture))
			closeManager(t, manager)
		})
	}
}

func TestManagerCheckSharesConfigUntilLastProcessOrCheckExits(t *testing.T) {
	for _, exitDuringCheck := range []bool{false, true} {
		t.Run(map[bool]string{false: "check finishes first", true: "process exits first"}[exitDuringCheck], func(t *testing.T) {
			fixture := newRuntimeFixture(t, "1.13.19", []byte(`{"route":{}}`))
			process := newFakeProcess(8702, true)
			executor := newFakeExecutor(process)
			executor.versions[fixture.bundle.BinaryPath] = fixture.bundle.ExactVersion.String()
			manager := newTestManager(t, fixture.runtimeDir, executor, newFakeClock(), immediateProbe())
			if err := manager.Start(testContext(t), fixture.bundle); err != nil {
				t.Fatal(err)
			}
			manager.mu.Lock()
			done := manager.process.done
			manager.mu.Unlock()
			executor.runHook = func(command Command) {
				if command.Args[0] != "check" {
					return
				}
				if exitDuringCheck {
					process.Exit(errors.New("unexpected exit"))
					// Check still owns operationMu: waiting here proves the reaper
					// can finish without that lifecycle lock or deleting our file.
					waitSignal(t, done, "reaper during check")
				}
				manager.PruneStartupConfigs(testContext(t))
				assertConfigContents(t, command.Args[2], fixture.bundle.StartupConfig)
			}
			if err := manager.Check(testContext(t), fixture.bundle); err != nil {
				t.Fatal(err)
			}
			if exitDuringCheck {
				assertConfigAbsent(t, snapshotPath(fixture))
			} else {
				assertConfigContents(t, snapshotPath(fixture), fixture.bundle.StartupConfig)
				if err := manager.Stop(testContext(t)); err != nil {
					t.Fatal(err)
				}
				assertConfigAbsent(t, snapshotPath(fixture))
			}
			closeManager(t, manager)
		})
	}
}

func TestPruneStartupConfigsPreservesActiveAndUnmanagedFiles(t *testing.T) {
	fixture := newRuntimeFixture(t, "1.13.19", []byte(`{"route":{}}`))
	executor := newFakeExecutor(newFakeProcess(8703, true))
	executor.versions[fixture.bundle.BinaryPath] = fixture.bundle.ExactVersion.String()
	manager := newTestManager(t, fixture.runtimeDir, executor, newFakeClock(), immediateProbe())
	manager.PruneStartupConfigs(testContext(t))
	assertConfigAbsent(t, fixture.runtimeDir)
	if err := manager.Start(testContext(t), fixture.bundle); err != nil {
		t.Fatal(err)
	}
	stale := []byte(`{"outbounds":[]}`)
	stalePath, err := materializeStartupConfig(fixture.runtimeDir, digestOf(stale), stale)
	if err != nil {
		t.Fatal(err)
	}
	directory := filepath.Dir(stalePath)
	temporary, err := os.CreateTemp(directory, ".startup-config-")
	if err != nil {
		t.Fatal(err)
	}
	temporary.Close()
	keep := []string{"notes.json", "current.json", ".startup-config-notes", strings.Repeat("x", 64) + ".json"}
	for _, name := range keep {
		if err := os.WriteFile(filepath.Join(directory, name), []byte("operator file"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	outside := filepath.Join(t.TempDir(), "target")
	if err := os.WriteFile(outside, []byte("outside"), 0o600); err != nil {
		t.Fatal(err)
	}
	linked := filepath.Join(directory, strings.Repeat("a", 64)+".json")
	if err := os.Symlink(outside, linked); err != nil {
		t.Fatal(err)
	}
	nested := filepath.Join(directory, strings.Repeat("b", 64)+".json")
	if err := os.Mkdir(nested, 0o700); err != nil {
		t.Fatal(err)
	}
	manager.PruneStartupConfigs(testContext(t))
	assertConfigAbsent(t, stalePath)
	assertConfigAbsent(t, temporary.Name())
	assertConfigContents(t, snapshotPath(fixture), fixture.bundle.StartupConfig)
	assertConfigContents(t, outside, []byte("outside"))
	for _, name := range append(keep, filepath.Base(linked), filepath.Base(nested)) {
		if _, err := os.Lstat(filepath.Join(directory, name)); err != nil {
			t.Fatalf("removed unmanaged entry %q: %v", name, err)
		}
	}
	closeManager(t, manager)
	assertConfigAbsent(t, snapshotPath(fixture))
}

func TestManagerRestartRebuildsReleasedSnapshots(t *testing.T) {
	base := t.TempDir()
	first := newRuntimeFixtureIn(t, base, "first", "1.13.19", []byte(`{"route":{}}`))
	second := newRuntimeFixtureIn(t, base, "second", "1.13.20", []byte(`{"outbounds":[]}`))
	executor := newFakeExecutor(newFakeProcess(8710, true), newFakeProcess(8711, true), newFakeProcess(8712, true))
	executor.versions[first.bundle.BinaryPath] = first.bundle.ExactVersion.String()
	executor.versions[second.bundle.BinaryPath] = second.bundle.ExactVersion.String()
	manager := newTestManager(t, first.runtimeDir, executor, newFakeClock(), immediateProbe())
	if err := manager.Start(testContext(t), first.bundle); err != nil {
		t.Fatal(err)
	}
	for _, rejected := range []bool{true, false} {
		executor.checkError = nil
		if rejected {
			executor.checkError = errors.New("candidate rejected")
		}
		if err := manager.Check(testContext(t), second.bundle); (err != nil) != rejected {
			t.Fatalf("candidate check: %v", err)
		}
		assertConfigContents(t, snapshotPath(first), first.bundle.StartupConfig)
		assertConfigAbsent(t, snapshotPath(second))
	}
	if err := manager.Restart(testContext(t), second.bundle); err != nil {
		t.Fatal(err)
	}
	assertConfigAbsent(t, snapshotPath(first))
	assertConfigContents(t, snapshotPath(second), second.bundle.StartupConfig)
	if err := manager.Restart(testContext(t), first.bundle); err != nil {
		t.Fatal(err)
	}
	assertConfigAbsent(t, snapshotPath(second))
	assertConfigContents(t, snapshotPath(first), first.bundle.StartupConfig)
	closeManager(t, manager)
	assertConfigAbsent(t, snapshotPath(first))
}

func TestManagerCanceledStopKeepsSnapshotUntilWaitReturns(t *testing.T) {
	fixture := newRuntimeFixture(t, "1.13.19", []byte(`{"route":{}}`))
	process := newFakeProcess(8713, true)
	gate := make(chan struct{})
	executor := &delayedWaitExecutor{fakeExecutor: newFakeExecutor(process), gate: gate}
	executor.versions[fixture.bundle.BinaryPath] = fixture.bundle.ExactVersion.String()
	manager := newTestManager(t, fixture.runtimeDir, executor, newFakeClock(), immediateProbe())
	if err := manager.Start(testContext(t), fixture.bundle); err != nil {
		t.Fatal(err)
	}
	manager.mu.Lock()
	done := manager.process.done
	manager.mu.Unlock()
	ctx, cancel := context.WithCancel(testContext(t))
	defer cancel()
	result := make(chan error, 1)
	go func() { result <- manager.Stop(ctx) }()
	waitSignal(t, process.waitFinished, "child exit before delayed Wait returns")
	cancel()
	if err := waitResult(t, result, "canceled Stop"); !errors.Is(err, context.Canceled) {
		t.Fatalf("Stop: %v", err)
	}
	manager.PruneStartupConfigs(testContext(t))
	assertConfigContents(t, snapshotPath(fixture), fixture.bundle.StartupConfig)
	close(gate)
	waitSignal(t, done, "delayed reaper")
	assertConfigAbsent(t, snapshotPath(fixture))
	closeManager(t, manager)
}

type delayedWaitExecutor struct {
	*fakeExecutor
	gate <-chan struct{}
}

func (executor *delayedWaitExecutor) Start(command Command) (ChildProcess, error) {
	child, err := executor.fakeExecutor.Start(command)
	if err != nil {
		return nil, err
	}
	return &delayedWaitProcess{ChildProcess: child, gate: executor.gate}, nil
}

type delayedWaitProcess struct {
	ChildProcess
	gate <-chan struct{}
}

func (process *delayedWaitProcess) Wait() error {
	err := process.ChildProcess.Wait()
	<-process.gate
	return err
}

func TestStartupConfigCleanupRejectsUnsafeDirectories(t *testing.T) {
	for _, scenario := range []string{"runtime-symlink", "configs-symlink", "runtime-permissions", "configs-permissions"} {
		t.Run(scenario, func(t *testing.T) {
			root := t.TempDir()
			runtimeDir := filepath.Join(root, "runtime")
			data := []byte(`{}`)
			path, err := materializeStartupConfig(runtimeDir, digestOf(data), data)
			if err != nil {
				t.Fatal(err)
			}
			target := runtimeDir
			if strings.HasPrefix(scenario, "configs-") {
				target = filepath.Dir(path)
			}
			if strings.HasSuffix(scenario, "symlink") {
				moved := target + "-original"
				if err := os.Rename(target, moved); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(moved, target); err != nil {
					t.Fatal(err)
				}
			} else if err := os.Chmod(target, 0o755); err != nil {
				t.Fatal(err)
			}
			files := &startupConfigFiles{runtimeDir: runtimeDir, users: make(map[string]int)}
			if err := files.prune(testContext(t)); err == nil {
				t.Fatal("cleaned unsafe directory")
			}
			assertConfigContents(t, path, data)
		})
	}
}

func TestManagerCleanupFailureWarnsOutsideLockAndRetries(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root can remove files from a non-writable directory")
	}
	fixture := newRuntimeFixture(t, "1.13.19", []byte(`{"route":{}}`))
	executor := newFakeExecutor()
	executor.versions[fixture.bundle.BinaryPath] = fixture.bundle.ExactVersion.String()
	var manager *Manager
	warnings := 0
	manager = newTestManagerWithOptions(t, Options{
		RuntimeDir: fixture.runtimeDir, Executor: executor, Clock: newFakeClock(), Probe: immediateProbe(),
		ObserveConfigCleanupError: func(err error) {
			if !errors.Is(err, os.ErrPermission) {
				t.Errorf("cleanup warning = %v, want permission error", err)
			}
			if !manager.configs.mu.TryLock() {
				t.Error("warning callback holds config ownership lock")
			} else {
				manager.configs.mu.Unlock()
			}
			warnings++
		},
	})
	directory := filepath.Join(fixture.runtimeDir, "configs")
	t.Cleanup(func() { _ = os.Chmod(directory, 0o700) })
	executor.runHook = func(command Command) {
		if command.Args[0] == "check" {
			if err := os.Chmod(directory, 0o500); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := manager.Check(testContext(t), fixture.bundle); err != nil {
		t.Fatalf("cleanup changed a successful check: %v", err)
	}
	if warnings != 1 {
		t.Fatalf("cleanup warnings = %d, want 1", warnings)
	}
	assertConfigContents(t, snapshotPath(fixture), fixture.bundle.StartupConfig)
	if err := os.Chmod(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	executor.runHook = nil
	other := fixture.bundle
	other.StartupConfig = []byte(`{"dns":{}}`)
	other.StartupConfigDigest = digestOf(other.StartupConfig)
	if err := manager.Check(testContext(t), other); err != nil {
		t.Fatal(err)
	}
	assertConfigAbsent(t, snapshotPath(fixture))
	assertConfigAbsent(t, filepath.Join(directory, other.StartupConfigDigest.String()+".json"))
	closeManager(t, manager)
}

func TestManagerCleanupWarningDoesNotHideProcessExit(t *testing.T) {
	fixture := newRuntimeFixture(t, "1.13.19", []byte(`{"route":{}}`))
	process := newFakeProcess(8720, true)
	executor := newFakeExecutor(process)
	executor.versions[fixture.bundle.BinaryPath] = fixture.bundle.ExactVersion.String()
	clock := newFakeClock()
	warningStarted := make(chan struct{})
	releaseWarning := make(chan struct{})
	ctx := testContext(t)
	manager := newTestManagerWithOptions(t, Options{
		RuntimeDir: fixture.runtimeDir, Executor: executor, Clock: clock,
		ObserveConfigCleanupError: func(error) {
			close(warningStarted)
			select {
			case <-releaseWarning:
			case <-ctx.Done():
			}
		},
	})
	directory := filepath.Join(fixture.runtimeDir, "configs")
	defer func() {
		close(releaseWarning)
		_ = os.Chmod(directory, 0o700)
		closeManager(t, manager)
	}()
	result := make(chan error, 1)
	go func() { result <- manager.Start(ctx, fixture.bundle) }()
	waitSignal(t, clock.timerCreated, "process health window")
	// This fails the private-directory check even when tests run as root.
	if err := os.Chmod(directory, 0o755); err != nil {
		t.Fatal(err)
	}
	process.Exit(errors.New("unexpected exit"))
	waitSignal(t, warningStarted, "blocked cleanup warning")
	clock.Advance(time.Second)
	err := waitResult(t, result, "Start while cleanup warning is blocked")
	if !errors.Is(err, ErrHealthFailed) && !errors.Is(err, ErrProcessExited) {
		t.Fatalf("Start = %v, want exited process to fail its health gate", err)
	}
	if status := manager.Status(); status.State != StateFailed || status.PID != 0 || status.Health != nil {
		t.Fatalf("exited process status = %+v, want failed without PID or health", status)
	}
	if err := manager.Stop(ctx); err != nil {
		t.Fatalf("Stop while cleanup warning is blocked: %v", err)
	}
}

func snapshotPath(fixture runtimeFixture) string {
	return filepath.Join(fixture.runtimeDir, "configs", fixture.bundle.StartupConfigDigest.String()+".json")
}

func assertConfigContents(t *testing.T, path string, want []byte) {
	t.Helper()
	if actual, err := os.ReadFile(path); err != nil || string(actual) != string(want) {
		t.Fatalf("config %s = %q, %v; want %q", path, actual, err, want)
	}
}

func assertConfigAbsent(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("path %s was not removed: %v", path, err)
	}
}
