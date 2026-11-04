// SPDX-License-Identifier: GPL-3.0-or-later
package service

import (
	"errors"
	"testing"

	coreruntime "github.com/rehuony/sing-box-panel/internal/sing-box-node/runtime"
)

func TestRuntimeControlLogsTargetAndDuration(t *testing.T) {
	ctx := t.Context()
	database := openRunnerStore(t, ctx)
	commands := FromStore(database)
	events := &recordedEvents{}
	commands.events = events.record
	services := &runtimeServices{
		database: database, commands: commands, manager: &fakeRuntimeManager{},
	}
	_, err := services.ExecuteRuntime(ctx, RuntimeRequest{Action: "check", StartupArtifactID: "startup_missing"})
	if err == nil {
		t.Fatal("checking a missing startup artifact succeeded")
	}
	failed := events.withPrefix("runtime.check.failed")
	if len(failed) != 1 {
		t.Fatalf("failure events = %+v", failed)
	}
	metadata := failed[0].Metadata
	if duration, ok := metadata["duration_ms"].(int64); metadata["startup_artifact_id"] != "startup_missing" || !ok || duration < 0 || metadata["error_code"] != "startup_artifact_not_found" {
		t.Fatalf("failure context = %+v", metadata)
	}
	if _, err := services.ExecuteRuntime(ctx, RuntimeRequest{Action: "stop"}); err != nil {
		t.Fatal(err)
	}
	stopped := events.withPrefix("runtime.stop.completed")
	if len(stopped) != 1 {
		t.Fatalf("completion events = %+v", stopped)
	}
	generation, _ := stopped[0].Metadata["generation"].(int64)
	if duration, ok := stopped[0].Metadata["duration_ms"].(int64); generation <= 0 || !ok || duration < 0 {
		t.Fatalf("completion context = %+v", stopped[0].Metadata)
	}
}

func TestRuntimeControlLogsCheckedTarget(t *testing.T) {
	for _, scenario := range []string{"start", "restart", "restart_failed", "check_failed"} {
		t.Run(scenario, func(t *testing.T) {
			ctx := t.Context()
			database, commands, previous := seedRuntimeObservation(t, ctx)
			events := &recordedEvents{}
			commands.events = events.record
			if _, err := receive(ctx, commands, `{"log":{"level":"debug"}}`); err != nil {
				t.Fatal(err)
			}
			manager := &configurationLaunchManager{}
			manager.live = coreruntime.LiveIdentity{Running: true, PID: previous.PID, BundleID: previous.ActivationBundleID}
			action, outcome, failureCode := "restart", "completed", ""
			var wantErr error
			switch scenario {
			case "start":
				action = "start"
				if _, err := database.ClearRuntimeObservation(ctx, previous.PID, previous.ProcessStartToken); err != nil {
					t.Fatal(err)
				}
				manager.live = coreruntime.LiveIdentity{}
			case "restart_failed":
				wantErr = coreruntime.ErrHealthFailed
				manager.launchError = wantErr
				outcome, failureCode = "failed", "core_health_failed"
			case "check_failed":
				wantErr = coreruntime.ErrCheckFailed
				manager.checkError = wantErr
				outcome, failureCode = "failed", "core_check_failed"
			}
			services := &runtimeServices{
				database: database, commands: commands, manager: manager,
				identity: &fakeRuntimeIdentityResolver{startToken: "new-incarnation"},
			}
			if _, err := services.ExecuteRuntime(ctx, RuntimeRequest{Action: action}); !errors.Is(err, wantErr) {
				t.Fatalf("execute = %v, want %v", err, wantErr)
			}
			matched := events.withPrefix("runtime." + action + "." + outcome)
			if len(matched) != 1 {
				t.Fatalf("operation events: %+v", matched)
			}
			details := matched[0].Metadata
			var metadata struct {
				Bundle, Startup, ErrorCode string
				Generation                 int64
			}
			metadata.Bundle, _ = details["activation_bundle_id"].(string)
			metadata.Startup, _ = details["startup_artifact_id"].(string)
			metadata.ErrorCode, _ = details["error_code"].(string)
			metadata.Generation, _ = details["generation"].(int64)
			if metadata.Startup == "" || metadata.Generation <= 0 || metadata.ErrorCode != failureCode {
				t.Fatalf("operation lost context: %v", details)
			}
			if scenario == "check_failed" {
				if metadata.Bundle != "" {
					t.Fatal("failed preflight must not claim a checked bundle")
				}
				return
			}
			bundle, err := database.GetActivationBundle(ctx, metadata.Bundle)
			if err != nil || bundle.ID == previous.ActivationBundleID || bundle.StartupArtifactID != metadata.Startup {
				t.Fatalf("operation did not retain checked target: %v %v", details, err)
			}
			if wantErr == nil {
				observation, err := database.RuntimeObservation(ctx)
				if err != nil || metadata.Bundle != observation.ActivationBundleID {
					t.Fatalf("logged target differs from applied bundle: %+v %v", observation, err)
				}
			}
		})
	}
}
