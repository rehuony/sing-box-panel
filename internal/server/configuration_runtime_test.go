// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/rehuony/sing-box-panel/internal/application"
	coreruntime "github.com/rehuony/sing-box-panel/internal/runtime"
	"github.com/rehuony/sing-box-panel/internal/store"
)

type configurationCheckManager struct {
	fakeRuntimeManager
	checkError error
	checkHook  func()
}

func (manager *configurationCheckManager) Check(ctx context.Context, bundle coreruntime.AppliedBundle) error {
	_ = manager.fakeRuntimeManager.Check(ctx, bundle)
	if manager.checkHook != nil {
		manager.checkHook()
	}
	return manager.checkError
}

func TestConfigurationRuntimePreflightPreservesProcessAndFencesChanges(t *testing.T) {
	for _, scenario := range []string{"success", "binary-rejected", "edited-during-check", "superseded-during-check"} {
		t.Run(scenario, func(t *testing.T) {
			ctx := context.Background()
			db, commands, observation := seedRuntimeObservation(t, ctx)
			file, err := commands.ConfigurationFile(ctx)
			if err != nil {
				t.Fatal(err)
			}
			saved, err := commands.SaveConfigurationFile(ctx, application.ConfigurationFileWrite{Revision: file.Revision, Content: `{"log":{"level":"debug"},"future":9007199254740993}`})
			if err != nil {
				t.Fatal(err)
			}
			before, err := db.Bootstrap(ctx)
			if err != nil {
				t.Fatal(err)
			}
			queued, err := commands.QueueRuntimeRestart(ctx)
			if err != nil {
				t.Fatal(err)
			}
			reserved, err := db.Bootstrap(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if reserved.Hub.DesiredBundleID != before.Hub.DesiredBundleID || reserved.Hub.AppliedBundleID != before.Hub.AppliedBundleID || reserved.Hub.DesiredRunning != before.Hub.DesiredRunning {
				t.Fatal("queue changed the desired process before validation")
			}
			task, err := db.ClaimTask(ctx, store.ClaimTaskInput{Lane: store.TaskLaneRuntime, LeaseOwner: "preflight", Now: time.Now().UTC(), LeaseDuration: time.Minute})
			if err != nil || task == nil || task.ID != queued.ID {
				t.Fatalf("claim: %+v %v", task, err)
			}
			manager := &configurationCheckManager{}
			if scenario == "binary-rejected" {
				manager.checkError = errors.New("binary rejected configuration")
			}
			if scenario == "edited-during-check" {
				manager.checkHook = func() {
					if _, err := commands.SaveConfigurationFile(ctx, application.ConfigurationFileWrite{Revision: saved.Revision, Content: "{"}); err != nil {
						t.Fatal(err)
					}
				}
			}
			if scenario == "superseded-during-check" {
				manager.checkHook = func() {
					if _, err := commands.QueueRuntimeStop(ctx); err != nil {
						t.Fatal(err)
					}
				}
			}
			services := &runtimeServices{database: db, commands: commands, manager: manager}
			bound, err := services.checkConfigurationForTask(ctx, *task, successfulTaskControl{})
			if scenario == "success" {
				if err != nil || bound.ActivationBundleID == "" {
					t.Fatalf("bind: %+v %v", bound, err)
				}
				material, err := commands.LoadRuntimeMaterial(ctx, bound.ActivationBundleID)
				if err != nil {
					t.Fatal(err)
				}
				if material.Startup.CanonicalRevisionID != saved.CanonicalRevisionID || !strings.Contains(string(manager.checkedBundle.StartupConfig), "9007199254740993") {
					t.Fatal("checked bytes did not match saved configuration")
				}
			} else if err == nil {
				t.Fatal("unsafe preflight was accepted")
			}
			current, err := db.RuntimeObservation(ctx)
			if err != nil || current != observation || manager.stopCalls != 0 {
				t.Fatalf("preflight changed the live process: %+v %v", current, err)
			}
		})
	}
}

func TestInvalidSavedConfigurationCannotStartOrRestartFromOldHistory(t *testing.T) {
	ctx := context.Background()
	db, commands, observation := seedRuntimeObservation(t, ctx)
	file, err := commands.ConfigurationFile(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := commands.SaveConfigurationFile(ctx, application.ConfigurationFileWrite{Revision: file.Revision, Content: "{"}); err != nil {
		t.Fatal(err)
	}
	for _, queue := range []func(context.Context) (application.Task, error){commands.QueueRuntimeStart, commands.QueueRuntimeRestart} {
		if _, err := queue(ctx); !errors.Is(err, store.ErrConfigurationFileUnparsed) {
			t.Fatalf("invalid file used old history: %v", err)
		}
	}
	current, err := db.RuntimeObservation(ctx)
	if err != nil || current != observation {
		t.Fatalf("changed observation: %+v %v", current, err)
	}
}

type configurationLaunchManager struct {
	configurationCheckManager
	launches int
}

func (manager *configurationLaunchManager) Start(_ context.Context, bundle coreruntime.AppliedBundle) error {
	manager.launches++
	manager.live = coreruntime.LiveIdentity{
		Running: true, State: coreruntime.StateRunning, PID: 5151,
		BundleID: bundle.ID, ArtifactID: bundle.ArtifactID, ExactVersion: bundle.ExactVersion,
		ArtifactDigest: bundle.ArtifactDigest, StartedAt: time.Now().UTC(),
	}
	return nil
}

func (manager *configurationLaunchManager) Restart(ctx context.Context, bundle coreruntime.AppliedBundle) error {
	return manager.Start(ctx, bundle)
}

func TestSavedConfigurationRestartCommitsCheckedBytesAndLoadedIdentity(t *testing.T) {
	ctx := context.Background()
	db, commands, previous := seedRuntimeObservation(t, ctx)
	file, err := commands.ConfigurationFile(ctx)
	if err != nil {
		t.Fatal(err)
	}
	saved, err := commands.SaveConfigurationFile(ctx, application.ConfigurationFileWrite{Revision: file.Revision, Content: `{"log":{"level":"debug"}}`})
	if err != nil {
		t.Fatal(err)
	}
	queued, err := commands.QueueRuntimeRestart(ctx)
	if err != nil {
		t.Fatal(err)
	}
	task, err := db.ClaimTask(ctx, store.ClaimTaskInput{Lane: store.TaskLaneRuntime, LeaseOwner: "launch-review", Now: time.Now().UTC(), LeaseDuration: time.Minute})
	if err != nil || task == nil || task.ID != queued.ID {
		t.Fatalf("claim: %+v %v", task, err)
	}
	manager := &configurationLaunchManager{}
	manager.live = coreruntime.LiveIdentity{Running: true, PID: previous.PID, BundleID: previous.ActivationBundleID}
	resolver := &fakeRuntimeIdentityResolver{startToken: "new-incarnation"}
	services := &runtimeServices{database: db, commands: commands, manager: manager, identity: resolver}
	result, err := runtimeIntentHandler(services)(ctx, *task, successfulTaskControl{})
	if err != nil {
		t.Fatal(err)
	}
	completed, err := db.CompleteTask(ctx, task.ID, task.LeaseOwner, time.Now().UTC(), store.TaskCompletion{Succeeded: true, Result: result.Payload, Runtime: result.Runtime})
	if err != nil {
		t.Fatal(err)
	}
	if completed.Status != store.TaskStatusSucceeded || manager.launches != 1 || manager.checkedBundle == nil {
		t.Fatalf("incomplete lifecycle: %+v", completed)
	}
	observation, err := db.RuntimeObservation(ctx)
	if err != nil {
		t.Fatal(err)
	}
	resolver.identity = application.RuntimeIdentity{PID: observation.PID, ProcessStartToken: observation.ProcessStartToken, ActivationBundleID: observation.ActivationBundleID}
	status, err := application.FromStoreWithRuntimeResolver(db, resolver).RuntimeStatus(ctx)
	if err != nil || status.LoadedCanonicalRevisionID != saved.CanonicalRevisionID {
		t.Fatalf("loaded identity: %+v %v", status, err)
	}
	file, err = commands.SaveConfigurationFile(ctx, application.ConfigurationFileWrite{Revision: saved.Revision, Content: `{"log":{"level":"trace"}}`})
	if err != nil {
		t.Fatal(err)
	}
	status, err = application.FromStoreWithRuntimeResolver(db, resolver).RuntimeStatus(ctx)
	if err != nil || status.LoadedCanonicalRevisionID == file.CanonicalRevisionID || status.LoadedCanonicalRevisionID != saved.CanonicalRevisionID {
		t.Fatalf("save changed loaded identity: %+v %v", status, err)
	}
}

func TestStartDoesNotReloadAnAlreadyRunningProcess(t *testing.T) {
	ctx := context.Background()
	db, commands, previous := seedRuntimeObservation(t, ctx)
	unchanged, err := commands.QueueRuntimeStart(ctx)
	if err != nil || unchanged.ActivationBundleID != previous.ActivationBundleID || unchanged.StartupArtifactID != "" {
		t.Fatalf("unchanged start: %+v %v", unchanged, err)
	}
	file, err := commands.ConfigurationFile(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := commands.SaveConfigurationFile(ctx, application.ConfigurationFileWrite{Revision: file.Revision, Content: `{"log":{"level":"debug"}}`}); err != nil {
		t.Fatal(err)
	}
	if _, err := commands.QueueRuntimeStart(ctx); err != nil {
		t.Fatal(err)
	}
	task, err := db.ClaimTask(ctx, store.ClaimTaskInput{Lane: store.TaskLaneRuntime, LeaseOwner: "start-review", Now: time.Now().UTC(), LeaseDuration: time.Minute})
	if err != nil || task == nil {
		t.Fatalf("claim: %+v %v", task, err)
	}
	manager := &configurationLaunchManager{}
	manager.live = coreruntime.LiveIdentity{Running: true, PID: previous.PID, BundleID: previous.ActivationBundleID}
	services := &runtimeServices{database: db, commands: commands, manager: manager}
	if _, err := runtimeIntentHandler(services)(ctx, *task, successfulTaskControl{}); err == nil {
		t.Fatal("start silently reloaded a changed configuration")
	}
	if manager.launches != 0 || manager.stopCalls != 0 {
		t.Fatal("start changed running process")
	}
}
