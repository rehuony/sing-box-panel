// SPDX-License-Identifier: GPL-3.0-or-later

package httpapi

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/rehuony/sing-box-panel/internal/application"
	"github.com/rehuony/sing-box-panel/internal/catalog"
	"github.com/rehuony/sing-box-panel/internal/coreartifact"
	"github.com/rehuony/sing-box-panel/internal/settings"
	"github.com/rehuony/sing-box-panel/internal/store"
)

func TestCoreHTTPRoutesUseApplicationServices(t *testing.T) {
	handler, database := newCoreHTTPFixture(t)
	asset := seedCoreHTTPCatalog(t, database)
	artifact := seedCoreHTTPArtifact(t, database)
	olderArtifact := artifact
	olderArtifact.ID = "core_http_older"
	olderArtifact.AssetID = 3000
	olderArtifact.ArchiveSHA256 = strings.Repeat("da", 32)
	olderArtifact.BinarySHA256 = strings.Repeat("db", 32)
	olderArtifact.BinaryPath = "/var/lib/sing-box-panel/artifacts/core_http_older/sing-box"
	olderArtifact.CreatedAt = artifact.CreatedAt.Add(-time.Hour)
	if _, err := database.UpsertCoreArtifact(context.Background(), olderArtifact); err != nil {
		t.Fatal(err)
	}

	assetsResponse := authenticatedRequest(
		handler,
		http.MethodGet,
		"/api/v1/core/catalog/assets?exact_version=1.13.19&architecture=amd64&variant=plain&installable=true",
		"",
		"",
	)
	if assetsResponse.Code != http.StatusOK {
		t.Fatalf("list catalog assets status=%d body=%s", assetsResponse.Code, assetsResponse.Body.String())
	}
	var assets application.CatalogAssetList
	if err := json.Unmarshal(assetsResponse.Body.Bytes(), &assets); err != nil {
		t.Fatal(err)
	}
	if assets.Validator != "catalog-v1" || len(assets.Assets) != 1 || assets.Assets[0].AssetID != asset.AssetID {
		t.Fatalf("catalog assets = %+v", assets)
	}

	refreshResponse := authenticatedRequest(handler, http.MethodPost, "/api/v1/core/catalog/refresh", `{"force":true}`, "")
	assertQueuedCoreHTTPTask(t, refreshResponse, store.TaskKindCatalogRefresh)

	installResponse := authenticatedRequest(
		handler,
		http.MethodPost,
		"/api/v1/core/install",
		`{"asset_id":3001}`,
		"",
	)
	assertQueuedCoreHTTPTask(t, installResponse, store.TaskKindCoreInstall)

	importResponse := authenticatedCoreUpload(handler, []byte("archive fixture"), "")
	assertQueuedCoreHTTPTask(t, importResponse, store.TaskKindCoreImport)

	supportResponse := authenticatedRequest(
		handler,
		http.MethodGet,
		"/api/v1/core/artifacts/"+artifact.ID+"/configuration-support",
		"",
		"",
	)
	if supportResponse.Code != http.StatusOK {
		t.Fatalf("configuration support status=%d body=%s", supportResponse.Code, supportResponse.Body.String())
	}
	var support application.ConfigurationAdapterSupport
	if err := json.Unmarshal(supportResponse.Body.Bytes(), &support); err != nil {
		t.Fatal(err)
	}
	if support.Supported || support.Profile.ExactVersion != "1.13.19" {
		t.Fatalf("configuration support = %+v", support)
	}

	listResponse := authenticatedRequest(
		handler,
		http.MethodGet,
		"/api/v1/core/artifacts?exact_version=1.13.19&architecture=amd64&variant=plain&source_kind=official&verification_state=verified&limit=1",
		"",
		"",
	)
	if listResponse.Code != http.StatusOK {
		t.Fatalf("list artifacts status=%d body=%s", listResponse.Code, listResponse.Body.String())
	}
	var page application.CoreArtifactPage
	if err := json.Unmarshal(listResponse.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 || page.Items[0].ID != artifact.ID {
		t.Fatalf("artifact page = %+v", page)
	}
	if page.Next == nil || page.Next.ID != artifact.ID {
		t.Fatalf("artifact cursor = %+v", page.Next)
	}
	cursorQuery := url.Values{
		"before_time": []string{page.Next.CreatedAt.Format(time.RFC3339Nano)},
		"before_id":   []string{page.Next.ID},
		"limit":       []string{"1"},
	}
	nextResponse := authenticatedRequest(handler, http.MethodGet, "/api/v1/core/artifacts?"+cursorQuery.Encode(), "", "")
	if nextResponse.Code != http.StatusOK {
		t.Fatalf("next artifact page status=%d body=%s", nextResponse.Code, nextResponse.Body.String())
	}
	var nextPage application.CoreArtifactPage
	if err := json.Unmarshal(nextResponse.Body.Bytes(), &nextPage); err != nil {
		t.Fatal(err)
	}
	if len(nextPage.Items) != 1 || nextPage.Items[0].ID != olderArtifact.ID || nextPage.Next != nil {
		t.Fatalf("next artifact page = %+v", nextPage)
	}

	quarantineResponse := authenticatedRequest(
		handler,
		http.MethodPost,
		"/api/v1/core/artifacts/"+olderArtifact.ID+"/quarantine",
		"",
		"",
	)
	if quarantineResponse.Code != http.StatusOK || !strings.Contains(quarantineResponse.Body.String(), `"verification_state":"quarantined"`) {
		t.Fatalf("quarantine artifact status=%d body=%s", quarantineResponse.Code, quarantineResponse.Body.String())
	}
	revokeResponse := authenticatedRequest(
		handler,
		http.MethodPost,
		"/api/v1/core/artifacts/"+olderArtifact.ID+"/revoke",
		"",
		"",
	)
	if revokeResponse.Code != http.StatusOK || !strings.Contains(revokeResponse.Body.String(), `"verification_state":"revoked"`) {
		t.Fatalf("revoke artifact status=%d body=%s", revokeResponse.Code, revokeResponse.Body.String())
	}

	getResponse := authenticatedRequest(handler, http.MethodGet, "/api/v1/core/artifacts/"+artifact.ID, "", "")
	if getResponse.Code != http.StatusOK || !strings.Contains(getResponse.Body.String(), `"id":"`+artifact.ID+`"`) {
		t.Fatalf("get artifact status=%d body=%s", getResponse.Code, getResponse.Body.String())
	}

	deleteResponse := authenticatedRequest(handler, http.MethodDelete, "/api/v1/core/artifacts/"+artifact.ID, "", "")
	if deleteResponse.Code != http.StatusNoContent || deleteResponse.Body.Len() != 0 {
		t.Fatalf("delete artifact status=%d body=%s", deleteResponse.Code, deleteResponse.Body.String())
	}
	missingResponse := authenticatedRequest(handler, http.MethodGet, "/api/v1/core/artifacts/"+artifact.ID, "", "")
	assertCoreHTTPProblem(t, missingResponse, http.StatusNotFound, "core_artifact_not_found")
}

func TestCoreHTTPRejectsAmbiguousAndOversizedInputs(t *testing.T) {
	handler, database := newCoreHTTPFixture(t)
	seedCoreHTTPCatalog(t, database)
	artifact := seedCoreHTTPArtifact(t, database)

	tests := []struct {
		name       string
		method     string
		target     string
		body       string
		wantStatus int
		wantCode   string
	}{
		{
			name: "duplicate query", method: http.MethodGet,
			target:     "/api/v1/core/catalog/assets?installable=true&installable=false",
			wantStatus: http.StatusBadRequest, wantCode: "query_invalid",
		},
		{
			name: "unknown query", method: http.MethodGet,
			target:     "/api/v1/core/catalog/assets?latest=true",
			wantStatus: http.StatusBadRequest, wantCode: "query_invalid",
		},
		{
			name: "invalid boolean", method: http.MethodGet,
			target:     "/api/v1/core/catalog/assets?installable=1",
			wantStatus: http.StatusBadRequest, wantCode: "query_invalid",
		},
		{
			name: "invalid catalog dimension", method: http.MethodGet,
			target:     "/api/v1/core/catalog/assets?architecture=386",
			wantStatus: http.StatusBadRequest, wantCode: "catalog_filter_invalid",
		},
		{
			name: "zero catalog version", method: http.MethodGet,
			target:     "/api/v1/core/catalog/assets?exact_version=0.0.0",
			wantStatus: http.StatusBadRequest, wantCode: "catalog_filter_invalid",
		},
		{
			name: "query too large", method: http.MethodGet,
			target:     "/api/v1/core/catalog/assets?variant=" + strings.Repeat("a", maximumCoreQueryBytes),
			wantStatus: http.StatusBadRequest, wantCode: "query_invalid",
		},
		{
			name: "limit too large", method: http.MethodGet,
			target:     "/api/v1/core/artifacts?limit=201",
			wantStatus: http.StatusBadRequest, wantCode: "query_invalid",
		},
		{
			name: "invalid artifact filter", method: http.MethodGet,
			target:     "/api/v1/core/artifacts?source_kind=mirror",
			wantStatus: http.StatusBadRequest, wantCode: "core_artifact_filter_invalid",
		},
		{
			name: "duplicate JSON member", method: http.MethodPost,
			target: "/api/v1/core/install", body: `{"asset_id":3001,"asset_id":3002}`,
			wantStatus: http.StatusUnprocessableEntity, wantCode: "invalid_json",
		},
		{
			name: "unknown JSON member", method: http.MethodPost,
			target: "/api/v1/core/install", body: `{"asset_id":3001,"force":true}`,
			wantStatus: http.StatusUnprocessableEntity, wantCode: "invalid_json",
		},
		{
			name: "missing asset", method: http.MethodPost,
			target: "/api/v1/core/install", body: `{}`,
			wantStatus: http.StatusUnprocessableEntity, wantCode: "core_install_invalid",
		},
		{
			name: "refresh body", method: http.MethodPost,
			target: "/api/v1/core/catalog/refresh", body: `{}`,
			wantStatus: http.StatusUnprocessableEntity, wantCode: "catalog_refresh_invalid",
		},
		{
			name: "import requires multipart", method: http.MethodPost,
			target: "/api/v1/core/import", body: `{"source_path":"/private/core.tar.gz","unexpected":"secret"}`,
			wantStatus: http.StatusUnsupportedMediaType, wantCode: "core_import_media_type",
		},
		{
			name: "import plain body", method: http.MethodPost,
			target: "/api/v1/core/import", body: strings.Repeat("x", maximumCoreImportRequestBytes+1),
			wantStatus: http.StatusUnsupportedMediaType, wantCode: "core_import_media_type",
		},
		{
			name: "delete body", method: http.MethodDelete,
			target: "/api/v1/core/artifacts/" + artifact.ID, body: `{}`,
			wantStatus: http.StatusUnprocessableEntity, wantCode: "request_body_not_allowed",
		},
		{
			name: "unsupported method", method: http.MethodPut,
			target:     "/api/v1/core/catalog/assets",
			wantStatus: http.StatusMethodNotAllowed, wantCode: "method_not_allowed",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			response := authenticatedRequest(handler, test.method, test.target, test.body, "")
			assertCoreHTTPProblem(t, response, test.wantStatus, test.wantCode)
		})
	}

	secretPath := "/private/customer/upload/core.tar.gz"
	invalidImport := authenticatedCoreUpload(handler, []byte("archive fixture"), strings.Repeat("0", 64))
	assertCoreHTTPProblem(t, invalidImport, http.StatusUnprocessableEntity, "core_import_invalid")
	if strings.Contains(invalidImport.Body.String(), secretPath) {
		t.Fatalf("problem response leaked import path: %s", invalidImport.Body.String())
	}
	stillPresent := authenticatedRequest(handler, http.MethodGet, "/api/v1/core/artifacts/"+artifact.ID, "", "")
	if stillPresent.Code != http.StatusOK {
		t.Fatalf("artifact was changed by rejected DELETE body: status=%d body=%s", stillPresent.Code, stillPresent.Body.String())
	}
}

