// SPDX-License-Identifier: GPL-3.0-or-later

package httpapi

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/rehuony/sing-box-panel/internal/application"
	"github.com/rehuony/sing-box-panel/internal/capability"
	"github.com/rehuony/sing-box-panel/internal/coreartifact"
)

func TestCapabilityGenerationHTTPLifecycleRequiresExplicitAcceptance(t *testing.T) {
	handler, _ := newCoreHTTPFixture(t)
	generation, commit, manifestDigest := capabilityHTTPGeneration(t, "1.13.19")

	refresh := authenticatedRequest(handler, http.MethodPost, "/api/v1/core/capability/refresh", string(generation), "")
	if refresh.Code != http.StatusOK {
		t.Fatalf("refresh status=%d body=%s", refresh.Code, refresh.Body.String())
	}
	var refreshed application.CapabilityGenerationRefresh
	if err := json.Unmarshal(refresh.Body.Bytes(), &refreshed); err != nil {
		t.Fatal(err)
	}
	if !refreshed.Created || len(refreshed.Candidates) != 1 || refreshed.Candidates[0].ExactCoreVersion != "1.13.19" {
		t.Fatalf("refresh = %+v", refreshed)
	}

	list := authenticatedRequest(handler, http.MethodGet, "/api/v1/core/capability/generations?limit=1", "", "")
	if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), `"commit_sha":"`+commit+`"`) {
		t.Fatalf("list status=%d body=%s", list.Code, list.Body.String())
	}

	reference := "core_version=1.13.19&commit=" + commit + "&manifest_sha256=" + manifestDigest
	inspect := authenticatedRequest(handler, http.MethodGet, "/api/v1/core/capability/inspect?"+reference, "", "")
	if inspect.Code != http.StatusOK || !strings.Contains(inspect.Body.String(), `"manifest":{`) {
		t.Fatalf("inspect status=%d body=%s", inspect.Code, inspect.Body.String())
	}

	requestBody := `{"exact_core_version":"1.13.19","commit_sha":"` + commit + `","manifest_sha256":"` + manifestDigest + `","accept":false}`
	preview := authenticatedRequest(handler, http.MethodPost, "/api/v1/core/capability/upgrade", requestBody, "")
	if preview.Code != http.StatusOK || !strings.Contains(preview.Body.String(), `"changed":true`) {
		t.Fatalf("preview status=%d body=%s", preview.Code, preview.Body.String())
	}
	statusBefore := authenticatedRequest(handler, http.MethodGet, "/api/v1/core/capability?core_version=1.13.19", "", "")
	if statusBefore.Code != http.StatusOK || strings.Contains(statusBefore.Body.String(), `"pin":`) {
		t.Fatalf("preview unexpectedly pinned candidate: status=%d body=%s", statusBefore.Code, statusBefore.Body.String())
	}

	requestBody = strings.Replace(requestBody, `"accept":false`, `"accept":true`, 1)
	upgrade := authenticatedRequest(handler, http.MethodPost, "/api/v1/core/capability/upgrade", requestBody, "")
	if upgrade.Code != http.StatusOK || !strings.Contains(upgrade.Body.String(), `"manifest_sha256":"`+manifestDigest+`"`) {
		t.Fatalf("upgrade status=%d body=%s", upgrade.Code, upgrade.Body.String())
	}
	statusAfter := authenticatedRequest(handler, http.MethodGet, "/api/v1/core/capability?core_version=1.13.19", "", "")
	if statusAfter.Code != http.StatusOK || !strings.Contains(statusAfter.Body.String(), `"pin":{`) ||
		!strings.Contains(statusAfter.Body.String(), `"presentation":{"semantic_facts"`) ||
		!strings.Contains(statusAfter.Body.String(), `"canonical_path":"/global/route_mode"`) ||
		!strings.Contains(statusAfter.Body.String(), `"kind":"select"`) ||
		strings.Contains(statusAfter.Body.String(), `"transforms"`) {
		t.Fatalf("accepted candidate was not pinned: status=%d body=%s", statusAfter.Code, statusAfter.Body.String())
	}
	quarantine := authenticatedRequest(handler, http.MethodPost, "/api/v1/core/capabilities/"+manifestDigest+"/quarantine", `{"reason_code":"presentation_blocked"}`, "")
	if quarantine.Code != http.StatusOK {
		t.Fatalf("quarantine status=%d body=%s", quarantine.Code, quarantine.Body.String())
	}
	statusQuarantined := authenticatedRequest(handler, http.MethodGet, "/api/v1/core/capability?core_version=1.13.19", "", "")
	if statusQuarantined.Code != http.StatusOK || strings.Contains(statusQuarantined.Body.String(), `"presentation"`) ||
		!strings.Contains(statusQuarantined.Body.String(), `"quarantined":true`) {
		t.Fatalf("quarantined presentation status=%d body=%s", statusQuarantined.Code, statusQuarantined.Body.String())
	}
}

