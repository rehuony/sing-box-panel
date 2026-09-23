// SPDX-License-Identifier: GPL-3.0-or-later

package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"runtime"
	"testing"
)

func TestSystemPlatformDescribesDeployedBinary(t *testing.T) {
	handler, _ := newCoreHTTPFixture(t)
	response := authenticatedRequest(handler, http.MethodGet, "/api/v1/system/status", "", "")
	var status SystemStatus
	if response.Code != 200 || json.Unmarshal(response.Body.Bytes(), &status) != nil || status.Platform.OS != runtime.GOOS || status.Platform.Arch != runtime.GOARCH {
		t.Fatal(response.Code, response.Body.String())
	}
}
func TestEnableCoreRejectsWrongPlatformBeforeQueueing(t *testing.T) {
	handler, db := newCoreHTTPFixture(t)
	artifact := seedCoreHTTPArtifact(t, db)
	// An incompatible CPU must be rejected before any runtime intent is queued.
	if runtime.GOARCH == "amd64" {
		artifact.Architecture = "arm64"
	} else {
		artifact.Architecture = "amd64"
	}
	artifact.ID = "wrong-platform"
	artifact.AssetID = 5001
	if _, err := db.UpsertCoreArtifact(context.Background(), artifact); err != nil {
		t.Fatal(err)
	}
	response := authenticatedRequest(handler, http.MethodPost, "/api/v1/core/artifacts/"+artifact.ID+"/enable", "", "")
	if response.Code != http.StatusConflict {
		t.Fatal(response.Code, response.Body.String())
	}
	response = authenticatedRequest(handler, http.MethodPost, "/api/v1/core/artifacts/missing/enable", "", "")
	if response.Code != http.StatusNotFound {
		t.Fatal(response.Code, response.Body.String())
	}
	bootstrap, err := db.Bootstrap(context.Background())
	if err != nil || bootstrap.Hub.DesiredRunning || bootstrap.Hub.TargetGeneration != 0 {
		t.Fatal("rejected enable changed runtime intent", err, bootstrap.Hub)
	}
}

func TestDisableCoreRejectsMissingOrUnselectedArtifacts(t *testing.T) {
	handler, db := newCoreHTTPFixture(t)
	artifact := seedCoreHTTPArtifact(t, db)
	for _, test := range []struct {
		id     string
		status int
	}{{"missing", http.StatusNotFound}, {artifact.ID, http.StatusConflict}} {
		response := authenticatedRequest(handler, http.MethodPost, "/api/v1/core/artifacts/"+test.id+"/disable", "", "")
		if response.Code != test.status {
			t.Fatalf("disable %s: %d %s", test.id, response.Code, response.Body.String())
		}
	}
	bootstrap, err := db.Bootstrap(context.Background())
	if err != nil || bootstrap.Hub.TargetGeneration != 0 {
		t.Fatalf("rejected disable changed runtime intent: %+v %v", bootstrap.Hub, err)
	}
}

func TestEnableCoreRequiresInitializedConfiguration(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("enabling installed cores requires Linux")
	}
	handler, db := newCoreHTTPFixture(t)
	artifact := seedCoreHTTPArtifact(t, db)
	artifact.ID = "native-core"
	artifact.AssetID = 3002
	artifact.Architecture = runtime.GOARCH
	if _, err := db.UpsertCoreArtifact(t.Context(), artifact); err != nil {
		t.Fatal(err)
	}
	response := authenticatedRequest(handler, http.MethodPost, "/api/v1/core/artifacts/"+artifact.ID+"/enable", "", "")
	assertCoreHTTPProblem(t, response, http.StatusConflict, "configuration_not_saved")
	file, err := db.ConfigurationFile(t.Context())
	if err != nil || file.Revision != 0 {
		t.Fatalf("enable created a configuration: %+v %v", file, err)
	}
	bootstrap, err := db.Bootstrap(t.Context())
	if err != nil || bootstrap.Hub.TargetGeneration != 0 || bootstrap.Hub.AppliedBundleID != "" {
		t.Fatalf("enable changed runtime state: %+v %v", bootstrap.Hub, err)
	}
	if err := handler.commands.InitializeConfigurationFile(t.Context()); err != nil {
		t.Fatal(err)
	}
	response = authenticatedRequest(handler, http.MethodPost, "/api/v1/core/artifacts/"+artifact.ID+"/enable", "", "")
	if response.Code != http.StatusOK {
		t.Fatalf("enable after saving configuration: %d %s", response.Code, response.Body.String())
	}
}

func TestConfigurationOperationsExplainMissingConfiguration(t *testing.T) {
	for _, action := range []string{"preview", "compile"} {
		t.Run(action, func(t *testing.T) {
			handler, db := newCoreHTTPFixture(t)
			artifact := seedCoreHTTPArtifact(t, db)
			response := authenticatedRequest(handler, http.MethodPost, "/api/v1/config/"+action,
				`{"core_artifact_id":"`+artifact.ID+`"}`, "")
			assertCoreHTTPProblem(t, response, http.StatusConflict, "configuration_not_saved")
		})
	}
}