func authenticatedCoreUpload(handler http.Handler, archive []byte, overrideDigest string) *httptest.ResponseRecorder {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, _ := writer.CreateFormFile("archive", "untrusted-client-name.tar.gz")
	_, _ = part.Write(archive)
	sum := sha256.Sum256(archive)
	digest := hex.EncodeToString(sum[:])
	if overrideDigest != "" {
		digest = overrideDigest
	}
	for name, value := range map[string]string{
		"source_description": "browser upload", "sha256": digest,
		"exact_version": "1.13.19", "architecture": "amd64", "variant": "plain",
	} {
		_ = writer.WriteField(name, value)
	}
	_ = writer.Close()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/core/import", &body)
	request.Header.Set("Authorization", "Bearer correct-management-token")
	request.Header.Set("Content-Type", writer.FormDataContentType())
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func TestCoreHTTPRejectsDeletingReferencedArtifact(t *testing.T) {
	handler, database := newCoreHTTPFixture(t)
	artifact := seedCoreHTTPArtifact(t, database)
	now := time.Date(2026, time.August, 26, 13, 0, 0, 0, time.UTC)
	revision, err := database.SaveCanonicalRevisionAndTask(context.Background(), "", store.NewCanonicalRevision{
		ID: "revision_core_http", SchemaVersion: 2, Document: json.RawMessage(`{"schema_version":2,"configuration":{}}`),
		CommandID: "command_core_http", CreatedAt: now,
	}, store.NewTask{
		ID: "task_core_http", Lane: store.TaskLaneMaintenance, Kind: store.TaskKindCanonicalSaved, CreatedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.CreateStartupArtifact(context.Background(), store.StartupArtifact{
		ID:                  "startup_core_http",
		CanonicalRevisionID: revision.ID, ExactCoreVersion: artifact.ExactVersion,
		AdapterID: "test/adapter", AdapterRevision: "1", CoreArtifactID: artifact.ID,
		ConfigBytes: []byte(`{}`), CreatedAt: now.Add(time.Second),
	}); err != nil {
		t.Fatal(err)
	}

	response := authenticatedRequest(handler, http.MethodDelete, "/api/v1/core/artifacts/"+artifact.ID, "", "")
	assertCoreHTTPProblem(t, response, http.StatusConflict, "core_artifact_in_use")
}

func TestCoreHTTPAuthenticationCSRFAndCatalogState(t *testing.T) {
	handler, _ := newCoreHTTPFixture(t)

	unauthenticated := httptest.NewRequest(http.MethodGet, "/api/v1/core/catalog/assets", nil)
	unauthenticatedResponse := httptest.NewRecorder()
	handler.ServeHTTP(unauthenticatedResponse, unauthenticated)
	assertCoreHTTPProblem(t, unauthenticatedResponse, http.StatusUnauthorized, "authentication_required")

	uninitialized := authenticatedRequest(handler, http.MethodGet, "/api/v1/core/catalog/assets", "", "")
	assertCoreHTTPProblem(t, uninitialized, http.StatusConflict, "catalog_not_initialized")

	login := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/auth/session",
		strings.NewReader(`{"token":"correct-management-token"}`),
	)
	loginResponse := httptest.NewRecorder()
	handler.ServeHTTP(loginResponse, login)
	if loginResponse.Code != http.StatusOK {
		t.Fatalf("login status=%d body=%s", loginResponse.Code, loginResponse.Body.String())
	}
	var session struct {
		CSRF string `json:"csrfToken"`
	}
	if err := json.Unmarshal(loginResponse.Body.Bytes(), &session); err != nil {
		t.Fatal(err)
	}
	cookie := loginResponse.Result().Cookies()[0]

	rejected := httptest.NewRequest(http.MethodPost, "/api/v1/core/catalog/refresh", nil)
	rejected.Host = "panel.example"
	rejected.Header.Set("Origin", "http://panel.example")
	rejected.AddCookie(cookie)
	rejectedResponse := httptest.NewRecorder()
	handler.ServeHTTP(rejectedResponse, rejected)
	assertCoreHTTPProblem(t, rejectedResponse, http.StatusForbidden, "csrf_failed")

	accepted := httptest.NewRequest(http.MethodPost, "/api/v1/core/catalog/refresh", strings.NewReader(`{"force":false}`))
	accepted.Header.Set("Content-Type", "application/json")
	accepted.Host = "panel.example"
	accepted.Header.Set("Origin", "http://panel.example")
	accepted.Header.Set("X-CSRF-Token", session.CSRF)
	accepted.AddCookie(cookie)
	acceptedResponse := httptest.NewRecorder()
	handler.ServeHTTP(acceptedResponse, accepted)
	assertQueuedCoreHTTPTask(t, acceptedResponse, store.TaskKindCatalogRefresh)
}

func newCoreHTTPFixture(t *testing.T) (*Handler, *store.Store) {
	t.Helper()
	dataDirectory := t.TempDir()
	database, err := store.Open(context.Background(), filepath.Join(dataDirectory, "panel.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	configuration := settings.Defaults(filepath.Join(dataDirectory, "settings.json"))
	configuration.DataDir = dataDirectory
	configuration.Auth.Token = "correct-management-token"
	if err := configuration.Validate(); err != nil {
		t.Fatal(err)
	}
	return NewHandler(HandlerOptions{
		Settings: configuration,
		Commands: application.FromStore(database),
	}), database
}

func seedCoreHTTPCatalog(t *testing.T, database *store.Store) catalog.Asset {
	t.Helper()
	version, err := coreartifact.ParseExactVersion("1.13.19")
	if err != nil {
		t.Fatal(err)
	}
	digest, err := coreartifact.ParseSHA256(strings.Repeat("ab", 32))
	if err != nil {
		t.Fatal(err)
	}
	asset := catalog.Asset{
		RepositoryID:    catalog.OfficialRepositoryID,
		ReleaseID:       2001,
		AssetID:         3001,
		Name:            "sing-box-1.13.19-linux-amd64.tar.gz",
		DownloadURL:     "https://github.com/SagerNet/sing-box/releases/download/v1.13.19/sing-box-1.13.19-linux-amd64.tar.gz",
		Size:            1234,
		Version:         version,
		OperatingSystem: coreartifact.OperatingSystemLinux,
		Architecture:    coreartifact.ArchitectureAMD64,
		Variant:         coreartifact.VariantPlain,
		APIDigest:       digest,
		HasAPIDigest:    true,
	}
	catalogJSON, err := json.Marshal(catalog.Catalog{
		RepositoryID: catalog.OfficialRepositoryID,
		Releases: []catalog.Release{{
			ID: asset.ReleaseID, Tag: "v" + version.String(), Version: version,
			Assets: []catalog.Asset{asset},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.SaveCatalogState(context.Background(), store.CatalogState{
		Validator: "catalog-v1", Catalog: catalogJSON, Diagnostics: json.RawMessage(`[]`),
		RefreshedAt: time.Date(2026, time.August, 26, 12, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatal(err)
	}
	return asset
}

func seedCoreHTTPArtifact(t *testing.T, database *store.Store) store.CoreArtifact {
	t.Helper()
	artifact, err := database.UpsertCoreArtifact(context.Background(), store.CoreArtifact{
		ID:                 "core_http_fixture",
		ExactVersion:       "1.13.19",
		OperatingSystem:    "linux",
		Architecture:       "amd64",
		Variant:            "plain",
		SourceKind:         store.CoreArtifactSourceOfficial,
		RepositoryID:       catalog.OfficialRepositoryID,
		ReleaseID:          2001,
		AssetID:            3001,
		ArchiveSHA256:      strings.Repeat("ab", 32),
		BinarySHA256:       strings.Repeat("bc", 32),
		BinaryPath:         "/var/lib/sing-box-panel/artifacts/core_http_fixture/sing-box",
		ReportedVersion:    "1.13.19",
		FeatureFingerprint: json.RawMessage(`{"features":[]}`),
		VerificationState:  store.CoreArtifactVerified,
		CreatedAt:          time.Date(2026, time.August, 26, 12, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}
	return artifact
}

func assertQueuedCoreHTTPTask(t *testing.T, response *httptest.ResponseRecorder, kind store.TaskKind) {
	t.Helper()
	if response.Code != http.StatusAccepted {
		t.Fatalf("queue %s status=%d body=%s", kind, response.Code, response.Body.String())
	}
	var task application.Task
	if err := json.Unmarshal(response.Body.Bytes(), &task); err != nil {
		t.Fatal(err)
	}
	if task.ID == "" || task.Kind != kind || task.Status != store.TaskStatusQueued {
		t.Fatalf("queued task = %+v", task)
	}
}

func assertCoreHTTPProblem(t *testing.T, response *httptest.ResponseRecorder, status int, code string) {
	t.Helper()
	if response.Code != status {
		t.Fatalf("status=%d want=%d body=%s", response.Code, status, response.Body.String())
	}
	if contentType := response.Header().Get("Content-Type"); !strings.HasPrefix(contentType, "application/problem+json") {
		t.Fatalf("problem content type = %q", contentType)
	}
	var problem Problem
	if err := json.Unmarshal(response.Body.Bytes(), &problem); err != nil {
		t.Fatal(err)
	}
	if problem.Status != status || problem.Code != code {
		t.Fatalf("problem = %+v", problem)
	}
}
