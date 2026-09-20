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
