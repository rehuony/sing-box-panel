// SPDX-License-Identifier: GPL-3.0-or-later

package httpapi

import (
	"bytes"
	"context"
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
	"github.com/rehuony/sing-box-panel/internal/configuration"
	"github.com/rehuony/sing-box-panel/internal/coreartifact"
	"github.com/rehuony/sing-box-panel/internal/settings"
	"github.com/rehuony/sing-box-panel/internal/store"
	"github.com/rehuony/sing-box-panel/internal/testutil"
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
		"/api/v1/core/catalog/assets?exact_version=1.13.19&architecture=amd64&variant=musl",
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

	refreshResponse := authenticatedRequest(handler, http.MethodPost, "/api/v1/core/catalog/refresh", `{"force":false}`, "")
	if refreshResponse.Code != http.StatusOK || !strings.Contains(refreshResponse.Body.String(), `"not_modified":true`) {
		t.Fatal(refreshResponse.Code, refreshResponse.Body.String())
	}
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
	var support application.ConfigurationSupport
	if err := json.Unmarshal(supportResponse.Body.Bytes(), &support); err != nil {
		t.Fatal(err)
	}
	if !support.Structured || support.ExactVersion != "1.13.19" {
		t.Fatalf("configuration support = %+v", support)
	}
	legacySchemaResponse := authenticatedRequest(
		handler,
		http.MethodGet,
		"/api/v1/core/artifacts/"+artifact.ID+"/configuration-schema",
		"",
		"",
	)
	if legacySchemaResponse.Code != http.StatusOK {
		t.Fatalf("reviewed 1.13 schema: %d %s", legacySchemaResponse.Code, legacySchemaResponse.Body.String())
	}

	listResponse := authenticatedRequest(
		handler,
		http.MethodGet,
		"/api/v1/core/artifacts?exact_version=1.13.19&architecture=amd64&variant=musl&source_kind=official&limit=1",
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

	for _, operation := range []string{"quarantine", "revoke"} {
		response := authenticatedRequest(handler, http.MethodPost, "/api/v1/core/artifacts/"+olderArtifact.ID+"/"+operation, "", "")
		if response.Code != http.StatusNotFound {
			t.Fatalf("removed %s endpoint status=%d", operation, response.Code)
		}
	}
	if strings.Contains(listResponse.Body.String(), `"verification_state"`) {
		t.Fatal("artifact response still exposes trust state")
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

func TestCoreConfigurationSchemaUsesExactVersionContractAndETag(t *testing.T) {
	handler, database := newCoreHTTPFixture(t)
	artifact, err := database.UpsertCoreArtifact(context.Background(), store.CoreArtifact{
		ID: "core_schema_http", ExactVersion: "1.14.0", OperatingSystem: "linux", Architecture: "amd64", Variant: "musl",
		SourceKind: store.CoreArtifactSourceUserVerified, UserSource: "schema HTTP fixture",
		ArchiveSHA256: strings.Repeat("a1", 32), BinarySHA256: strings.Repeat("b1", 32),
		BinaryPath: "/var/lib/sing-box-panel/artifacts/core_schema_http/sing-box", ReportedVersion: "1.14.0",
		FeatureFingerprint: json.RawMessage(`{"status":"not_reported"}`),
		CreatedAt:          time.Date(2026, time.August, 31, 12, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}
	target := "/api/v1/core/artifacts/" + artifact.ID + "/configuration-schema"

	response := authenticatedRequest(handler, http.MethodGet, target, "", "")
	if response.Code != http.StatusOK {
		t.Fatalf("configuration schema status=%d body=%s", response.Code, response.Body.String())
	}
	var contract application.ConfigurationSchema
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

	request := httptest.NewRequest(http.MethodGet, target, nil)
	request.Header.Set("Authorization", "Bearer correct-management-token")
	request.Header.Set("If-None-Match", etag)
	notModified := httptest.NewRecorder()
	handler.ServeHTTP(notModified, request)
	if notModified.Code != http.StatusNotModified || notModified.Body.Len() != 0 {
		t.Fatalf("conditional configuration schema status=%d body=%s", notModified.Code, notModified.Body.String())
	}
}

func TestConfigurationSchemaETagBindsVersionAndDigest(t *testing.T) {
	base := application.ConfigurationSchema{
		ExactVersion: "1.14.0",
		SchemaSHA256: strings.Repeat("b", 64),
	}
	baseETag := configurationSchemaETag(base)
	variants := []application.ConfigurationSchema{
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
	revision, err := testutil.SaveConfiguration(context.Background(), database, 0, store.NewCanonicalRevision{
		ID: "revision_core_http", SchemaVersion: configuration.SchemaVersion, Document: json.RawMessage(`{}`),
		CommandID: "command_core_http", CreatedAt: now,
	},
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.CreateStartupArtifact(context.Background(), store.StartupArtifact{
		ID:                  "startup_core_http",
		CanonicalRevisionID: revision.ID, ExactCoreVersion: artifact.ExactVersion,
		CoreArtifactID: artifact.ID, ConfigBytes: []byte(`{}`), CreatedAt: now.Add(time.Second),
	}); err != nil {
		t.Fatal(err)
	}

	response := authenticatedRequest(handler, http.MethodDelete, "/api/v1/core/artifacts/"+artifact.ID, "", "")
	assertCoreHTTPProblem(t, response, http.StatusConflict, "core_artifact_in_use")
}

func TestCoreHTTPAuthenticationCSRFAndCatalogState(t *testing.T) {
	handler, database := newCoreHTTPFixture(t)

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
	configuration.Auth.Token = "correct-management-token"
	if err := configuration.Validate(); err != nil {
		t.Fatal(err)
	}
	commands := application.FromStoreWithSettings(database, configuration)
	commands.SetRuntimeController(httpRuntimeFixture{commands: commands})
	return NewHandler(HandlerOptions{
		Settings: configuration,
		Commands: commands,
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
		RefreshedAt: time.Now().UTC(),
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
		Variant:            "musl",
		SourceKind:         store.CoreArtifactSourceOfficial,
		RepositoryID:       catalog.OfficialRepositoryID,
		ReleaseID:          2001,
		AssetID:            3001,
		ArchiveSHA256:      strings.Repeat("ab", 32),
		BinarySHA256:       strings.Repeat("bc", 32),
		BinaryPath:         "/var/lib/sing-box-panel/artifacts/core_http_fixture/sing-box",
		ReportedVersion:    "1.13.19",
		FeatureFingerprint: json.RawMessage(`{"features":[]}`),

		CreatedAt: time.Date(2026, time.August, 26, 12, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}
	return artifact
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
