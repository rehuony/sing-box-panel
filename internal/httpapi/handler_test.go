// SPDX-License-Identifier: GPL-3.0-or-later

package httpapi

import (
	"context"
	"encoding/json"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/rehuony/sing-box-panel/internal/application"
	"github.com/rehuony/sing-box-panel/internal/buildinfo"
	"github.com/rehuony/sing-box-panel/internal/settings"
	"github.com/rehuony/sing-box-panel/internal/store"
)

func testHandler(t *testing.T) *Handler {
	t.Helper()
	value := settings.Defaults(t.TempDir() + "/setting.json")
	value.DataDir = t.TempDir()
	value.Auth.Token = "correct-management-token"
	if err := value.Validate(); err != nil {
		t.Fatal(err)
	}
	return NewHandler(HandlerOptions{Settings: value, Build: buildinfo.Info{Version: "test"}})
}

func TestCanonicalHTTPUsesIfMatchCAS(t *testing.T) {
	ctx := context.Background()
	database, err := store.Open(ctx, filepath.Join(t.TempDir(), "panel.db"))
	if err != nil {
		t.Fatalf("store.Open() error = %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	value := settings.Defaults(t.TempDir() + "/setting.json")
	value.DataDir = t.TempDir()
	value.Auth.Token = "correct-management-token"
	handler := NewHandler(HandlerOptions{Settings: value, Commands: application.FromStore(database)})
	document := `{"schema_version":2,"configuration":{}}`

	replace := httptest.NewRequest(http.MethodPut, "/api/v1/config/canonical", strings.NewReader(document))
	replace.Header.Set("Authorization", "Bearer correct-management-token")
	replace.Header.Set("If-Match", `"none"`)
	replaceResponse := httptest.NewRecorder()
	handler.ServeHTTP(replaceResponse, replace)
	if replaceResponse.Code != http.StatusOK || replaceResponse.Header().Get("ETag") == "" {
		t.Fatalf("replace status = %d; etag=%q body=%s", replaceResponse.Code, replaceResponse.Header().Get("ETag"), replaceResponse.Body.String())
	}

	show := httptest.NewRequest(http.MethodGet, "/api/v1/config/canonical", nil)
	show.Header.Set("Authorization", "Bearer correct-management-token")
	showResponse := httptest.NewRecorder()
	handler.ServeHTTP(showResponse, show)
	if showResponse.Code != http.StatusOK || showResponse.Header().Get("ETag") != replaceResponse.Header().Get("ETag") {
		t.Fatalf("show status = %d; etag=%q body=%s", showResponse.Code, showResponse.Header().Get("ETag"), showResponse.Body.String())
	}

	stale := httptest.NewRequest(http.MethodPut, "/api/v1/config/canonical", strings.NewReader(document))
	stale.Header.Set("Authorization", "Bearer correct-management-token")
	stale.Header.Set("If-Match", `"none"`)
	staleResponse := httptest.NewRecorder()
	handler.ServeHTTP(staleResponse, stale)
	if staleResponse.Code != http.StatusPreconditionFailed {
		t.Fatalf("stale status = %d; body=%s", staleResponse.Code, staleResponse.Body.String())
	}
}

func TestCanonicalPatchHTTPPreservesLosslessValuesAndRejectsInvalidInput(t *testing.T) {
	ctx := context.Background()
	database, err := store.Open(ctx, filepath.Join(t.TempDir(), "panel.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	value := settings.Defaults(t.TempDir() + "/setting.json")
	value.DataDir = t.TempDir()
	value.Auth.Token = "correct-management-token"
	handler := NewHandler(HandlerOptions{Settings: value, Commands: application.FromStore(database)})

	initialResponse := authenticatedRequest(
		handler,
		http.MethodPut,
		"/api/v1/config/canonical",
		`{"schema_version":2,"configuration":{"experimental":{"untouched":9007199254740993}}}`,
		`"none"`,
	)
	if initialResponse.Code != http.StatusOK {
		t.Fatalf("initial status=%d body=%s", initialResponse.Code, initialResponse.Body.String())
	}
	var initial application.CanonicalSave
	if err := json.Unmarshal(initialResponse.Body.Bytes(), &initial); err != nil {
		t.Fatal(err)
	}

	patchResponse := authenticatedRequest(
		handler,
		http.MethodPatch,
		"/api/v1/config/canonical",
		`{"changes":[{"op":"set","path":"/configuration/experimental/large","value_json":"9007199254740995"},{"op":"set","path":"/configuration/experimental/payload","value_json":"{\"huge\":1e999,\"decimal\":1.0}"}]}`,
		quoteETag(initial.Revision.ID),
	)
	if patchResponse.Code != http.StatusOK {
		t.Fatalf("patch status=%d body=%s", patchResponse.Code, patchResponse.Body.String())
	}
	for _, exact := range []string{
		`"untouched":9007199254740993`,
		`"large":9007199254740995`,
		`"payload":{"decimal":1.0,"huge":1e999}`,
	} {
		if !strings.Contains(patchResponse.Body.String(), exact) {
			t.Fatalf("patch response lost %s: %s", exact, patchResponse.Body.String())
		}
	}
	if patchResponse.Header().Get("ETag") == "" || patchResponse.Header().Get("ETag") == quoteETag(initial.Revision.ID) {
		t.Fatalf("patch ETag = %q", patchResponse.Header().Get("ETag"))
	}

	missingBase := authenticatedRequest(
		handler,
		http.MethodPatch,
		"/api/v1/config/canonical",
		`{"changes":[{"op":"set","path":"/configuration/experimental/value","value_json":"true"}]}`,
		"",
	)
	if missingBase.Code != http.StatusPreconditionRequired {
		t.Fatalf("missing base status=%d body=%s", missingBase.Code, missingBase.Body.String())
	}
	duplicateValueKey := authenticatedRequest(
		handler,
		http.MethodPatch,
		"/api/v1/config/canonical",
		`{"changes":[{"op":"set","path":"/configuration/experimental/value","value_json":"{\"x\":1,\"x\":2}"}]}`,
		patchResponse.Header().Get("ETag"),
	)
	if duplicateValueKey.Code != http.StatusUnprocessableEntity {
		t.Fatalf("duplicate value key status=%d body=%s", duplicateValueKey.Code, duplicateValueKey.Body.String())
	}
	stale := authenticatedRequest(
		handler,
		http.MethodPatch,
		"/api/v1/config/canonical",
		`{"changes":[{"op":"set","path":"/configuration/experimental/value","value_json":"true"}]}`,
		quoteETag(initial.Revision.ID),
	)
	if stale.Code != http.StatusPreconditionFailed {
		t.Fatalf("stale status=%d body=%s", stale.Code, stale.Body.String())
	}
}

func TestLegacyEntityRoutesAreNotExposed(t *testing.T) {
	handler := testHandler(t)
	response := authenticatedRequest(handler, http.MethodGet, "/api/v1/nodes", "", "")
	if response.Code != http.StatusNotFound {
		t.Fatalf("legacy entity route status=%d body=%s", response.Code, response.Body.String())
	}
}

func authenticatedRequest(handler http.Handler, method, target, body, ifMatch string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, target, strings.NewReader(body))
	request.Header.Set("Authorization", "Bearer correct-management-token")
	if ifMatch != "" {
		request.Header.Set("If-Match", ifMatch)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func TestBasePathAndSPAFallback(t *testing.T) {
	value := settings.Defaults(t.TempDir() + "/setting.json")
	value.DataDir = t.TempDir()
	value.Auth.Token = "correct-management-token"
	value.Server.BasePath = "/panel"
	assets := fstest.MapFS{
		"index.html":    &fstest.MapFile{Data: []byte(`<base href="/" data-sbp-runtime /><meta name="sing-box-panel-base-path" content="__SBP_BASE_PATH__" />`)},
		"assets/app.js": &fstest.MapFile{Data: []byte(`console.log("panel")`)},
	}
	handler := NewHandler(HandlerOptions{Settings: value, Assets: fs.FS(assets)})

	root := httptest.NewRequest(http.MethodGet, "/panel", nil)
	rootResponse := httptest.NewRecorder()
	handler.ServeHTTP(rootResponse, root)
	if rootResponse.Code != http.StatusPermanentRedirect || rootResponse.Header().Get("Location") != "/panel/" {
		t.Fatalf("base redirect = %d %q", rootResponse.Code, rootResponse.Header().Get("Location"))
	}

	deepLink := httptest.NewRequest(http.MethodGet, "/panel/login", nil)
	deepLinkResponse := httptest.NewRecorder()
	handler.ServeHTTP(deepLinkResponse, deepLink)
	if deepLinkResponse.Code != http.StatusOK {
		t.Fatalf("deep link status = %d; body = %s", deepLinkResponse.Code, deepLinkResponse.Body.String())
	}
	if body := deepLinkResponse.Body.String(); !strings.Contains(body, `href="/panel/"`) || !strings.Contains(body, `content="/panel"`) {
		t.Fatalf("deep link index did not receive runtime base path: %s", body)
	}

	asset := httptest.NewRequest(http.MethodGet, "/panel/assets/app.js", nil)
	assetResponse := httptest.NewRecorder()
	handler.ServeHTTP(assetResponse, asset)
	if assetResponse.Code != http.StatusOK || !strings.Contains(assetResponse.Body.String(), "console.log") {
		t.Fatalf("asset status = %d; body = %s", assetResponse.Code, assetResponse.Body.String())
	}
}

func TestHealthIsPublic(t *testing.T) {
	handler := testHandler(t)
	request := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d; body = %s", response.Code, response.Body.String())
	}
	if response.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatal("security headers are missing")
	}
}

func TestLoginAndAuthenticatedStatus(t *testing.T) {
	handler := testHandler(t)
	login := httptest.NewRequest(http.MethodPost, "/api/v1/auth/session", strings.NewReader(`{"token":"correct-management-token"}`))
	login.Header.Set("Content-Type", "application/json")
	loginResponse := httptest.NewRecorder()
	handler.ServeHTTP(loginResponse, login)
	if loginResponse.Code != http.StatusOK {
		t.Fatalf("login status = %d; body = %s", loginResponse.Code, loginResponse.Body.String())
	}
	var payload struct {
		DisplayName string `json:"displayName"`
		CSRF        string `json:"csrfToken"`
	}
	if err := json.Unmarshal(loginResponse.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.DisplayName == "" || payload.CSRF == "" {
		t.Fatal("login returned an incomplete session")
	}
	cookies := loginResponse.Result().Cookies()
	if len(cookies) != 1 || !cookies[0].HttpOnly || cookies[0].SameSite != http.SameSiteStrictMode {
		t.Fatalf("cookies = %#v", cookies)
	}
	status := httptest.NewRequest(http.MethodGet, "/api/v1/system/status", nil)
	status.AddCookie(cookies[0])
	statusResponse := httptest.NewRecorder()
	handler.ServeHTTP(statusResponse, status)
	if statusResponse.Code != http.StatusOK {
		t.Fatalf("status request = %d; body = %s", statusResponse.Code, statusResponse.Body.String())
	}
}

func TestInvalidLoginDoesNotCreateSession(t *testing.T) {
	handler := testHandler(t)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/session", strings.NewReader(`{"token":"wrong"}`))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d; body = %s", response.Code, response.Body.String())
	}
	if len(response.Result().Cookies()) != 0 {
		t.Fatal("invalid login created a cookie")
	}
}

func TestLoginRateLimitUsesRemoteAddressAndExpires(t *testing.T) {
	handler := testHandler(t)
	now := time.Date(2026, time.August, 26, 0, 0, 0, 0, time.UTC)
	handler.logins.now = func() time.Time { return now }
	for attempt := 0; attempt < loginFailureLimit; attempt++ {
		request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/session", strings.NewReader(`{"token":"wrong"}`))
		request.RemoteAddr = "192.0.2.10:1234"
		request.Header.Set("X-Forwarded-For", "198.51.100."+strconv.Itoa(attempt+1))
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusUnauthorized {
			t.Fatalf("attempt %d status = %d; body = %s", attempt+1, response.Code, response.Body.String())
		}
	}

	blocked := httptest.NewRequest(http.MethodPost, "/api/v1/auth/session", strings.NewReader(`{"token":"correct-management-token"}`))
	blocked.RemoteAddr = "192.0.2.10:9999"
	blocked.Header.Set("X-Forwarded-For", "203.0.113.200")
	blockedResponse := httptest.NewRecorder()
	handler.ServeHTTP(blockedResponse, blocked)
	if blockedResponse.Code != http.StatusTooManyRequests || blockedResponse.Header().Get("Retry-After") != "60" {
		t.Fatalf("blocked status = %d retry-after=%q body=%s", blockedResponse.Code, blockedResponse.Header().Get("Retry-After"), blockedResponse.Body.String())
	}

	now = now.Add(loginFailureWindow)
	retry := httptest.NewRequest(http.MethodPost, "/api/v1/auth/session", strings.NewReader(`{"token":"correct-management-token"}`))
	retry.RemoteAddr = "192.0.2.10:1234"
	retryResponse := httptest.NewRecorder()
	handler.ServeHTTP(retryResponse, retry)
	if retryResponse.Code != http.StatusOK {
		t.Fatalf("retry status = %d; body = %s", retryResponse.Code, retryResponse.Body.String())
	}
}

func TestSuccessfulLoginClearsFailureBudget(t *testing.T) {
	handler := testHandler(t)
	for attempt := 0; attempt < loginFailureLimit-1; attempt++ {
		response := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/session", strings.NewReader(`{"token":"wrong"}`))
		handler.ServeHTTP(response, request)
	}
	success := httptest.NewRecorder()
	handler.ServeHTTP(success, httptest.NewRequest(http.MethodPost, "/api/v1/auth/session", strings.NewReader(`{"token":"correct-management-token"}`)))
	if success.Code != http.StatusOK {
		t.Fatalf("success status = %d; body = %s", success.Code, success.Body.String())
	}
	for attempt := 0; attempt < loginFailureLimit-1; attempt++ {
		response := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/session", strings.NewReader(`{"token":"wrong"}`))
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusUnauthorized {
			t.Fatalf("attempt after reset %d status = %d", attempt+1, response.Code)
		}
	}
}

func TestBearerStatus(t *testing.T) {
	handler := testHandler(t)
	request := httptest.NewRequest(http.MethodGet, "/api/v1/system/status", nil)
	request.Header.Set("Authorization", "Bearer correct-management-token")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d; body = %s", response.Code, response.Body.String())
	}
}

func TestLoginRejectsDuplicateToken(t *testing.T) {
	handler := testHandler(t)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/session", strings.NewReader(`{"token":"correct-management-token","token":"wrong"}`))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d; body = %s", response.Code, response.Body.String())
	}
}

