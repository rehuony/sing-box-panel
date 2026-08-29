// SPDX-License-Identifier: GPL-3.0-or-later

package runtime

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rehuony/sing-box-panel/internal/coreartifact"
)

func TestManagerStartsOnlyVerifiedAppliedBundle(t *testing.T) {
	t.Parallel()

	fixture := newRuntimeFixture(t, "1.13.19", []byte(`{"log":{"level":"info"}}`))
	process := newFakeProcess(4101, true)
	executor := newFakeExecutor(process)
	executor.versions[fixture.bundle.BinaryPath] = fixture.bundle.ExactVersion.String()
	clock := newFakeClock()
	probe := &fakeProbe{level: MonitoringLimited, observation: HealthObservation{Healthy: true, Code: "api_ready"}}
	manager := newTestManager(t, fixture.runtimeDir, executor, clock, probe)

	if err := manager.Start(testContext(t), fixture.bundle); err != nil {
		t.Fatalf("Start: %v", err)
	}
	status := manager.Status()
	if status.State != StateRunning || status.PID != process.PID() {
		t.Fatalf("status = %+v, want running PID %d", status, process.PID())
	}
	if status.RequestedExactVersion != fixture.bundle.ExactVersion ||
		status.ActualExactVersion != fixture.bundle.ExactVersion ||
		status.RequestedArtifactID != fixture.bundle.ArtifactID ||
		status.ActualArtifactID != fixture.bundle.ArtifactID ||
		status.RequestedArtifactDigest != fixture.bundle.ArtifactDigest ||
		status.ActualArtifactDigest != fixture.bundle.ArtifactDigest {
		t.Fatalf("verified identity not recorded: %+v", status)
	}
	if status.Health == nil || status.Health.Level != MonitoringLimited ||
		status.Health.Code != "api_ready" || !status.Health.Healthy {
		t.Fatalf("health = %+v, want limited api_ready", status.Health)
	}
	live := manager.ObserveLiveIdentity()
	if !live.Running || live.State != StateRunning || live.PID != process.PID() ||
		live.ExactVersion != fixture.bundle.ExactVersion ||
		live.ArtifactDigest != fixture.bundle.ArtifactDigest ||
		live.ArtifactID != fixture.bundle.ArtifactID || live.BundleID != fixture.bundle.ID {
		t.Fatalf("live identity = %+v, want verified running identity", live)
	}

	commands := executor.Commands()
	if len(commands) != 3 {
		t.Fatalf("commands = %d, want version/check/run", len(commands))
	}
	wantArguments := [][]string{
		{"version"},
		{"check", "-c", commands[1].Args[2]},
		{"run", "-c", commands[1].Args[2]},
	}
	for index, command := range commands {
		if command.Path != fixture.bundle.BinaryPath || !reflect.DeepEqual(command.Args, wantArguments[index]) {
			t.Fatalf("command[%d] = %q %q, want %q %q", index, command.Path, command.Args, fixture.bundle.BinaryPath, wantArguments[index])
		}
		if command.Dir != fixture.runtimeDir || !reflect.DeepEqual(command.Env, fixedCommandEnvironment) {
			t.Fatalf("command[%d] dir/env = %q/%q", index, command.Dir, command.Env)
		}
	}
	materialized, err := os.ReadFile(commands[1].Args[2])
	if err != nil {
		t.Fatalf("ReadFile(materialized): %v", err)
	}
	if !reflect.DeepEqual(materialized, fixture.bundle.StartupConfig) {
		t.Fatalf("materialized config = %q, want exact bytes %q", materialized, fixture.bundle.StartupConfig)
	}
	info, err := os.Stat(commands[1].Args[2])
	if err != nil {
		t.Fatalf("Stat(materialized): %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("materialized mode = %o, want 600", info.Mode().Perm())
	}

	closeManager(t, manager)
	if process.WaitCalls() != 1 {
		t.Fatalf("Wait calls = %d, want exactly one", process.WaitCalls())
	}
}

func TestManagerStartCreatesPrivateRuntimeDirectoryBeforeExecution(t *testing.T) {
	t.Parallel()

	fixture := newRuntimeFixture(t, "1.13.19", []byte(`{"inbounds":[]}`))
	process := newFakeProcess(4198, true)
	executor := newFakeExecutor(process)
	executor.versions[fixture.bundle.BinaryPath] = fixture.bundle.ExactVersion.String()
	inspected := false
	executor.runHook = func(command Command) {
		if !reflect.DeepEqual(command.Args, []string{"version"}) {
			return
		}
		inspected = true
		info, err := os.Stat(command.Dir)
		if err != nil {
			t.Errorf("stat runtime directory before first execution: %v", err)
			return
		}
		if !info.IsDir() || info.Mode().Perm() != 0o700 {
			t.Errorf("runtime directory mode before first execution = %v, want private directory", info.Mode())
		}
	}
	manager := newTestManager(t, fixture.runtimeDir, executor, newFakeClock(), immediateProbe())

	if _, err := os.Stat(fixture.runtimeDir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("runtime directory unexpectedly existed before start: %v", err)
	}
	if err := manager.Start(testContext(t), fixture.bundle); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if !inspected {
		t.Fatal("version command did not inspect the runtime directory")
	}
	closeManager(t, manager)
}

func TestManagerCheckCreatesPrivateRuntimeDirectoryBeforeExecution(t *testing.T) {
	t.Parallel()

	fixture := newRuntimeFixture(t, "1.13.19", []byte(`{"inbounds":[]}`))
	executor := newFakeExecutor(newFakeProcess(4199, true))
	executor.versions[fixture.bundle.BinaryPath] = fixture.bundle.ExactVersion.String()
	manager := newTestManager(t, fixture.runtimeDir, executor, newFakeClock(), immediateProbe())

	if _, err := os.Stat(fixture.runtimeDir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("runtime directory unexpectedly existed before check: %v", err)
	}
	if err := manager.Check(testContext(t), fixture.bundle); err != nil {
		t.Fatalf("Check: %v", err)
	}
	info, err := os.Stat(fixture.runtimeDir)
	if err != nil {
		t.Fatalf("stat runtime directory: %v", err)
	}
	if !info.IsDir() || info.Mode().Perm() != 0o700 {
		t.Fatalf("runtime directory mode = %v, want private directory", info.Mode())
	}
	for _, command := range executor.Commands() {
		if command.Dir != fixture.runtimeDir {
			t.Fatalf("command directory = %q, want %q", command.Dir, fixture.runtimeDir)
		}
	}
	closeManager(t, manager)
}

func TestManagerRejectsDigestMismatchBeforeExecution(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		mutate     func(*AppliedBundle)
		wantError  error
		wantActual bool
	}{
		{
			name: "config",
			mutate: func(bundle *AppliedBundle) {
				bundle.StartupConfigDigest = digestOf([]byte("different"))
			},
			wantError: ErrStartupConfigDigest,
		},
		{
			name: "artifact",
			mutate: func(bundle *AppliedBundle) {
				bundle.ArtifactDigest = digestOf([]byte("different"))
			},
			wantError:  ErrArtifactDigest,
			wantActual: true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			fixture := newRuntimeFixture(t, "1.13.19", []byte(`{"route":{}}`))
			test.mutate(&fixture.bundle)
			executor := newFakeExecutor(newFakeProcess(4102, true))
			manager := newTestManager(t, fixture.runtimeDir, executor, newFakeClock(), immediateProbe())

			err := manager.Start(testContext(t), fixture.bundle)
			if !errors.Is(err, test.wantError) {
				t.Fatalf("Start error = %v, want %v", err, test.wantError)
			}
			if got := len(executor.Commands()); got != 0 {
				t.Fatalf("executed %d commands before digest acceptance", got)
			}
			status := manager.Status()
			if status.State != StateFailed || status.Failure == nil {
				t.Fatalf("status = %+v, want failed", status)
			}
			if test.wantActual && status.ActualArtifactDigest.IsZero() {
				t.Fatal("actual mismatching artifact digest was not recorded")
			}
			closeManager(t, manager)
		})
	}
}

