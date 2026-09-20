// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/rehuony/sing-box-panel/internal/application"
	"github.com/rehuony/sing-box-panel/internal/artifactstore"
	"github.com/rehuony/sing-box-panel/internal/store"
)

func TestDisableCoreClearsSelectionOnlyAfterSuccessfulStop(t *testing.T) {
	for _, scenario := range []string{"running", "stopped", "stop-failed", "canceled", "superseded", "ordinary-stop"} {
		t.Run(scenario, func(t *testing.T) {
			ctx := context.Background()
			db, commands, previous := seedRuntimeObservation(t, ctx)
			if scenario == "stopped" {
				if _, err := db.ClearRuntimeObservation(ctx, previous.PID, previous.ProcessStartToken); err != nil {
					t.Fatal(err)
				}
			}
			var queued application.Task
			var err error
			if scenario == "ordinary-stop" {
				queued, err = commands.QueueRuntimeStop(ctx)
			} else {
				queued, err = commands.DisableCore(ctx, previous.CoreArtifactID)
			}
			if err != nil {
				t.Fatal(err)
			}
			task, err := db.ClaimTask(ctx, store.ClaimTaskInput{Lane: store.TaskLaneRuntime, LeaseOwner: "disable-test", Now: time.Now().UTC(), LeaseDuration: time.Minute})
			if err != nil || task == nil || task.ID != queued.ID {
				t.Fatalf("claim: %+v %v", task, err)
			}
			manager := &fakeRuntimeManager{}
			if scenario == "stop-failed" {
				manager.stopErr = errors.New("stop failed")
			}
			services := &runtimeServices{database: db, commands: commands, manager: manager,
				identity: &fakeRuntimeIdentityResolver{startToken: previous.ProcessStartToken}}
			control := taskControlFunc(func(context.Context) error {
				if scenario == "canceled" {
					_, _, err := db.RequestTaskCancellation(ctx, task.ID, time.Now().UTC())
					return errors.Join(errors.New("canceled before stop"), err)
				}
				return nil
			})
			result, handleErr := runtimeIntentHandler(services)(ctx, *task, control)
			if scenario == "superseded" {
				if _, err := commands.QueueRuntimeStop(ctx); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := db.CompleteTask(ctx, task.ID, task.LeaseOwner, time.Now().UTC(), store.TaskCompletion{Succeeded: handleErr == nil, Runtime: result.Runtime}); err != nil {
				t.Fatal(err)
			}
			bootstrap, err := db.Bootstrap(ctx)
			if err != nil {
				t.Fatal(err)
			}
			cleared := scenario == "running" || scenario == "stopped"
			if !cleared {
				if bootstrap.Hub.AppliedBundleID != previous.ActivationBundleID {
					t.Fatalf("selection was cleared by %s: %+v", scenario, bootstrap.Hub)
				}
				return
			}
			if handleErr != nil || manager.stopCalls != 1 || bootstrap.Hub.AppliedBundleID != "" || bootstrap.Hub.DesiredBundleID != "" || bootstrap.Hub.RollbackBundleID != "" || bootstrap.Hub.DesiredRunning {
				t.Fatalf("disable did not clear selection: %+v %v", bootstrap.Hub, handleErr)
			}
			status, err := application.FromStoreWithRuntimeResolver(db, &fakeRuntimeIdentityResolver{err: application.ErrNoRunningCore}).RuntimeStatus(ctx)
			if err != nil || status.EnabledCore != nil || status.Running != nil {
				t.Fatalf("disabled status: %+v %v", status, err)
			}
			if _, err := commands.QueueRuntimeStart(ctx); !errors.Is(err, store.ErrNoAppliedBundle) {
				t.Fatalf("start reused disabled selection: %v", err)
			}
			if _, err := commands.DisableCore(ctx, previous.CoreArtifactID); !errors.Is(err, store.ErrCoreNotEnabled) {
				t.Fatalf("disabled stale selection: %v", err)
			}
			root, err := filepath.EvalSymlinks(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			artifacts, err := artifactstore.New(artifactstore.Options{Root: root})
			if err != nil {
				t.Fatal(err)
			}
			link := filepath.Join(root, "current")
			if err := os.Symlink(previous.CoreArtifactID, link); err != nil {
				t.Fatal(err)
			}
			if err := commands.SyncEnabledCoreLink(ctx, artifacts); err != nil {
				t.Fatal(err)
			}
			if _, err := os.Lstat(link); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("disable retained current link: %v", err)
			}
		})
	}
}