func TestSessionRefreshAndCSRFProtectedLogout(t *testing.T) {
	handler := testHandler(t)
	login := httptest.NewRequest(http.MethodPost, "/api/v1/auth/session", strings.NewReader(`{"token":"correct-management-token"}`))
	loginResponse := httptest.NewRecorder()
	handler.ServeHTTP(loginResponse, login)
	if loginResponse.Code != http.StatusOK {
		t.Fatalf("login status = %d; body = %s", loginResponse.Code, loginResponse.Body.String())
	}
	var loginPayload struct {
		CSRF string `json:"csrfToken"`
	}
	if err := json.Unmarshal(loginResponse.Body.Bytes(), &loginPayload); err != nil {
		t.Fatal(err)
	}
	cookie := loginResponse.Result().Cookies()[0]

	refresh := httptest.NewRequest(http.MethodGet, "/api/v1/auth/session", nil)
	refresh.AddCookie(cookie)
	refreshResponse := httptest.NewRecorder()
	handler.ServeHTTP(refreshResponse, refresh)
	if refreshResponse.Code != http.StatusOK || !strings.Contains(refreshResponse.Body.String(), loginPayload.CSRF) {
		t.Fatalf("refresh status = %d; body = %s", refreshResponse.Code, refreshResponse.Body.String())
	}

	rejected := httptest.NewRequest(http.MethodDelete, "/api/v1/auth/session", nil)
	rejected.Host = "panel.example"
	rejected.Header.Set("Origin", "http://panel.example")
	rejected.AddCookie(cookie)
	rejectedResponse := httptest.NewRecorder()
	handler.ServeHTTP(rejectedResponse, rejected)
	if rejectedResponse.Code != http.StatusForbidden {
		t.Fatalf("logout without CSRF status = %d; body = %s", rejectedResponse.Code, rejectedResponse.Body.String())
	}

	logout := httptest.NewRequest(http.MethodDelete, "/api/v1/auth/session", nil)
	logout.Host = "panel.example"
	logout.Header.Set("Origin", "http://panel.example")
	logout.Header.Set("X-CSRF-Token", loginPayload.CSRF)
	logout.AddCookie(cookie)
	logoutResponse := httptest.NewRecorder()
	handler.ServeHTTP(logoutResponse, logout)
	if logoutResponse.Code != http.StatusNoContent {
		t.Fatalf("logout status = %d; body = %s", logoutResponse.Code, logoutResponse.Body.String())
	}
}