func TestManagerRejectsExactVersionMismatch(t *testing.T) {
	t.Parallel()

	fixture := newRuntimeFixture(t, "1.13.19", []byte(`{"dns":{}}`))
	executor := newFakeExecutor(newFakeProcess(4103, true))
	executor.versions[fixture.bundle.BinaryPath] = "1.12.8"
	manager := newTestManager(t, fixture.runtimeDir, executor, newFakeClock(), immediateProbe())

	err := manager.Start(testContext(t), fixture.bundle)
	if !errors.Is(err, ErrVersionMismatch) {
		t.Fatalf("Start error = %v, want ErrVersionMismatch", err)
	}
	status := manager.Status()
	wantActual, parseError := coreartifact.ParseExactVersion("1.12.8")
	if parseError != nil {
		t.Fatal(parseError)
	}
	if status.State != StateFailed || status.ActualExactVersion != wantActual {
		t.Fatalf("status = %+v, want actual 1.12.8 and failed", status)
	}
	commands := executor.Commands()
	if len(commands) != 1 || !reflect.DeepEqual(commands[0].Args, []string{"version"}) {
		t.Fatalf("commands = %+v, want only version", commands)
	}
	closeManager(t, manager)
}

func TestManagerRunsExactCheckBeforeStart(t *testing.T) {
	t.Parallel()

	fixture := newRuntimeFixture(t, "1.13.19", []byte(`{"inbounds":[]}`))
	executor := newFakeExecutor(newFakeProcess(4104, true))
	executor.versions[fixture.bundle.BinaryPath] = fixture.bundle.ExactVersion.String()
	executor.checkError = errors.New("invalid sing-box config")
	manager := newTestManager(t, fixture.runtimeDir, executor, newFakeClock(), immediateProbe())

	err := manager.Start(testContext(t), fixture.bundle)
	if !errors.Is(err, ErrCheckFailed) {
		t.Fatalf("Start error = %v, want ErrCheckFailed", err)
	}
	commands := executor.Commands()
	if len(commands) != 2 || commands[1].Args[0] != "check" {
		t.Fatalf("commands = %+v, want version then check", commands)
	}
	if executor.StartCalls() != 0 {
		t.Fatal("run process started after failed exact check")
	}
	if got := err.Error(); got != "runtime check_config failed (rejected)" {
		t.Fatalf("public error leaked command diagnostics: %q", got)
	}
	closeManager(t, manager)
}

