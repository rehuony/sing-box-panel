// SPDX-License-Identifier: GPL-3.0-or-later

package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/rehuony/sing-box-panel/internal/application"
	"github.com/rehuony/sing-box-panel/internal/canonical"
	"github.com/rehuony/sing-box-panel/internal/store"
)

func TestRuntimeAndManualHTTPRoutesUseApplicationServices(t *testing.T) {
	handler, database := newCoreHTTPFixture(t)
	core := seedCoreHTTPArtifact(t, database)
	revision, startup := seedRuntimeHTTPStartup(t, database, core)

	statusResponse := authenticatedRequest(handler, http.MethodGet, "/api/v1/core/status", "", "")
	if statusResponse.Code != http.StatusOK {
		t.Fatalf("runtime status=%d body=%s", statusResponse.Code, statusResponse.Body.String())
	}
	var runtimeStatus application.RuntimeStatus
	if err := json.Unmarshal(statusResponse.Body.Bytes(), &runtimeStatus); err != nil {
		t.Fatal(err)
	}
	if runtimeStatus.ObservationState != "stopped" || runtimeStatus.Running != nil {
		t.Fatalf("runtime status = %+v", runtimeStatus)
	}

	checkResponse := authenticatedRequest(
		handler,
		http.MethodPost,
		"/api/v1/core/check",
		`{"startup_artifact_id":"`+startup.ID+`"}`,
		"",
	)
	assertQueuedCoreHTTPTask(t, checkResponse, "startup-check")
	if _, err := database.CompleteStartupArtifactCheck(
		context.Background(), startup.ID, true, json.RawMessage(`[]`), time.Now().UTC(),
	); err != nil {
		t.Fatal(err)
	}

	activateResponse := authenticatedRequest(
		handler,
		http.MethodPost,
		"/api/v1/core/activate",
		`{"startup_artifact_id":"`+startup.ID+`","monitoring_tier":"process_only"}`,
		"",
	)
	if activateResponse.Code != http.StatusAccepted {
		t.Fatalf("activate status=%d body=%s", activateResponse.Code, activateResponse.Body.String())
	}
	var activated struct {
		Activation application.ActivationSummary `json:"activation"`
		Task       application.Task              `json:"task"`
	}
	if err := json.Unmarshal(activateResponse.Body.Bytes(), &activated); err != nil {
		t.Fatal(err)
	}
	if activated.Activation.StartupArtifactID != startup.ID ||
		activated.Task.Kind != "runtime-apply" || activated.Task.Status != store.TaskStatusQueued {
		t.Fatalf("activation response = %+v", activated)
	}
	aliasResponse := authenticatedRequest(
		handler,
		http.MethodPost,
		"/api/v1/config/apply",
		`{"startup_artifact_id":"`+startup.ID+`"}`,
		"",
	)
	if aliasResponse.Code != http.StatusAccepted ||
		!strings.Contains(aliasResponse.Body.String(), activated.Activation.ActivationBundleID) {
		t.Fatalf("config apply alias status=%d body=%s", aliasResponse.Code, aliasResponse.Body.String())
	}

	startResponse := authenticatedRequest(handler, http.MethodPost, "/api/v1/core/start", "", "")
	assertCoreHTTPProblem(t, startResponse, http.StatusConflict, "no_applied_bundle")
	stopResponse := authenticatedRequest(handler, http.MethodPost, "/api/v1/core/stop", "", "")
	assertQueuedCoreHTTPTask(t, stopResponse, "runtime-stop")

	manualBytes := "{\n  // preserved comment\n  \"log\": {\"level\": \"info\"}\n}\n"
	manualPreviewResponse := authenticatedRequest(
		handler,
		http.MethodPost,
		"/api/v1/config/manual/preview?core_version=1.13.19&core_artifact_id="+core.ID,
		manualBytes,
		`"`+revision.ID+`"`,
	)
	if manualPreviewResponse.Code != http.StatusOK || strings.Contains(manualPreviewResponse.Body.String(), "preserved comment") {
		t.Fatalf("manual preview status=%d body=%s", manualPreviewResponse.Code, manualPreviewResponse.Body.String())
	}
	var manualPreview application.ManualReplacePreview
	if err := json.Unmarshal(manualPreviewResponse.Body.Bytes(), &manualPreview); err != nil {
		t.Fatal(err)
	}
	if manualPreview.Reverse.Available || manualPreview.Reverse.ReasonCode != "capability_pin_unavailable" ||
		len(manualPreview.Reverse.ResidualPaths) != 1 || manualPreview.Reverse.ResidualPaths[0] != "/log/level" {
		t.Fatalf("manual preview = %+v", manualPreview)
	}
	manualResponse := authenticatedRequest(
		handler,
		http.MethodPut,
		"/api/v1/config/manual?core_version=1.13.19&core_artifact_id="+core.ID,
		manualBytes,
		`"`+revision.ID+`"`,
	)
	if manualResponse.Code != http.StatusAccepted {
		t.Fatalf("manual replace status=%d body=%s", manualResponse.Code, manualResponse.Body.String())
	}
	var saved application.ManualSave
	if err := json.Unmarshal(manualResponse.Body.Bytes(), &saved); err != nil {
		t.Fatal(err)
	}
	if saved.Artifact.Raw != manualBytes || saved.Resolution.ExactVersion != "1.13.19" ||
		saved.Resolution.Source != "explicit" || saved.Task.Kind != "startup-check" ||
		saved.Preview.ConfigSHA256 != manualPreview.ConfigSHA256 {
		t.Fatalf("manual save = %+v", saved)
	}
	if manualResponse.Header().Get("ETag") != `"`+revision.ID+`"` {
		t.Fatalf("manual ETag = %q", manualResponse.Header().Get("ETag"))
	}

	showResponse := authenticatedRequest(handler, http.MethodGet, "/api/v1/config/manual/"+saved.Artifact.ID, "", "")
	if showResponse.Code != http.StatusOK || !strings.Contains(showResponse.Body.String(), "preserved comment") {
		t.Fatalf("manual show status=%d body=%s", showResponse.Code, showResponse.Body.String())
	}
	listResponse := authenticatedRequest(
		handler,
		http.MethodGet,
		"/api/v1/config/manual?core_version=1.13.19&core_artifact_id="+core.ID+"&limit=10",
		"",
		"",
	)
	if listResponse.Code != http.StatusOK || !strings.Contains(listResponse.Body.String(), `"source":"explicit"`) {
		t.Fatalf("manual list status=%d body=%s", listResponse.Code, listResponse.Body.String())
	}
	discardResponse := authenticatedRequest(handler, http.MethodDelete, "/api/v1/config/manual/"+saved.Artifact.ID, "", "")
	if discardResponse.Code != http.StatusOK || !strings.Contains(discardResponse.Body.String(), `"state":"stale"`) {
		t.Fatalf("manual discard status=%d body=%s", discardResponse.Code, discardResponse.Body.String())
	}

	omittedVersion := authenticatedRequest(handler, http.MethodGet, "/api/v1/config/manual", "", "")
	assertCoreHTTPProblem(t, omittedVersion, http.StatusConflict, "core_not_running")
}

