// SPDX-License-Identifier: GPL-3.0-or-later
package server

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/rehuony/sing-box-panel/internal/application"
	coreruntime "github.com/rehuony/sing-box-panel/internal/runtime"
	"github.com/rehuony/sing-box-panel/internal/store"
)

func TestRuntimeControlLogsTargetAndDuration(t *testing.T) {
	ctx := t.Context()
	database := openRunnerStore(t, ctx)
	services := &runtimeServices{
		database: database, commands: application.FromStore(database), manager: &fakeRuntimeManager{},
	}
	_, err := services.ExecuteRuntime(ctx, application.RuntimeRequest{Action: "check", StartupArtifactID: "startup_missing"})
	if err == nil {
		t.Fatal("checking a missing startup artifact succeeded")
	}
	page, err := database.ListPanelLogs(ctx, store.PanelLogFilter{SearchCodes: []string{"runtime.check.failed"}})
	if err != nil || len(page.Items) != 1 {
		t.Fatalf("failure log = %+v, %v", page, err)
	}
	var metadata struct {
		Target   string `json:"startup_artifact_id"`
		Duration *int64 `json:"duration_ms"`
		Error    string `json:"error_code"`
	}
	if err := json.Unmarshal(page.Items[0].Metadata, &metadata); err != nil {
		t.Fatal(err)
	}
	if metadata.Target != "startup_missing" || metadata.Duration == nil || *metadata.Duration < 0 || metadata.Error != "startup_artifact_not_found" {
		t.Fatalf("failure context = %+v", metadata)
	}
	if _, err := services.ExecuteRuntime(ctx, application.RuntimeRequest{Action: "stop"}); err != nil {
		t.Fatal(err)
	}
	page, err = database.ListPanelLogs(ctx, store.PanelLogFilter{SearchCodes: []string{"runtime.stop.completed"}})
	if err != nil || len(page.Items) != 1 {
		t.Fatalf("completion log = %+v, %v", page, err)
	}
	var completed struct {
		Generation int64  `json:"generation"`
		Duration   *int64 `json:"duration_ms"`
	}
	if err := json.Unmarshal(page.Items[0].Metadata, &completed); err != nil {
		t.Fatal(err)
	}
	if completed.Generation <= 0 || completed.Duration == nil || *completed.Duration < 0 {
		t.Fatalf("completion context = %+v", completed)
	}
}

func TestRuntimeControlLogsCheckedTarget(t *testing.T) {
	for _, scenario := range []string{"start", "restart", "restart_failed", "check_failed"} {
		t.Run(scenario, func(t *testing.T) {
			ctx := t.Context()
			database, commands, previous := seedRuntimeObservation(t, ctx)
			file, err := commands.ConfigurationFile(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := commands.SaveConfigurationFile(ctx, application.ConfigurationFileWrite{Revision: file.Revision, Content: `{"log":{"level":"debug"}}`}); err != nil {
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
			if _, err := services.ExecuteRuntime(ctx, application.RuntimeRequest{Action: action}); !errors.Is(err, wantErr) {
				t.Fatalf("execute = %v, want %v", err, wantErr)
			}
			page, err := commands.PanelLogs(ctx, store.PanelLogFilter{SearchCodes: []string{"runtime." + action + "." + outcome}})
			if err != nil || len(page.Items) != 1 {
				t.Fatalf("operation log: %+v %v", page, err)
			}
			var metadata struct {
				Bundle     string `json:"activation_bundle_id"`
				Startup    string `json:"startup_artifact_id"`
				Generation int64  `json:"generation"`
				ErrorCode  string `json:"error_code"`
			}
			if err := json.Unmarshal(page.Items[0].Metadata, &metadata); err != nil {
				t.Fatal(err)
			}
			if metadata.Startup == "" || metadata.Generation <= 0 || metadata.ErrorCode != failureCode {
				t.Fatalf("operation lost context: %s", page.Items[0].Metadata)
			}
			if scenario == "check_failed" {
				if metadata.Bundle != "" {
					t.Fatal("failed preflight must not claim a checked bundle")
				}
				return
			}
			bundle, err := database.GetActivationBundle(ctx, metadata.Bundle)
			if err != nil || bundle.ID == previous.ActivationBundleID || bundle.StartupArtifactID != metadata.Startup {
				t.Fatalf("operation did not retain checked target: %s %v", page.Items[0].Metadata, err)
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