func TestManagerDetectsArtifactMutationBetweenVerificationSteps(t *testing.T) {
	t.Parallel()

	fixture := newRuntimeFixture(t, "1.13.19", []byte(`{"outbounds":[]}`))
	executor := newFakeExecutor(newFakeProcess(4105, true))
	executor.versions[fixture.bundle.BinaryPath] = fixture.bundle.ExactVersion.String()
	executor.runHook = func(command Command) {
		if reflect.DeepEqual(command.Args, []string{"version"}) {
			if err := os.WriteFile(command.Path, []byte("mutated executable"), 0o700); err != nil {
				t.Errorf("mutate binary: %v", err)
			}
		}
	}
	manager := newTestManager(t, fixture.runtimeDir, executor, newFakeClock(), immediateProbe())

	err := manager.Start(testContext(t), fixture.bundle)
	if !errors.Is(err, ErrArtifactDigest) {
		t.Fatalf("Start error = %v, want ErrArtifactDigest", err)
	}
	if executor.StartCalls() != 0 {
		t.Fatal("mutated artifact was started")
	}
	if manager.Status().ActualArtifactDigest == fixture.bundle.ArtifactDigest {
		t.Fatal("status retained stale actual artifact digest after mutation")
	}
	closeManager(t, manager)
}

func TestManagerHealthFailureTerminatesProcess(t *testing.T) {
	t.Parallel()

	fixture := newRuntimeFixture(t, "1.13.19", []byte(`{"experimental":{}}`))
	process := newFakeProcess(4106, true)
	executor := newFakeExecutor(process)
	executor.versions[fixture.bundle.BinaryPath] = fixture.bundle.ExactVersion.String()
	probe := &fakeProbe{level: MonitoringLimited, observation: HealthObservation{Healthy: false, Code: "api_unavailable"}}
	manager := newTestManager(t, fixture.runtimeDir, executor, newFakeClock(), probe)

	err := manager.Start(testContext(t), fixture.bundle)
	if !errors.Is(err, ErrHealthFailed) {
		t.Fatalf("Start error = %v, want ErrHealthFailed", err)
	}
	waitSignal(t, process.terminated, "SIGTERM")
	waitSignal(t, process.waitFinished, "process reaper")
	status := manager.Status()
	if status.State != StateFailed || status.PID != 0 || status.Failure == nil || status.Failure.Operation != "health_check" {
		t.Fatalf("status = %+v, want failed health state", status)
	}
	closeManager(t, manager)
}

