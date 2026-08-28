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

func TestRuntimeAndConfigurationHTTPRoutesUseApplicationServices(t *testing.T) {
	handler, database := newCoreHTTPFixture(t)
	core := seedSupportedRuntimeHTTPCore(t, database)
	_, startup := seedRuntimeHTTPStartup(t, database, core)

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

	previewResponse := authenticatedRequest(
		handler,
		http.MethodPost,
		"/api/v1/config/preview",
		`{"core_artifact_id":"`+core.ID+`"}`,
		"",
	)
	if previewResponse.Code != http.StatusOK || !strings.Contains(previewResponse.Body.String(), `"adapter_id":"sing-box/v1_13_19/official-linux-plain"`) {
		t.Fatalf("preview status=%d body=%s", previewResponse.Code, previewResponse.Body.String())
	}
	compileResponse := authenticatedRequest(
		handler,
		http.MethodPost,
		"/api/v1/config/compile",
		`{"core_artifact_id":"`+core.ID+`"}`,
		"",
	)
	if compileResponse.Code != http.StatusAccepted {
		t.Fatalf("compile status=%d body=%s", compileResponse.Code, compileResponse.Body.String())
	}
	var compiled application.ConfigurationCompile
	if err := json.Unmarshal(compileResponse.Body.Bytes(), &compiled); err != nil {
		t.Fatal(err)
	}
	if compiled.Artifact.AdapterID != "sing-box/v1_13_19/official-linux-plain" || compiled.Artifact.AdapterRevision != "2" || compiled.Task.Kind != "startup-check" {
		t.Fatalf("compile = %+v", compiled)
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

	startResponse := authenticatedRequest(handler, http.MethodPost, "/api/v1/core/start", "", "")
	assertCoreHTTPProblem(t, startResponse, http.StatusConflict, "no_applied_bundle")
	stopResponse := authenticatedRequest(handler, http.MethodPost, "/api/v1/core/stop", "", "")
	assertQueuedCoreHTTPTask(t, stopResponse, "runtime-stop")
}

func TestRuntimeAndConfigurationHTTPRejectAmbiguousInputs(t *testing.T) {
	handler, database := newCoreHTTPFixture(t)
	core := seedSupportedRuntimeHTTPCore(t, database)
	_, startup := seedRuntimeHTTPStartup(t, database, core)

	tests := []struct {
		name       string
		method     string
		target     string
		body       string
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
			name: "preview missing core", method: http.MethodPost, target: "/api/v1/config/preview", body: `{}`,
			wantStatus: http.StatusUnprocessableEntity, wantCode: "configuration_preview_invalid",
		},
		{
			name: "compile unknown field", method: http.MethodPost, target: "/api/v1/config/compile",
			body:       `{"core_artifact_id":"` + core.ID + `","raw":{}}`,
			wantStatus: http.StatusUnprocessableEntity, wantCode: "invalid_json",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			response := authenticatedRequest(handler, test.method, test.target, test.body, "")
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
		ID: "revision_runtime_http", SchemaVersion: canonical.SchemaVersionV2,
		Document: canonical.EmptyV2().CanonicalJSON(), CommandID: "command_runtime_http", CreatedAt: createdAt,
	}, store.NewTask{
		ID: "task_runtime_http", Lane: store.TaskLaneMaintenance,
		Kind: "canonical-saved", CreatedAt: createdAt,
	})
	if err != nil {
		t.Fatal(err)
	}
	startup, err := database.CreateStartupArtifact(context.Background(), store.StartupArtifact{
		ID: "startup_runtime_http", CanonicalRevisionID: revision.ID,
		ExactCoreVersion: core.ExactVersion, AdapterID: "sing-box/v1_13_19/official-linux-plain",
		AdapterRevision: "2", CoreArtifactID: core.ID, ConfigBytes: []byte(`{}`),
		Diagnostics: json.RawMessage(`[]`), CreatedAt: createdAt.Add(time.Second),
	})
	if err != nil {
		t.Fatal(err)
	}
	return revision, startup
}

func seedSupportedRuntimeHTTPCore(t *testing.T, database *store.Store) store.CoreArtifact {
	t.Helper()
	artifact, err := database.UpsertCoreArtifact(context.Background(), store.CoreArtifact{
		ID: "core_runtime_http", ExactVersion: "1.13.19", OperatingSystem: "linux", Architecture: "arm64", Variant: "plain",
		SourceKind: store.CoreArtifactSourceUserVerified, UserSource: "runtime HTTP fixture",
		ArchiveSHA256: strings.Repeat("ca", 32), BinarySHA256: strings.Repeat("cb", 32),
		BinaryPath: "/var/lib/sing-box-panel/artifacts/core_runtime_http/sing-box", ReportedVersion: "1.13.19",
		FeatureFingerprint: json.RawMessage(`{"status":"reported","features":["badlinkname","tfogo_checklinkname0","with_acme","with_ccm","with_clash_api","with_dhcp","with_gvisor","with_ocm","with_quic","with_tailscale","with_utls","with_wireguard"]}`),
		VerificationState:  store.CoreArtifactVerified,
		CreatedAt:          time.Date(2026, time.August, 26, 13, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}
	return artifact
}
