// SPDX-License-Identifier: GPL-3.0-or-later

package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/rehuony/sing-box-panel/internal/coreartifact"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/corecache"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/nodeconfig"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/settings"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/store"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/testutil"
)

func TestCoreConfigurationSchemaUsesExactVersionContractAndETag(t *testing.T) {
	handler, database := newCoreHTTPFixture(t)
	artifact, err := database.RecordCachedArtifact(context.Background(), store.CachedArtifact{
		ID: "core_schema_http", SourceKind: store.CachedArtifactUserVerified, UserSource: "schema HTTP fixture",
		SHA256: strings.Repeat("a1", 32), Size: 1, OS: "linux", Arch: "amd64", Variant: "musl", ExactVersion: "1.14.0",
		CreatedAt: time.Date(2026, time.August, 31, 12, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}
	target := testNodePath(t, database) + "/core/artifacts/" + artifact.ID + "/configuration-schema"

	response := authenticatedRequest(t, handler, http.MethodGet, target, "", "")
	if response.Code != http.StatusOK {
		t.Fatalf("configuration schema status=%d body=%s", response.Code, response.Body.String())
	}
	var contract nodeconfig.ConfigurationSchema
	if err := json.Unmarshal(response.Body.Bytes(), &contract); err != nil {
		t.Fatal(err)
	}
	if contract.ExactVersion != artifact.ExactVersion {
		t.Fatalf("configuration schema version = %q", contract.ExactVersion)
	}
	if contract.SchemaSHA256 == "" || len(contract.Schema) == 0 {
		t.Fatalf("configuration schema contract = %+v", contract)
	}
	etag := response.Header().Get("ETag")
	if etag != configurationSchemaETag(contract) {
		t.Fatalf("configuration schema ETag=%q want=%q", etag, configurationSchemaETag(contract))
	}
	if response.Header().Get("Cache-Control") != "private, no-cache" {
		t.Fatalf("configuration schema cache control = %q", response.Header().Get("Cache-Control"))
	}

	for _, validator := range []string{etag, "W/" + etag, `"other", W/` + etag, "*"} {
		request := httptest.NewRequest(http.MethodGet, target, nil)
		testutil.Authorize(t, handler, request)
		request.Header.Set("If-None-Match", validator)
		notModified := httptest.NewRecorder()
		handler.ServeHTTP(notModified, request)
		if notModified.Code != http.StatusNotModified || notModified.Body.Len() != 0 {
			t.Fatalf("conditional configuration schema validator=%q status=%d body=%s", validator, notModified.Code, notModified.Body.String())
		}
	}
}

func TestConfigurationSchemaETagBindsVersionAndDigest(t *testing.T) {
	base := nodeconfig.ConfigurationSchema{
		ExactVersion: "1.14.0",
		SchemaSHA256: strings.Repeat("b", 64),
	}
	baseETag := configurationSchemaETag(base)
	variants := []nodeconfig.ConfigurationSchema{
		{ExactVersion: "1.14.1", SchemaSHA256: base.SchemaSHA256},
		{ExactVersion: base.ExactVersion, SchemaSHA256: strings.Repeat("d", 64)},
	}
	for _, variant := range variants {
		if got := configurationSchemaETag(variant); got == baseETag {
			t.Fatalf("configuration schema ETag did not bind variant: %+v", variant)
		}
	}
}

func TestCoreHTTPRejectsAmbiguousAndOversizedInputs(t *testing.T) {
	handler, database := newCoreHTTPFixture(t)
	seedCoreHTTPCatalog(t, database)
	node := testNodePath(t, database)
	artifact := struct{ ID string }{ID: "core_http"}

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
			target:     node + "/core/artifacts?limit=201",
			wantStatus: http.StatusBadRequest, wantCode: "query_invalid",
		},
		{
			name: "invalid artifact filter", method: http.MethodGet,
			target:     node + "/core/artifacts?source_kind=mirror",
			wantStatus: http.StatusBadRequest, wantCode: "core_artifact_filter_invalid",
		},
		{
			name: "duplicate JSON member", method: http.MethodPost,
			target: node + "/core/install", body: `{"asset_id":3001,"asset_id":3002}`,
			wantStatus: http.StatusUnprocessableEntity, wantCode: "invalid_json",
		},
		{
			name: "unknown JSON member", method: http.MethodPost,
			target: node + "/core/install", body: `{"asset_id":3001,"force":true}`,
			wantStatus: http.StatusUnprocessableEntity, wantCode: "invalid_json",
		},
		{
			name: "missing asset", method: http.MethodPost,
			target: node + "/core/install", body: `{}`,
			wantStatus: http.StatusUnprocessableEntity, wantCode: "core_install_invalid",
		},
		{
			name: "refresh body", method: http.MethodPost,
			target: "/api/v1/core/catalog/refresh", body: `{}`,
			wantStatus: http.StatusUnprocessableEntity, wantCode: "catalog_refresh_invalid",
		},
		{
			name: "import requires multipart", method: http.MethodPost,
			target: node + "/core/import", body: `{"source_path":"/private/core.tar.gz","unexpected":"secret"}`,
			wantStatus: http.StatusUnsupportedMediaType, wantCode: "core_import_media_type",
		},
		{
			name: "import plain body", method: http.MethodPost,
			target: node + "/core/import", body: strings.Repeat("x", maximumCoreImportRequestBytes+1),
			wantStatus: http.StatusUnsupportedMediaType, wantCode: "core_import_media_type",
		},
		{
			name: "delete body", method: http.MethodDelete,
			target: node + "/core/artifacts/" + artifact.ID, body: `{}`,
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
			response := authenticatedRequest(t, handler, test.method, test.target, test.body, "")
			assertCoreHTTPProblem(t, response, test.wantStatus, test.wantCode)
		})
	}

	secretPath := "/private/customer/upload/core.tar.gz"
	invalidImport := authenticatedCoreUpload(t, handler, node+"/core/import", []byte("archive fixture"), strings.Repeat("0", 64))
	assertCoreHTTPProblem(t, invalidImport, http.StatusUnprocessableEntity, "core_import_invalid")
	if strings.Contains(invalidImport.Body.String(), secretPath) {
		t.Fatalf("problem response leaked import path: %s", invalidImport.Body.String())
	}
}

