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
	"testing/fstest"
	"time"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/getkin/kin-openapi/routers/legacy"

	"github.com/rehuony/sing-box-panel/internal/buildinfo"
	"github.com/rehuony/sing-box-panel/internal/sing-box-node/agent"
	"github.com/rehuony/sing-box-panel/internal/sing-box-node/host"
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
	cpu, load, total, used := 12.5, 0.4, uint64(8<<30), uint64(3<<30)
	started := time.Date(2026, time.November, 9, 8, 0, 0, 0, time.UTC)
	readings := func(context.Context) agent.Readings {
		return agent.Readings{
			Runtime: &agent.RuntimeReading{State: "running", EnabledVersion: "1.14.3", PID: 42, RunningVersion: "1.14.3", StartedAt: &started},
			Host: &host.Snapshot{
				SampledAt: started, CPUPercent: &cpu, CPUCount: 4, LoadOne: &load,
				MemoryTotal: &total, MemoryUsed: &used, DiskTotal: &total, DiskUsed: &used,
			},
			Traffic:  &agent.TrafficReading{SampledAt: started, UploadBytes: 1024, DownloadBytes: 4096, Connections: 3, MemoryBytes: 64 << 20},
			PublicIP: "203.0.113.7",
		}
	}
	handler := NewHandler(Options{DataDir: dataDir, Link: link, Readings: readings, Build: buildinfo.Info{Version: "test"}})
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
	// The node's own readings ride along, read-only.
	if status.Node.PublicIP != "203.0.113.7" || status.Runtime == nil || status.Runtime.State != "running" || status.Runtime.PID != 42 ||
		status.Host == nil || *status.Host.CPUPercent != cpu || status.Traffic == nil || status.Traffic.DownloadBytes != 4096 {
		t.Fatalf("readings = %s", response.Body.String())
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

func TestConsoleServesItsApplicationOnlyToLoopbackHosts(t *testing.T) {
	handler := NewHandler(Options{DataDir: t.TempDir(), Assets: fstest.MapFS{
		"index.html":               {Data: []byte("<!doctype html><title>console</title>")},
		"assets/index-Bbjcgd42.js": {Data: []byte("console.log(1)")},
		"favicon.svg":              {Data: []byte("<svg/>")},
	}})
	get := func(path, host string) *httptest.ResponseRecorder {
		request := httptest.NewRequest(http.MethodGet, path, nil)
		request.Host = host
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		return response
	}
	// Any application route is the application's index, which is never cached.
	if response := get("/status", "127.0.0.1:3001"); response.Code != http.StatusOK ||
		!strings.Contains(response.Body.String(), "<title>console</title>") || response.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("route = %d %q %q", response.Code, response.Header().Get("Cache-Control"), response.Body.String())
	}
	if response := get("/assets/index-Bbjcgd42.js", "localhost:3001"); response.Code != http.StatusOK ||
		response.Header().Get("Cache-Control") != "public, max-age=31536000, immutable" {
		t.Fatalf("hashed asset = %d %q", response.Code, response.Header().Get("Cache-Control"))
	}
	if response := get("/", "attacker.example:3001"); response.Code != http.StatusMisdirectedRequest {
		t.Fatalf("rebinding host = %d", response.Code)
	}
	if response := get("/api/v1/missing", "127.0.0.1:3001"); response.Code != http.StatusNotFound ||
		!strings.Contains(response.Body.String(), "operation_not_found") {
		t.Fatalf("unknown API = %d %s", response.Code, response.Body.String())
	}
}