func TestCapabilityGenerationHTTPRejectsAmbiguousReferences(t *testing.T) {
	handler, _ := newCoreHTTPFixture(t)
	commit := strings.Repeat("1", 40)
	digest := strings.Repeat("2", 64)
	tests := []struct {
		name       string
		method     string
		target     string
		body       string
		wantStatus int
		wantCode   string
	}{
		{name: "missing inspect digest", method: http.MethodGet, target: "/api/v1/core/capability/inspect?core_version=1.13.19&commit=" + commit, wantStatus: http.StatusBadRequest, wantCode: "capability_reference_invalid"},
		{name: "duplicate inspect version", method: http.MethodGet, target: "/api/v1/core/capability/inspect?core_version=1.13.19&core_version=1.13.20&commit=" + commit + "&manifest_sha256=" + digest, wantStatus: http.StatusBadRequest, wantCode: "query_invalid"},
		{name: "invalid generation", method: http.MethodPost, target: "/api/v1/core/capability/refresh", body: `{}`, wantStatus: http.StatusUnprocessableEntity, wantCode: "capability_generation_invalid"},
		{name: "refresh query", method: http.MethodPost, target: "/api/v1/core/capability/refresh?latest=true", body: `{}`, wantStatus: http.StatusBadRequest, wantCode: "query_invalid"},
		{name: "missing accept", method: http.MethodPost, target: "/api/v1/core/capability/upgrade", body: `{"exact_core_version":"1.13.19","commit_sha":"` + commit + `","manifest_sha256":"` + digest + `"}`, wantStatus: http.StatusUnprocessableEntity, wantCode: "capability_upgrade_invalid"},
		{name: "unknown upgrade member", method: http.MethodPost, target: "/api/v1/core/capability/upgrade", body: `{"exact_core_version":"1.13.19","commit_sha":"` + commit + `","manifest_sha256":"` + digest + `","accept":true,"latest":true}`, wantStatus: http.StatusUnprocessableEntity, wantCode: "invalid_json"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			response := authenticatedRequest(handler, test.method, test.target, test.body, "")
			assertCoreHTTPProblem(t, response, test.wantStatus, test.wantCode)
		})
	}
}

func TestCapabilityManifestHTTPQuarantineIsPermanentAndAuditable(t *testing.T) {
	handler, _ := newCoreHTTPFixture(t)
	generation, commit, manifestDigest := capabilityHTTPGeneration(t, "1.13.19")
	refresh := authenticatedRequest(handler, http.MethodPost, "/api/v1/core/capability/refresh", string(generation), "")
	if refresh.Code != http.StatusOK {
		t.Fatalf("refresh status=%d body=%s", refresh.Code, refresh.Body.String())
	}
	target := "/api/v1/core/capabilities/" + manifestDigest + "/quarantine"

	quarantine := authenticatedRequest(handler, http.MethodPost, target, `{"reason_code":"security_advisory"}`, "")
	if quarantine.Code != http.StatusOK {
		t.Fatalf("quarantine status=%d body=%s", quarantine.Code, quarantine.Body.String())
	}
	var first application.CapabilityQuarantineView
	if err := json.Unmarshal(quarantine.Body.Bytes(), &first); err != nil {
		t.Fatal(err)
	}
	if first.ManifestSHA256 != manifestDigest || first.ReasonCode != "security_advisory" || first.QuarantinedAt.IsZero() {
		t.Fatalf("quarantine = %+v", first)
	}

	retry := authenticatedRequest(handler, http.MethodPost, target, `{"reason_code":"security_advisory"}`, "")
	var repeated application.CapabilityQuarantineView
	if retry.Code != http.StatusOK || json.Unmarshal(retry.Body.Bytes(), &repeated) != nil || repeated != first {
		t.Fatalf("retry status=%d quarantine=%+v body=%s", retry.Code, repeated, retry.Body.String())
	}
	conflict := authenticatedRequest(handler, http.MethodPost, target, `{"reason_code":"operator_validation_failed"}`, "")
	assertCoreHTTPProblem(t, conflict, http.StatusConflict, "capability_quarantine_conflict")

	reference := "core_version=1.13.19&commit=" + commit + "&manifest_sha256=" + manifestDigest
	inspect := authenticatedRequest(handler, http.MethodGet, "/api/v1/core/capability/inspect?"+reference, "", "")
	if inspect.Code != http.StatusOK || !strings.Contains(inspect.Body.String(), `"quarantined":true`) ||
		!strings.Contains(inspect.Body.String(), `"reason_code":"security_advisory"`) {
		t.Fatalf("inspect status=%d body=%s", inspect.Code, inspect.Body.String())
	}
}