func TestCSRFUsesConfiguredExternalOriginWithoutForwardedHeaders(t *testing.T) {
	value := settings.Defaults(t.TempDir() + "/setting.json")
	value.DataDir = t.TempDir()
	value.Auth.Token = "correct-management-token"
	value.Server.ExternalOrigin = "http://panel.example"
	if err := value.Validate(); err != nil {
		t.Fatal(err)
	}
	handler := NewHandler(HandlerOptions{Settings: value, Build: buildinfo.Info{Version: "test"}})
	login := httptest.NewRequest(http.MethodPost, "/api/v1/auth/session", strings.NewReader(`{"token":"correct-management-token"}`))
	loginResponse := httptest.NewRecorder()
	handler.ServeHTTP(loginResponse, login)
	var payload struct {
		CSRF string `json:"csrfToken"`
	}
	if err := json.Unmarshal(loginResponse.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	cookie := loginResponse.Result().Cookies()[0]

	logout := httptest.NewRequest(http.MethodDelete, "/api/v1/auth/session", nil)
	logout.Host = "127.0.0.1:3000"
	logout.Header.Set("Origin", "http://panel.example:80")
	logout.Header.Set("X-Forwarded-Host", "attacker.example")
	logout.Header.Set("X-Forwarded-Proto", "https")
	logout.Header.Set("X-CSRF-Token", payload.CSRF)
	logout.AddCookie(cookie)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, logout)
	if response.Code != http.StatusNoContent {
		t.Fatalf("external-origin logout status=%d body=%s", response.Code, response.Body.String())
	}
}