func authenticatedCoreUpload(t *testing.T, handler http.Handler, target string, archive []byte, overrideDigest string) *httptest.ResponseRecorder {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, _ := writer.CreateFormFile("archive", "untrusted-client-name.tar.gz")
	_, _ = part.Write(archive)
	// Explicitly exercise rejection of the removed upload parameter.
	if overrideDigest != "" {
		_ = writer.WriteField("sha256", overrideDigest)
	}
	for name, value := range map[string]string{
		"source_description": "browser upload",
		"exact_version":      "1.13.19", "architecture": "amd64", "variant": "musl",
	} {
		_ = writer.WriteField(name, value)
	}
	_ = writer.Close()
	request := httptest.NewRequest(http.MethodPost, target, &body)
	testutil.Authorize(t, handler, request)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func TestCoreHTTPAuthenticationCSRFAndCatalogState(t *testing.T) {
	handler, database := newCoreHTTPFixture(t)

	unauthenticated := httptest.NewRequest(http.MethodGet, "/api/v1/core/catalog/assets", nil)
	unauthenticatedResponse := httptest.NewRecorder()
	handler.ServeHTTP(unauthenticatedResponse, unauthenticated)
	assertCoreHTTPProblem(t, unauthenticatedResponse, http.StatusUnauthorized, "authentication_required")

	uninitialized := authenticatedRequest(t, handler, http.MethodGet, "/api/v1/core/catalog/assets", "", "")
	assertCoreHTTPProblem(t, uninitialized, http.StatusConflict, "catalog_not_initialized")

	login := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/auth/session",
		strings.NewReader(`{"email":"admin@example.com","password":"test-administrator-password"}`),
	)
	login.Header.Set("Origin", "http://example.com")
	login.Header.Set("Content-Type", "application/json")
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

	seedCoreHTTPCatalog(t, database)
	accepted := httptest.NewRequest(http.MethodPost, "/api/v1/core/catalog/refresh", strings.NewReader(`{"force":false}`))
	accepted.Header.Set("Content-Type", "application/json")
	accepted.Host = "panel.example"
	accepted.Header.Set("Origin", "http://panel.example")
	accepted.Header.Set("X-CSRF-Token", session.CSRF)
	accepted.AddCookie(cookie)
	acceptedResponse := httptest.NewRecorder()
	handler.ServeHTTP(acceptedResponse, accepted)
	if acceptedResponse.Code != http.StatusOK {
		t.Fatal(acceptedResponse.Code, acceptedResponse.Body.String())
	}
}

