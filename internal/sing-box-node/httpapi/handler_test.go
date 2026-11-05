// SPDX-License-Identifier: GPL-3.0-or-later

package httpapi

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/getkin/kin-openapi/routers/legacy"

	"github.com/rehuony/sing-box-panel/internal/buildinfo"
	"github.com/rehuony/sing-box-panel/internal/sing-box-node/agent"
	"github.com/rehuony/sing-box-panel/internal/sing-box-node/settings"
	"github.com/rehuony/sing-box-panel/internal/webauth"
)

const consoleOrigin = "http://127.0.0.1:3001"

func loadContract(t *testing.T) *openapi3.T {
	t.Helper()
	_, filename, _, _ := runtime.Caller(0)
	contract, err := openapi3.NewLoader().LoadFromFile(filepath.Join(filepath.Dir(filename), "..", "..", "..", "api", "node-openapi.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if err := contract.Validate(context.Background()); err != nil {
		t.Fatal(err)
	}
	return contract
}

func TestOperationsMatchTheContract(t *testing.T) {
	contract := loadContract(t)
	documented := map[string]bool{}
	for path, item := range contract.Paths.Map() {
		for method := range item.Operations() {
			documented[method+" /api/v1"+path] = true
		}
	}
	for _, operation := range operations {
		key := operation.method + " " + operation.path
		if !documented[key] {
			t.Errorf("%s is served but not documented", key)
		}
		delete(documented, key)
	}
	for key := range documented {
		t.Errorf("%s is documented but not served", key)
	}
}

func TestConsoleAdmitsOnlyItsOperatorOnALoopbackHost(t *testing.T) {
	ctx := t.Context()
	dataDir := t.TempDir()
	setConsole := func(enabled bool, password string) {
		t.Helper()
		hash, err := webauth.HashPassword(ctx, password)
		if err != nil {
			t.Fatal(err)
		}
		if err := settings.SaveConsole(dataDir, settings.Console{Enabled: enabled, Listen: settings.DefaultConsoleListen, PasswordHash: hash}); err != nil {
			t.Fatal(err)
		}
	}
	setConsole(true, "console-password-one")
	_, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	if err := settings.Save(dataDir, settings.Enrollment{Endpoint: "panel.example:443", PanelID: "panel", NodeID: "node", Key: key}); err != nil {
		t.Fatal(err)
	}
	link, err := agent.OpenLink(dataDir, buildinfo.Info{Version: "test"})
	if err != nil {
		t.Fatal(err)
	}
	handler := NewHandler(Options{DataDir: dataDir, Link: link, Build: buildinfo.Info{Version: "test"}})
	router, err := legacy.NewRouter(loadContract(t))
	if err != nil {
		t.Fatal(err)
	}
	var cookie *http.Cookie
	csrf := ""
	serve := func(method, path, body string, edit func(*http.Request)) *httptest.ResponseRecorder {
		t.Helper()
		request := httptest.NewRequest(method, path, strings.NewReader(body))
		request.Host = "127.0.0.1:3001"
		request.Header.Set("Origin", consoleOrigin)
		if body != "" {
			request.Header.Set("Content-Type", "application/json")
		}
		if cookie != nil {
			request.AddCookie(cookie)
		}
		if csrf != "" {
			request.Header.Set("X-CSRF-Token", csrf)
		}
		if edit != nil {
			edit(request)
		}
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		route, parameters, err := router.FindRoute(request)
		if err != nil {
			t.Fatal(err)
		}
		input := &openapi3filter.ResponseValidationInput{
			RequestValidationInput: &openapi3filter.RequestValidationInput{Request: request, PathParams: parameters, Route: route},
			Status:                 response.Code, Header: response.Header(),
		}
		input.SetBodyBytes(response.Body.Bytes())
		if err := openapi3filter.ValidateResponse(ctx, input); err != nil {
			t.Fatalf("%s %s: %v\n%s", method, path, err, response.Body.String())
		}
		return response
	}
	expect := func(response *httptest.ResponseRecorder, status int, code string) {
		t.Helper()
		var problem struct{ Code string }
		_ = json.Unmarshal(response.Body.Bytes(), &problem)
		if response.Code != status || problem.Code != code {
			t.Fatalf("status = %d %q, want %d %q: %s", response.Code, problem.Code, status, code, response.Body.String())
		}
	}
	login := func(password string) *httptest.ResponseRecorder {
		return serve(http.MethodPost, "/api/v1/auth/session", `{"password":"`+password+`"}`, nil)
	}

	// A page under another name that resolves to the node cannot reach it.
	expect(serve(http.MethodGet, "/api/v1/status", "", func(request *http.Request) { request.Host = "attacker.example:3001" }), http.StatusMisdirectedRequest, "host_not_allowed")
	expect(serve(http.MethodGet, "/api/v1/status", "", nil), http.StatusUnauthorized, "authentication_required")
	expect(login("wrong-console-password"), http.StatusUnauthorized, "invalid_credentials")
	expect(serve(http.MethodPost, "/api/v1/auth/session", `{"password":"console-password-one"}`, func(request *http.Request) {
		request.Header.Set("Origin", "http://attacker.example")
	}), http.StatusForbidden, "csrf_failed")
	response := login("console-password-one")
	expect(response, http.StatusOK, "")
	cookie = response.Result().Cookies()[0]
	var session struct{ CSRFToken string }
	if err := json.Unmarshal(response.Body.Bytes(), &session); err != nil || cookie.Name != sessionCookie || !cookie.HttpOnly || session.CSRFToken == "" {
		t.Fatalf("session = %+v, cookie = %+v", session, cookie)
	}

	var status statusView
	response = serve(http.MethodGet, "/api/v1/status", "", nil)
	if err := json.Unmarshal(response.Body.Bytes(), &status); err != nil || status.Enrollment == nil || status.Enrollment.NodeID != "node" ||
		status.Enrollment.Trust != "system" || status.Connection.State != agent.ConnectionConnecting {
		t.Fatalf("status = %s", response.Body.String())
	}
	// State changes need the session's CSRF token.
	expect(serve(http.MethodDelete, "/api/v1/enrollment", "", nil), http.StatusForbidden, "csrf_failed")
	csrf = session.CSRFToken
	response = serve(http.MethodDelete, "/api/v1/enrollment", "", nil)
	if err := json.Unmarshal(response.Body.Bytes(), &status); err != nil || response.Code != http.StatusOK ||
		status.Enrollment != nil || status.Connection.State != agent.ConnectionUnenrolled {
		t.Fatalf("unenroll = %d %s", response.Code, response.Body.String())
	}
	if _, err := settings.Load(dataDir); !errors.Is(err, settings.ErrNotEnrolled) {
		t.Fatalf("the enrollment outlived unenrolling: %v", err)
	}
	expect(serve(http.MethodPost, "/api/v1/enrollment", `{"code":"not-a-code"}`, nil), http.StatusUnprocessableEntity, "enrollment_code_invalid")

	// A new password ends every session; a disabled console admits no one.
	setConsole(true, "console-password-two")
	expect(serve(http.MethodGet, "/api/v1/status", "", nil), http.StatusUnauthorized, "session_expired")
	setConsole(false, "console-password-two")
	expect(login("console-password-two"), http.StatusServiceUnavailable, "console_disabled")
}