func TestManagerStopEscalatesAfterInjectedGracePeriod(t *testing.T) {
	t.Parallel()

	fixture := newRuntimeFixture(t, "1.13.19", []byte(`{"route":{}}`))
	process := newFakeProcess(4107, false)
	executor := newFakeExecutor(process)
	executor.versions[fixture.bundle.BinaryPath] = fixture.bundle.ExactVersion.String()
	clock := newFakeClock()
	manager := newTestManagerWithOptions(t, Options{
		RuntimeDir:    fixture.runtimeDir,
		Executor:      executor,
		Clock:         clock,
		Probe:         immediateProbe(),
		ShutdownGrace: 5 * time.Second,
	})
	if err := manager.Start(testContext(t), fixture.bundle); err != nil {
		t.Fatalf("Start: %v", err)
	}

	result := make(chan error, 1)
	go func() { result <- manager.Stop(testContext(t)) }()
	waitSignal(t, process.terminated, "SIGTERM")
	waitSignal(t, clock.timerCreated, "shutdown timer")
	clock.Advance(5 * time.Second)
	waitSignal(t, process.killed, "SIGKILL")
	if err := waitResult(t, result, "Stop"); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if status := manager.Status(); status.State != StateStopped || status.PID != 0 {
		t.Fatalf("status after Stop = %+v", status)
	}
	closeManager(t, manager)
}

func TestManagerRestartRequiresAndUsesFreshAppliedBundle(t *testing.T) {
	t.Parallel()

	base := t.TempDir()
	first := newRuntimeFixtureIn(t, base, "first", "1.12.8", []byte(`{"log":{"level":"warn"}}`))
	second := newRuntimeFixtureIn(t, base, "second", "1.13.19", []byte(`{"log":{"level":"error"}}`))
	second.runtimeDir = first.runtimeDir
	firstProcess := newFakeProcess(4108, true)
	secondProcess := newFakeProcess(4109, true)
	executor := newFakeExecutor(firstProcess, secondProcess)
	executor.versions[first.bundle.BinaryPath] = first.bundle.ExactVersion.String()
	executor.versions[second.bundle.BinaryPath] = second.bundle.ExactVersion.String()
	manager := newTestManager(t, first.runtimeDir, executor, newFakeClock(), immediateProbe())

	if err := manager.Start(testContext(t), first.bundle); err != nil {
		t.Fatalf("Start(first): %v", err)
	}
	if err := manager.Restart(testContext(t), second.bundle); err != nil {
		t.Fatalf("Restart(second): %v", err)
	}
	waitSignal(t, firstProcess.terminated, "first SIGTERM")
	status := manager.Status()
	if status.State != StateRunning || status.BundleID != second.bundle.ID ||
		status.ActualArtifactID != second.bundle.ArtifactID ||
		status.ActualExactVersion != second.bundle.ExactVersion ||
		status.ActualArtifactDigest != second.bundle.ArtifactDigest || status.PID != secondProcess.PID() {
		t.Fatalf("restart status = %+v, want exact second bundle", status)
	}
	commands := executor.Commands()
	if len(commands) != 6 || commands[3].Path != second.bundle.BinaryPath ||
		!reflect.DeepEqual(commands[3].Args, []string{"version"}) || commands[5].Path != second.bundle.BinaryPath {
		t.Fatalf("restart commands = %+v", commands)
	}
	closeManager(t, manager)
}