func TestCapabilityManifestHTTPQuarantineValidatesAuthCSRFAndContract(t *testing.T) {
	handler, _ := newCoreHTTPFixture(t)
	digest := strings.Repeat("b", 64)
	target := "/api/v1/core/capabilities/" + digest + "/quarantine"

	unauthenticated := httptest.NewRequest(http.MethodPost, target, strings.NewReader(`{"reason_code":"security_advisory"}`))
	unauthenticatedResponse := httptest.NewRecorder()
	handler.ServeHTTP(unauthenticatedResponse, unauthenticated)
	assertCoreHTTPProblem(t, unauthenticatedResponse, http.StatusUnauthorized, "authentication_required")

	login := httptest.NewRequest(http.MethodPost, "/api/v1/auth/session", strings.NewReader(`{"token":"correct-management-token"}`))
	loginResponse := httptest.NewRecorder()
	handler.ServeHTTP(loginResponse, login)
	if loginResponse.Code != http.StatusOK {
		t.Fatalf("login status=%d body=%s", loginResponse.Code, loginResponse.Body.String())
	}
	cookie := loginResponse.Result().Cookies()[0]
	withoutCSRF := httptest.NewRequest(http.MethodPost, target, strings.NewReader(`{"reason_code":"security_advisory"}`))
	withoutCSRF.Host = "panel.example"
	withoutCSRF.Header.Set("Origin", "http://panel.example")
	withoutCSRF.AddCookie(cookie)
	withoutCSRFResponse := httptest.NewRecorder()
	handler.ServeHTTP(withoutCSRFResponse, withoutCSRF)
	assertCoreHTTPProblem(t, withoutCSRFResponse, http.StatusForbidden, "csrf_failed")

	for _, test := range []struct {
		name       string
		method     string
		target     string
		body       string
		wantStatus int
		wantCode   string
	}{
		{name: "invalid digest", method: http.MethodPost, target: "/api/v1/core/capabilities/not-a-digest/quarantine", body: `{"reason_code":"security_advisory"}`, wantStatus: http.StatusBadRequest, wantCode: "capability_quarantine_digest_invalid"},
		{name: "invalid reason", method: http.MethodPost, target: target, body: `{"reason_code":"Security advisory"}`, wantStatus: http.StatusUnprocessableEntity, wantCode: "capability_quarantine_invalid"},
		{name: "unknown member", method: http.MethodPost, target: target, body: `{"reason_code":"security_advisory","restore":true}`, wantStatus: http.StatusUnprocessableEntity, wantCode: "invalid_json"},
		{name: "query rejected", method: http.MethodPost, target: target + "?force=true", body: `{"reason_code":"security_advisory"}`, wantStatus: http.StatusBadRequest, wantCode: "query_invalid"},
		{name: "wrong method", method: http.MethodGet, target: target, wantStatus: http.StatusMethodNotAllowed, wantCode: "method_not_allowed"},
	} {
		t.Run(test.name, func(t *testing.T) {
			response := authenticatedRequest(handler, test.method, test.target, test.body, "")
			assertCoreHTTPProblem(t, response, test.wantStatus, test.wantCode)
		})
	}
}

func capabilityHTTPGeneration(t *testing.T, exactVersion string) ([]byte, string, string) {
	t.Helper()
	version, err := coreartifact.ParseExactVersion(exactVersion)
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := capability.NewManifest(capability.ManifestSpec{
		SchemaVersion: capability.ManifestSchemaVersion,
		CoreVersion:   version,
		SupportLevel:  capability.SupportNativeStructured,
		SemanticFacts: []capability.SemanticFact{{
			ID:             "route.mode",
			CanonicalPath:  "/global/route_mode",
			Classification: capability.CoverageSupported,
			OwnedPaths:     []string{"/route_mode"},
		}},
		Transforms: []capability.Transform{{
			ID:        "route.mode.rename",
			FactID:    "route.mode",
			Primitive: capability.PrimitiveRename,
			From:      []string{"/global/route_mode"},
			To:        []string{"/route_mode"},
		}},
		UI: []capability.UIDescriptor{{
			ID:     "route.mode.select",
			FactID: "route.mode",
			Kind:   capability.UISelect,
			Label:  "Route mode",
			Order:  10,
			Options: []capability.UIOption{
				{Value: "direct", Label: "Direct"},
				{Value: "block", Label: "Block"},
			},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	manifestJSON, err := manifest.CanonicalJSON()
	if err != nil {
		t.Fatal(err)
	}
	digest, err := manifest.Digest()
	if err != nil {
		t.Fatal(err)
	}
	commit := strings.Repeat("1", 40)
	envelope := map[string]any{
		"schema_version": capability.GenerationSchemaVersion,
		"repository":     capability.ManifestRepository,
		"commit_sha":     commit,
		"manifest_count": 1,
		"manifests": []map[string]any{{
			"path":            "capabilities/" + exactVersion + ".json",
			"manifest_sha256": digest.String(),
			"manifest":        json.RawMessage(manifestJSON),
		}},
	}
	encoded, err := json.Marshal(envelope)
	if err != nil {
		t.Fatal(err)
	}
	// Guard the helper against accidentally returning a body whose transport
	// digest is the all-zero value, which would exercise quarantine behavior.
	sum := sha256.Sum256(encoded)
	if hex.EncodeToString(sum[:]) == strings.Repeat("0", 64) {
		t.Fatal("impossible zero generation digest")
	}
	return encoded, commit, digest.String()
}