func newCoreHTTPFixture(t *testing.T) (*Handler, *store.Store) {
	t.Helper()
	dataDirectory := t.TempDir()
	database, err := store.Open(context.Background(), filepath.Join(dataDirectory, "panel.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	configuration := settings.Defaults()
	configuration.DataDir = dataDirectory
	configuration.Auth.Email = testutil.AdminEmail
	configuration.Auth.PasswordHash = testutil.PasswordHash
	if err := configuration.Validate(); err != nil {
		t.Fatal(err)
	}
	commands := newTestServices(t, database, configuration)
	return newTestHandler(t, HandlerOptions{
		Settings: configuration,
		Services: commands,
	}), database
}

func seedCoreHTTPCatalog(t *testing.T, database *store.Store) corecache.Asset {
	t.Helper()
	version, err := coreartifact.ParseExactVersion("1.13.19")
	if err != nil {
		t.Fatal(err)
	}
	digest, err := coreartifact.ParseSHA256(strings.Repeat("ab", 32))
	if err != nil {
		t.Fatal(err)
	}
	asset := corecache.Asset{
		RepositoryID:    corecache.OfficialRepositoryID,
		ReleaseID:       2001,
		AssetID:         3001,
		Name:            "sing-box-1.13.19-linux-amd64-musl.tar.gz",
		DownloadURL:     "https://github.com/SagerNet/sing-box/releases/download/v1.13.19/sing-box-1.13.19-linux-amd64-musl.tar.gz",
		Size:            1234,
		Version:         version,
		OperatingSystem: coreartifact.OperatingSystemLinux,
		Architecture:    coreartifact.ArchitectureAMD64,
		Variant:         coreartifact.VariantMusl,
		APIDigest:       digest,
		HasAPIDigest:    true,
	}
	catalogJSON, err := json.Marshal(corecache.Catalog{
		RepositoryID: corecache.OfficialRepositoryID,
		Releases: []corecache.Release{{
			ID: asset.ReleaseID, Tag: "v" + version.String(), Version: version,
			Assets: []corecache.Asset{asset},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.SaveCatalogState(context.Background(), store.CatalogState{
		Validator: "catalog-v1", Catalog: catalogJSON, Diagnostics: json.RawMessage(`[]`),
		RefreshedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}
	return asset
}

// testNodePath registers a node, which has no session, and returns the base
// path of its resources.
func testNodePath(t *testing.T, database *store.Store) string {
	t.Helper()
	return "/api/v1/nodes/" + testutil.CreateNode(t, database, "edge")
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

func TestNodeResourcesRequireAnExistingConnectedNode(t *testing.T) {
	handler, database := newCoreHTTPFixture(t)
	node := testNodePath(t, database)
	for _, target := range []string{"/core/status", "/core/artifacts", "/core/logs/files", "/filesystem/entries", "/config/artifacts"} {
		missing := authenticatedRequest(t, handler, http.MethodGet, "/api/v1/nodes/node_missing"+target, "", "")
		assertCoreHTTPProblem(t, missing, http.StatusNotFound, "node_not_found")
		// Commands are never queued for a node without a session.
		offline := authenticatedRequest(t, handler, http.MethodGet, node+target, "", "")
		assertCoreHTTPProblem(t, offline, http.StatusServiceUnavailable, "node_offline")
	}
	// The panel keeps each node's configuration, so it is editable offline.
	saved := authenticatedRequest(t, handler, http.MethodPut, node+"/config/file", `{"revision":0,"content":"{}"}`, "")
	if saved.Code != http.StatusOK {
		t.Fatalf("save offline node configuration: status=%d body=%s", saved.Code, saved.Body.String())
	}
	file := authenticatedRequest(t, handler, http.MethodGet, node+"/config/file", "", "")
	if file.Code != http.StatusOK || !strings.Contains(file.Body.String(), `"revision":1`) {
		t.Fatalf("read offline node configuration: status=%d body=%s", file.Code, file.Body.String())
	}
}