func TestManagerUnexpectedExitTransitionsToFailed(t *testing.T) {
	t.Parallel()

	fixture := newRuntimeFixture(t, "1.13.19", []byte(`{"route":{}}`))
	process := newFakeProcess(4110, true)
	executor := newFakeExecutor(process)
	executor.versions[fixture.bundle.BinaryPath] = fixture.bundle.ExactVersion.String()
	manager := newTestManager(t, fixture.runtimeDir, executor, newFakeClock(), immediateProbe())
	if err := manager.Start(testContext(t), fixture.bundle); err != nil {
		t.Fatalf("Start: %v", err)
	}

	process.Exit(errors.New("unexpected child exit"))
	waitSignal(t, process.waitFinished, "process reaper")
	status := manager.Status()
	if status.State != StateFailed || status.PID != 0 || status.Failure == nil || status.Failure.Code != "unexpected_exit" {
		t.Fatalf("status = %+v, want unexpected-exit failure", status)
	}
	closeManager(t, manager)
}

func TestStopCancelsInProgressStartupAndJoinsChild(t *testing.T) {
	t.Parallel()

	fixture := newRuntimeFixture(t, "1.13.19", []byte(`{"route":{}}`))
	process := newFakeProcess(4111, true)
	executor := newFakeExecutor(process)
	executor.versions[fixture.bundle.BinaryPath] = fixture.bundle.ExactVersion.String()
	probe := newBlockingProbe(MonitoringProcessOnly)
	manager := newTestManager(t, fixture.runtimeDir, executor, newFakeClock(), probe)
	startResult := make(chan error, 1)
	go func() { startResult <- manager.Start(testContext(t), fixture.bundle) }()
	waitSignal(t, probe.entered, "health probe")

	stopResult := make(chan error, 1)
	go func() { stopResult <- manager.Stop(testContext(t)) }()
	startError := waitResult(t, startResult, "Start cancellation")
	if !errors.Is(startError, context.Canceled) || !errors.Is(startError, ErrHealthFailed) {
		t.Fatalf("Start error = %v, want canceled health failure", startError)
	}
	if err := waitResult(t, stopResult, "Stop"); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if status := manager.Status(); status.State != StateStopped || status.PID != 0 {
		t.Fatalf("status = %+v, want stopped", status)
	}
	if process.WaitCalls() != 1 {
		t.Fatalf("Wait calls = %d, want exactly one", process.WaitCalls())
	}
	closeManager(t, manager)
}

func TestManagerCheckHealthUsesDeclaredMonitoringLevel(t *testing.T) {
	t.Parallel()

	for _, level := range []MonitoringLevel{MonitoringLimited, MonitoringProcessOnly} {
		t.Run(string(level), func(t *testing.T) {
			t.Parallel()
			fixture := newRuntimeFixture(t, "1.13.19", []byte(`{"route":{}}`))
			process := newFakeProcess(4200+int(level[0]), true)
			executor := newFakeExecutor(process)
			executor.versions[fixture.bundle.BinaryPath] = fixture.bundle.ExactVersion.String()
			probe := &fakeProbe{level: level, observation: HealthObservation{Healthy: true, Code: "evidence_ready"}}
			manager := newTestManager(t, fixture.runtimeDir, executor, newFakeClock(), probe)
			if err := manager.Start(testContext(t), fixture.bundle); err != nil {
				t.Fatalf("Start: %v", err)
			}
			health, err := manager.CheckHealth(testContext(t))
			if err != nil {
				t.Fatalf("CheckHealth: %v", err)
			}
			if health.Level != level || !health.Healthy || health.Code != "evidence_ready" {
				t.Fatalf("health = %+v, want declared %s evidence", health, level)
			}
			closeManager(t, manager)
		})
	}
}

