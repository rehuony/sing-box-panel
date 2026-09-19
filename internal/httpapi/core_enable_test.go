// SPDX-License-Identifier: GPL-3.0-or-later

package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"runtime"
	"testing"

	"github.com/rehuony/sing-box-panel/internal/application"
	"github.com/rehuony/sing-box-panel/internal/store"
)

func TestSystemPlatformDescribesDeployedBinary(t *testing.T) {
	handler, _ := newCoreHTTPFixture(t)
	response := authenticatedRequest(handler, http.MethodGet, "/api/v1/system/status", "", "")
	var status SystemStatus
	if response.Code != 200 || json.Unmarshal(response.Body.Bytes(), &status) != nil || status.Platform.OS != runtime.GOOS || status.Platform.Arch != runtime.GOARCH {
		t.Fatal(response.Code, response.Body.String())
	}
}
func TestEnableCoreRejectsPlatformOrTrustBeforeQueueing(t *testing.T) {
	handler, db := newCoreHTTPFixture(t)
	artifact := seedCoreHTTPArtifact(t, db)
	// Both artifacts are rejected on every test host: one has a different CPU,
	// the other is quarantined. No runtime intention may be queued.
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
	artifact.ID = "quarantined"
	artifact.Architecture = runtime.GOARCH
	artifact.AssetID = 5002
	artifact.VerificationState = store.CoreArtifactQuarantined
	if _, err := db.UpsertCoreArtifact(context.Background(), artifact); err != nil {
		t.Fatal(err)
	}
	if _, err := handler.commands.EnableCore(context.Background(), artifact.ID); !errors.Is(err, application.ErrCoreArtifactVerificationBlocked) {
		t.Fatalf("quarantined artifact must fail trust verification: %v", err)
	}
	response = authenticatedRequest(handler, http.MethodPost, "/api/v1/core/artifacts/"+artifact.ID+"/enable", "", "")
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
