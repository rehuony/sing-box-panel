// SPDX-License-Identifier: GPL-3.0-or-later

package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/rehuony/sing-box-panel/internal/application"
	"github.com/rehuony/sing-box-panel/internal/configuration"
	"github.com/rehuony/sing-box-panel/internal/store"
)

func TestRuntimeHistoryHTTPUsesStablePairedCursor(t *testing.T) {
	handler, database := newCoreHTTPFixture(t)
	initialized, err := database.LatestRuntimeTransition(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	now := initialized.OccurredAt.Add(time.Minute)
	for index, reason := range []string{"startup_reconciled_stopped", "stop_succeeded"} {
		if _, err := database.AppendRuntimeTransition(context.Background(), store.RuntimeTransitionInput{
			DedupeKey:  "runtime-http-" + strconv.Itoa(index),
			State:      store.RuntimeTransitionStopped,
			Reason:     reason,
			OccurredAt: now.Add(time.Duration(index) * time.Second),
		}); err != nil {
			t.Fatal(err)
		}
	}
	query := url.Values{
		"state": []string{string(store.RuntimeTransitionStopped)},
		"limit": []string{"1"},
	}
	response := authenticatedRequest(
		handler,
		http.MethodGet,
		"/api/v1/core/runtime/history?"+query.Encode(),
		"",
		"",
	)
	if response.Code != http.StatusOK {
		t.Fatalf("runtime history status=%d body=%s", response.Code, response.Body.String())
	}
	var page application.RuntimeHistoryPage
	if err := json.Unmarshal(response.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 || page.Items[0].Reason != "stop_succeeded" ||
		page.Next == nil || page.HistoryStartedAt.IsZero() {
		t.Fatalf("runtime history page = %+v", page)
	}
	nextQuery := url.Values{
		"state":       []string{string(store.RuntimeTransitionStopped)},
		"before_time": []string{page.Next.OccurredAt.Format(time.RFC3339Nano)},
		"before_id":   []string{strconv.FormatInt(page.Next.ID, 10)},
		"limit":       []string{"1"},
	}
	nextResponse := authenticatedRequest(
		handler,
		http.MethodGet,
		"/api/v1/core/runtime/history?"+nextQuery.Encode(),
		"",
		"",
	)
	if nextResponse.Code != http.StatusOK {
		t.Fatalf("next runtime history status=%d body=%s", nextResponse.Code, nextResponse.Body.String())
	}
	var next application.RuntimeHistoryPage
	if err := json.Unmarshal(nextResponse.Body.Bytes(), &next); err != nil {
		t.Fatal(err)
	}
	if len(next.Items) != 1 || next.Items[0].Reason != "startup_reconciled_stopped" || next.Next != nil {
		t.Fatalf("next runtime history page = %+v", next)
	}

	for _, target := range []string{
		"/api/v1/core/runtime/history?before_time=" + url.QueryEscape(now.Format(time.RFC3339Nano)),
		"/api/v1/core/runtime/history?state=starting",
	} {
		invalid := authenticatedRequest(handler, http.MethodGet, target, "", "")
		if invalid.Code != http.StatusBadRequest {
			t.Fatalf("invalid runtime history %q status=%d body=%s", target, invalid.Code, invalid.Body.String())
		}
	}
}

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
	if previewResponse.Code != http.StatusOK || !strings.Contains(previewResponse.Body.String(), `"structured":false`) ||
		!strings.Contains(previewResponse.Body.String(), `"config":{}`) {
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
	if compiled.Task.Kind != store.TaskKindStartupCheck {
		t.Fatalf("compile = %+v", compiled)
	}

	checkResponse := authenticatedRequest(
		handler,
		http.MethodPost,
		"/api/v1/core/check",
		`{"startup_artifact_id":"`+startup.ID+`"}`,
		"",
	)
	assertQueuedCoreHTTPTask(t, checkResponse, store.TaskKindStartupCheck)
	if _, err := database.CompleteStartupArtifactCheck(
		context.Background(), startup.ID, true, time.Now().UTC(),
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
		activated.Task.Kind != store.TaskKindRuntimeApply || activated.Task.Status != store.TaskStatusQueued {
		t.Fatalf("activation response = %+v", activated)
	}

	startResponse := authenticatedRequest(handler, http.MethodPost, "/api/v1/core/start", "", "")
	assertCoreHTTPProblem(t, startResponse, http.StatusConflict, "no_applied_bundle")
	stopResponse := authenticatedRequest(handler, http.MethodPost, "/api/v1/core/stop", "", "")
	assertQueuedCoreHTTPTask(t, stopResponse, store.TaskKindRuntimeStop)
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
			name: "rollback evidence required", method: http.MethodPost, target: "/api/v1/core/rollback", body: `{}`,
			wantStatus: http.StatusUnprocessableEntity, wantCode: "rollback_bundle_id_invalid",
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
		ID: "revision_runtime_http", SchemaVersion: configuration.SchemaVersion,
		Document: configuration.Empty().CanonicalJSON(), CommandID: "command_runtime_http", CreatedAt: createdAt,
	}, store.NewTask{
		ID: "task_runtime_http", Lane: store.TaskLaneMaintenance,
		Kind: store.TaskKindCanonicalSaved, CreatedAt: createdAt,
	})
	if err != nil {
		t.Fatal(err)
	}
	startup, err := database.CreateStartupArtifact(context.Background(), store.StartupArtifact{
		ID: "startup_runtime_http", CanonicalRevisionID: revision.ID,
		ExactCoreVersion: core.ExactVersion, CoreArtifactID: core.ID, ConfigBytes: []byte(`{}`),
		CreatedAt: createdAt.Add(time.Second),
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
		FeatureFingerprint: json.RawMessage(`{"status":"reported","features":["badlinkname","tfogo_checklinkname0","with_acme","with_ccm","with_clash_api","with_dhcp","with_gvisor","with_naive_outbound","with_ocm","with_purego","with_quic","with_tailscale","with_utls","with_wireguard"]}`),
		VerificationState:  store.CoreArtifactVerified,
		CreatedAt:          time.Date(2026, time.August, 26, 13, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}
	return artifact
}