func TestDefaultProcessOnlyProbeUsesInjectedClock(t *testing.T) {
	t.Parallel()

	fixture := newRuntimeFixture(t, "1.13.19", []byte(`{"route":{}}`))
	process := newFakeProcess(4112, true)
	executor := newFakeExecutor(process)
	executor.versions[fixture.bundle.BinaryPath] = fixture.bundle.ExactVersion.String()
	clock := newFakeClock()
	manager := newTestManagerWithOptions(t, Options{
		RuntimeDir:          fixture.runtimeDir,
		Executor:            executor,
		Clock:               clock,
		ProcessHealthWindow: 3 * time.Second,
	})
	result := make(chan error, 1)
	go func() { result <- manager.Start(testContext(t), fixture.bundle) }()
	waitSignal(t, clock.timerCreated, "process-only health timer")
	clock.Advance(3 * time.Second)
	if err := waitResult(t, result, "Start"); err != nil {
		t.Fatalf("Start: %v", err)
	}
	health := manager.Status().Health
	if health == nil || health.Level != MonitoringProcessOnly || health.Code != "process_alive" {
		t.Fatalf("health = %+v, want process-only evidence", health)
	}
	closeManager(t, manager)
}

func TestManagerCloseOwnsAndWaitsForReaper(t *testing.T) {
	t.Parallel()

	fixture := newRuntimeFixture(t, "1.13.19", []byte(`{"route":{}}`))
	process := newFakeProcess(4113, true)
	executor := newFakeExecutor(process)
	executor.versions[fixture.bundle.BinaryPath] = fixture.bundle.ExactVersion.String()
	manager := newTestManager(t, fixture.runtimeDir, executor, newFakeClock(), immediateProbe())
	if err := manager.Wait(); !errors.Is(err, ErrNotClosed) {
		t.Fatalf("Wait before Close = %v, want ErrNotClosed", err)
	}
	if err := manager.Start(testContext(t), fixture.bundle); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := manager.Close(testContext(t)); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := manager.Wait(); err != nil {
		t.Fatalf("Wait: %v", err)
	}
	if process.WaitCalls() != 1 {
		t.Fatalf("Wait calls = %d, want exactly one", process.WaitCalls())
	}
	if err := manager.Start(testContext(t), fixture.bundle); !errors.Is(err, ErrClosed) {
		t.Fatalf("Start after Close = %v, want ErrClosed", err)
	}
	if err := manager.Close(testContext(t)); err != nil {
		t.Fatalf("second Close: %v", err)
	}
}

type runtimeFixture struct {
	runtimeDir string
	bundle     AppliedBundle
}

func newRuntimeFixture(t *testing.T, version string, config []byte) runtimeFixture {
	t.Helper()
	return newRuntimeFixtureIn(t, t.TempDir(), "bundle", version, config)
}

func newRuntimeFixtureIn(t *testing.T, base, id, version string, config []byte) runtimeFixture {
	t.Helper()
	binaryPath := filepath.Join(base, id+"-sing-box")
	binary := []byte("test executable bytes for " + id + " " + version)
	if err := os.WriteFile(binaryPath, binary, 0o700); err != nil {
		t.Fatalf("WriteFile(binary): %v", err)
	}
	exactVersion, err := coreartifact.ParseExactVersion(version)
	if err != nil {
		t.Fatalf("ParseExactVersion: %v", err)
	}
	return runtimeFixture{
		runtimeDir: filepath.Join(base, "runtime"),
		bundle: AppliedBundle{
			ID:                  id,
			ArtifactID:          id + "-artifact",
			ExactVersion:        exactVersion,
			ArtifactDigest:      digestOf(binary),
			BinaryPath:          binaryPath,
			StartupConfig:       append([]byte(nil), config...),
			StartupConfigDigest: digestOf(config),
		},
	}
}

func digestOf(data []byte) coreartifact.SHA256 {
	return coreartifact.NewSHA256(sha256.Sum256(data))
}

func newTestManager(
	t *testing.T,
	runtimeDir string,
	executor CommandExecutor,
	clock Clock,
	probe HealthProbe,
) *Manager {
	t.Helper()
	return newTestManagerWithOptions(t, Options{
		RuntimeDir: runtimeDir,
		Executor:   executor,
		Clock:      clock,
		Probe:      probe,
	})
}