func TestRuntimeAndManualHTTPRejectAmbiguousInputs(t *testing.T) {
	handler, database := newCoreHTTPFixture(t)
	core := seedCoreHTTPArtifact(t, database)
	revision, startup := seedRuntimeHTTPStartup(t, database, core)

	tests := []struct {
		name       string
		method     string
		target     string
		body       string
		ifMatch    string
		wantStatus int
		wantCode   string
	}{
		{
			name: "duplicate check field", method: http.MethodPost, target: "/api/v1/core/check",
			body:       `{"startup_artifact_id":"one","startup_artifact_id":"two"}`,
			wantStatus: http.StatusUnprocessableEntity, wantCode: "invalid_json",
		},
		{
			name: "unknown monitoring", method: http.MethodPost, target: "/api/v1/core/activate",
			body:       `{"startup_artifact_id":"` + startup.ID + `","monitoring_tier":"invented"}`,
			wantStatus: http.StatusUnprocessableEntity, wantCode: "activation_request_invalid",
		},
		{
			name: "lifecycle body", method: http.MethodPost, target: "/api/v1/core/restart", body: `{}`,
			wantStatus: http.StatusUnprocessableEntity, wantCode: "request_body_not_allowed",
		},
		{
			name: "manual base required", method: http.MethodPut,
			target: "/api/v1/config/manual?core_version=1.13.19&core_artifact_id=" + core.ID,
			body:   `{}`, wantStatus: http.StatusPreconditionRequired, wantCode: "base_revision_required",
		},
		{
			name: "manual preview base required", method: http.MethodPost,
			target: "/api/v1/config/manual/preview?core_version=1.13.19&core_artifact_id=" + core.ID,
			body:   `{}`, wantStatus: http.StatusPreconditionRequired, wantCode: "base_revision_required",
		},
		{
			name: "invalid manual JSONC", method: http.MethodPut,
			target: "/api/v1/config/manual?core_version=1.13.19&core_artifact_id=" + core.ID,
			body:   `{`, ifMatch: `"` + revision.ID + `"`,
			wantStatus: http.StatusUnprocessableEntity, wantCode: "manual_json_invalid",
		},
		{
			name: "nested manual path", method: http.MethodGet, target: "/api/v1/config/manual/a/b",
			wantStatus: http.StatusMethodNotAllowed, wantCode: "method_not_allowed",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			response := authenticatedRequest(handler, test.method, test.target, test.body, test.ifMatch)
			assertCoreHTTPProblem(t, response, test.wantStatus, test.wantCode)
		})
	}
}

func seedRuntimeHTTPStartup(
	t *testing.T,
	database *store.Store,
	core store.CoreArtifact,
) (store.CanonicalRevision, store.StartupArtifact) {
	t.Helper()
	createdAt := time.Date(2026, time.August, 26, 14, 0, 0, 0, time.UTC)
	revision, err := database.SaveCanonicalRevisionAndTask(context.Background(), "", store.NewCanonicalRevision{
		ID: "revision_runtime_http", SchemaVersion: canonical.SchemaVersion, Document: canonical.Empty().CanonicalJSON(),
		CommandID: "command_runtime_http", CreatedAt: createdAt,
	}, store.NewTask{
		ID: "task_runtime_http", Lane: store.TaskLaneMaintenance,
		Kind: "canonical-saved", CreatedAt: createdAt,
	})
	if err != nil {
		t.Fatal(err)
	}
	startup, err := database.CreateStartupArtifact(context.Background(), store.StartupArtifact{
		ID: "startup_runtime_http", Kind: store.StartupArtifactManual,
		CanonicalRevisionID: revision.ID, ExactCoreVersion: core.ExactVersion,
		RendererVersion: "manual-json-v1", CoreArtifactID: core.ID,
		ConfigBytes: []byte(`{}`), Diagnostics: json.RawMessage(`[]`),
		CreatedAt: createdAt.Add(time.Second),
	})
	if err != nil {
		t.Fatal(err)
	}
	return revision, startup
}