func newTestManagerWithOptions(t *testing.T, options Options) *Manager {
	t.Helper()
	manager, err := NewManager(options)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	return manager
}

func closeManager(t *testing.T, manager *Manager) {
	t.Helper()
	ctx := testContext(t)
	if err := manager.Close(ctx); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := manager.Wait(); err != nil {
		t.Fatalf("Wait: %v", err)
	}
}

func testContext(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)
	return ctx
}

func waitSignal(t *testing.T, channel <-chan struct{}, description string) {
	t.Helper()
	select {
	case <-channel:
	case <-testContext(t).Done():
		t.Fatalf("timed out waiting for %s", description)
	}
}

func waitResult(t *testing.T, channel <-chan error, description string) error {
	t.Helper()
	select {
	case err := <-channel:
		return err
	case <-testContext(t).Done():
		t.Fatalf("timed out waiting for %s", description)
		return nil
	}
}

type fakeExecutor struct {
	mu         sync.Mutex
	commands   []Command
	processes  []*fakeProcess
	versions   map[string]string
	checkError error
	runHook    func(Command)
	startCalls int
}

func newFakeExecutor(processes ...*fakeProcess) *fakeExecutor {
	return &fakeExecutor{processes: processes, versions: make(map[string]string)}
}

func (executor *fakeExecutor) Run(ctx context.Context, command Command, _ int64) ([]byte, error) {
	if err := contextError(ctx); err != nil {
		return nil, err
	}
	executor.record(command)
	executor.mu.Lock()
	version := executor.versions[command.Path]
	checkError := executor.checkError
	hook := executor.runHook
	executor.mu.Unlock()
	if hook != nil {
		hook(command)
	}
	if len(command.Args) == 0 {
		return nil, errors.New("missing command")
	}
	switch command.Args[0] {
	case "version":
		if version == "" {
			return nil, errors.New("missing fake version")
		}
		return []byte(fmt.Sprintf("sing-box version %s\n", version)), nil
	case "check":
		return nil, checkError
	default:
		return nil, errors.New("unexpected fake command")
	}
}

func (executor *fakeExecutor) Start(command Command) (ChildProcess, error) {
	executor.record(command)
	executor.mu.Lock()
	defer executor.mu.Unlock()
	executor.startCalls++
	if len(executor.processes) == 0 {
		return nil, errors.New("no fake process queued")
	}
	process := executor.processes[0]
	executor.processes = executor.processes[1:]
	return process, nil
}

func (executor *fakeExecutor) record(command Command) {
	executor.mu.Lock()
	defer executor.mu.Unlock()
	command.Args = append([]string(nil), command.Args...)
	command.Env = append([]string(nil), command.Env...)
	executor.commands = append(executor.commands, command)
}

func (executor *fakeExecutor) Commands() []Command {
	executor.mu.Lock()
	defer executor.mu.Unlock()
	commands := make([]Command, len(executor.commands))
	copy(commands, executor.commands)
	for index := range commands {
		commands[index].Args = append([]string(nil), commands[index].Args...)
		commands[index].Env = append([]string(nil), commands[index].Env...)
	}
	return commands
}

func (executor *fakeExecutor) StartCalls() int {
	executor.mu.Lock()
	defer executor.mu.Unlock()
	return executor.startCalls
}

type fakeProcess struct {
	pid             int
	exitOnTerminate bool
	exitOnce        sync.Once
	exited          chan struct{}
	terminated      chan struct{}
	killed          chan struct{}
	waitStarted     chan struct{}
	waitFinished    chan struct{}
	waitErrorMu     sync.Mutex
	waitError       error
	waitCalls       atomic.Int32
}

func newFakeProcess(pid int, exitOnTerminate bool) *fakeProcess {
	return &fakeProcess{
		pid:             pid,
		exitOnTerminate: exitOnTerminate,
		exited:          make(chan struct{}),
		terminated:      make(chan struct{}, 1),
		killed:          make(chan struct{}, 1),
		waitStarted:     make(chan struct{}, 1),
		waitFinished:    make(chan struct{}, 1),
	}
}

func (process *fakeProcess) PID() int { return process.pid }

func (process *fakeProcess) Wait() error {
	process.waitCalls.Add(1)
	process.waitStarted <- struct{}{}
	<-process.exited
	process.waitErrorMu.Lock()
	err := process.waitError
	process.waitErrorMu.Unlock()
	process.waitFinished <- struct{}{}
	return err
}

func (process *fakeProcess) Terminate() error {
	select {
	case process.terminated <- struct{}{}:
	default:
	}
	if process.exitOnTerminate {
		process.Exit(nil)
	}
	return nil
}

func (process *fakeProcess) Kill() error {
	select {
	case process.killed <- struct{}{}:
	default:
	}
	process.Exit(errors.New("killed"))
	return nil
}

func (process *fakeProcess) Exit(err error) {
	process.exitOnce.Do(func() {
		process.waitErrorMu.Lock()
		process.waitError = err
		process.waitErrorMu.Unlock()
		close(process.exited)
	})
}

func (process *fakeProcess) WaitCalls() int { return int(process.waitCalls.Load()) }

type fakeProbe struct {
	level       MonitoringLevel
	observation HealthObservation
	err         error
}

func immediateProbe() *fakeProbe {
	return &fakeProbe{
		level:       MonitoringProcessOnly,
		observation: HealthObservation{Healthy: true, Code: "process_alive"},
	}
}

func (probe *fakeProbe) Level() MonitoringLevel { return probe.level }

func (probe *fakeProbe) AwaitHealthy(context.Context, ProcessInfo) (HealthObservation, error) {
	return probe.observation, probe.err
}

type blockingProbe struct {
	level   MonitoringLevel
	entered chan struct{}
}

func newBlockingProbe(level MonitoringLevel) *blockingProbe {
	return &blockingProbe{level: level, entered: make(chan struct{}, 1)}
}

func (probe *blockingProbe) Level() MonitoringLevel { return probe.level }

func (probe *blockingProbe) AwaitHealthy(ctx context.Context, process ProcessInfo) (HealthObservation, error) {
	probe.entered <- struct{}{}
	select {
	case <-ctx.Done():
		return HealthObservation{}, ctx.Err()
	case <-process.Exited:
		return HealthObservation{}, ErrProcessExited
	}
}

type fakeClock struct {
	mu           sync.Mutex
	now          time.Time
	timers       []*fakeTimer
	timerCreated chan struct{}
}

func newFakeClock() *fakeClock {
	return &fakeClock{
		now:          time.Date(2026, time.August, 26, 12, 0, 0, 0, time.UTC),
		timerCreated: make(chan struct{}, 32),
	}
}

func (clock *fakeClock) Now() time.Time {
	clock.mu.Lock()
	defer clock.mu.Unlock()
	return clock.now
}

func (clock *fakeClock) NewTimer(duration time.Duration) Timer {
	clock.mu.Lock()
	timer := &fakeTimer{
		channel: make(chan time.Time, 1),
		due:     clock.now.Add(duration),
	}
	clock.timers = append(clock.timers, timer)
	clock.mu.Unlock()
	clock.timerCreated <- struct{}{}
	return timer
}

func (clock *fakeClock) Advance(duration time.Duration) {
	clock.mu.Lock()
	clock.now = clock.now.Add(duration)
	now := clock.now
	timers := append([]*fakeTimer(nil), clock.timers...)
	clock.mu.Unlock()
	for _, timer := range timers {
		if !timer.stopped.Load() && !timer.due.After(now) && !timer.fired.Swap(true) {
			timer.channel <- now
		}
	}
}

type fakeTimer struct {
	channel chan time.Time
	due     time.Time
	stopped atomic.Bool
	fired   atomic.Bool
}

func (timer *fakeTimer) C() <-chan time.Time { return timer.channel }

func (timer *fakeTimer) Stop() { timer.stopped.Store(true) }
