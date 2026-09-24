// SPDX-License-Identifier: GPL-3.0-or-later
package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/rehuony/sing-box-panel/internal/application"
	"github.com/rehuony/sing-box-panel/internal/corelogs"
	"github.com/rehuony/sing-box-panel/internal/store"
)

func TestCoreLogDeletionValidatesFilesAndRequiresAuthenticationAndCSRF(t *testing.T) {
	handler, _ := newCoreHTTPFixture(t)
	logs, err := corelogs.New(handler.settings.DataDir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := logs.Writer().Write([]byte("INFO today's output\n")); err != nil {
		t.Fatal(err)
	}
	files, err := logs.List()
	if err != nil || len(files) != 1 {
		t.Fatal(files, err)
	}
	today := files[0].Name
	archive := time.Now().UTC().AddDate(0, 0, -1).Format("2006-01-02") + "-000.log"
	archivePath := filepath.Join(handler.settings.DataDir, "logs", "core", archive)
	if err := os.WriteFile(archivePath, []byte("INFO old output\n"), 0600); err != nil {
		t.Fatal(err)
	}
	path := "/api/v1/core/logs/files?file=" + archive
	unauthenticated := httptest.NewRecorder()
	handler.ServeHTTP(unauthenticated, httptest.NewRequest(http.MethodDelete, path, nil))
	if unauthenticated.Code != http.StatusUnauthorized {
		t.Fatal(unauthenticated.Code)
	}
	login := httptest.NewRecorder()
	handler.ServeHTTP(login, httptest.NewRequest(http.MethodPost, "/api/v1/auth/session", strings.NewReader(`{"token":"correct-management-token"}`)))
	var session struct {
		CSRF string `json:"csrfToken"`
	}
	if login.Code != http.StatusOK || json.Unmarshal(login.Body.Bytes(), &session) != nil {
		t.Fatal(login.Code, login.Body.String())
	}
	request := httptest.NewRequest(http.MethodDelete, path, nil)
	request.AddCookie(login.Result().Cookies()[0])
	request.Header.Set("Origin", "http://example.com")
	denied := httptest.NewRecorder()
	handler.ServeHTTP(denied, request)
	if denied.Code != http.StatusForbidden {
		t.Fatal(denied.Code, denied.Body.String())
	}
	for _, test := range []struct {
		query, body string
		status      int
	}{
		{"file=" + today, "", http.StatusConflict},
		{"file=..%2Fnative.log", "", http.StatusBadRequest},
		{"", "", http.StatusBadRequest},
		{"file=" + archive + "&file=" + today, "", http.StatusBadRequest},
		{"file=" + archive + "&unexpected=true", "", http.StatusBadRequest},
		{"file=" + archive, `{}`, http.StatusUnprocessableEntity},
	} {
		response := authenticatedRequest(handler, http.MethodDelete, "/api/v1/core/logs/files?"+test.query, test.body, "")
		if response.Code != test.status {
			t.Fatalf("query %s: %d %s", test.query, response.Code, response.Body.String())
		}
	}
	request = httptest.NewRequest(http.MethodDelete, path, nil)
	request.AddCookie(login.Result().Cookies()[0])
	request.Header.Set("Origin", "http://example.com")
	request.Header.Set("X-CSRF-Token", session.CSRF)
	deleted := httptest.NewRecorder()
	handler.ServeHTTP(deleted, request)
	if deleted.Code != http.StatusNoContent {
		t.Fatal(deleted.Code, deleted.Body.String())
	}
	if _, err := os.Stat(archivePath); !os.IsNotExist(err) {
		t.Fatalf("archive still exists: %v", err)
	}
	response := authenticatedRequest(handler, http.MethodDelete, path, "", "")
	if response.Code != http.StatusNotFound {
		t.Fatal(response.Code, response.Body.String())
	}
	response = authenticatedRequest(handler, http.MethodGet, "/api/v1/logs/panel?search=Core%20log%20file%20deletion", "", "")
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "core.log.delete.completed") {
		t.Fatal(response.Code, response.Body.String())
	}
}

func TestCoreLogFilesAreAuthenticatedAndCursorReadable(t *testing.T) {
	handler, _ := newCoreHTTPFixture(t)
	logs, err := corelogs.New(handler.settings.DataDir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = logs.Writer().Write([]byte("INFO connected\nERROR password=secret\n")); err != nil {
		t.Fatal(err)
	}
	unauthed := httptest.NewRecorder()
	handler.ServeHTTP(unauthed, httptest.NewRequest("GET", "/api/v1/core/logs/files", nil))
	if unauthed.Code != 401 {
		t.Fatal(unauthed.Code)
	}
	response := authenticatedRequest(handler, "GET", "/api/v1/core/logs/files", "", "")
	var list struct {
		Items []corelogs.File `json:"items"`
	}
	if response.Code != 200 || json.Unmarshal(response.Body.Bytes(), &list) != nil || len(list.Items) != 1 {
		t.Fatal(response.Code, response.Body.String())
	}
	response = authenticatedRequest(handler, "GET", "/api/v1/core/logs/content?file="+list.Items[0].Name, "", "")
	var chunk corelogs.Chunk
	if response.Code != 200 || json.Unmarshal(response.Body.Bytes(), &chunk) != nil || !strings.Contains(chunk.Text, "INFO connected") || strings.Contains(chunk.Text, "secret") {
		t.Fatal(response.Code, response.Body.String())
	}
	response = authenticatedRequest(handler, "GET", "/api/v1/core/logs/content?file=..%2F..%2Fpanel.db", "", "")
	if response.Code < 400 {
		t.Fatal(response.Code)
	}
	request := httptest.NewRequest("GET", "/api/v1/core/logs/stream?file="+list.Items[0].Name, nil)
	ctx, cancel := context.WithCancel(request.Context())
	defer cancel()
	request = request.WithContext(ctx)
	request.Header.Set("Authorization", "Bearer correct-management-token")
	stream := &cancelingLogRecorder{ResponseRecorder: httptest.NewRecorder(), cancel: cancel}
	handler.ServeHTTP(stream, request)
	if stream.Code != 200 || !strings.Contains(stream.Body.String(), "event: output") {
		t.Fatal(stream.Code, stream.Body.String())
	}
}

func TestPanelLogsIncludeCompletedOperations(t *testing.T) {
	handler, _ := newCoreHTTPFixture(t)
	handler.commands.RecordOperation(context.Background(), "catalog.refresh", "Catalog refresh", nil, application.OperationLogContext{})
	response := authenticatedRequest(handler, "GET", "/api/v1/logs/panel?limit=5", "", "")
	var page store.PanelLogPage
	if response.Code != 200 || json.Unmarshal(response.Body.Bytes(), &page) != nil {
		t.Fatal(response.Code, response.Body.String())
	}
	found := false
	for _, entry := range page.Items {
		if entry.Code == "catalog.refresh.completed" {
			found = true
		}
	}
	if !found {
		t.Fatal(page)
	}
	for _, path := range []string{"/api/v1/tasks", "/api/v1/tasks/old/retry", "/api/v1/tasks/old/cancel"} {
		if response := authenticatedRequest(handler, http.MethodPost, path, "", ""); response.Code != 404 {
			t.Fatalf("removed endpoint: %s %d", path, response.Code)
		}
	}
}

func TestPanelLogSearchCodesValidationAndMatching(t *testing.T) {
	handler, _ := newCoreHTTPFixture(t)
	handler.commands.RecordOperation(t.Context(), "runtime.start", "Core start", nil, application.OperationLogContext{})
	for _, query := range []string{
		"search_codes=", "search_codes=runtime.start.completed,", "search_codes=INVALID",
		"search_codes=a&search_codes=b", "search_codes=a%27OR1", "search_codes=" + strings.Repeat("a", 129),
		"search_codes=" + strings.Repeat("a,", 128) + "a",
		"search_codes=" + strings.Repeat("a", store.MaximumPanelLogSearchCodes*(store.MaximumLogCodeBytes+1)+1),
	} {
		response := authenticatedRequest(handler, http.MethodGet, "/api/v1/logs/panel?"+query, "", "")
		if response.Code != http.StatusBadRequest {
			t.Fatalf("invalid codes accepted: %d %s", response.Code, response.Body.String())
		}
	}
	response := authenticatedRequest(handler, http.MethodGet, "/api/v1/logs/panel?search=%E6%A0%B8%E5%BF%83&search_codes=runtime.start.completed&level=info", "", "")
	var page store.PanelLogPage
	if response.Code != 200 || json.Unmarshal(response.Body.Bytes(), &page) != nil || page.Total != 1 || len(page.Items) != 1 || page.Items[0].Code != "runtime.start.completed" {
		t.Fatalf("translated search: %d %s", response.Code, response.Body.String())
	}
	unauthenticated := httptest.NewRecorder()
	handler.ServeHTTP(unauthenticated, httptest.NewRequest(http.MethodGet, "/api/v1/logs/panel?search_codes=runtime.start.completed", nil))
	if unauthenticated.Code != http.StatusUnauthorized {
		t.Fatal(unauthenticated.Code)
	}
}

func TestPanelLogSearchCodesAcceptsFullDocumentedRange(t *testing.T) {
	handler, _ := newCoreHTTPFixture(t)
	codes := make([]string, store.MaximumPanelLogSearchCodes)
	for i := range codes {
		codes[i] = fmt.Sprintf("x%03d%s", i, strings.Repeat("a", store.MaximumLogCodeBytes-4))
	}
	_, err := handler.commands.RecordLog(t.Context(), application.LogRecordRequest{
		Source: store.LogSourcePanel, Level: store.LogLevelInfo, Code: codes[len(codes)-1],
		Message: "Boundary search event", Metadata: json.RawMessage(`{}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	var fullyEscaped strings.Builder
	for _, b := range []byte(strings.Join(codes, ",")) {
		fmt.Fprintf(&fullyEscaped, "%%%02X", b)
	}
	for _, encoded := range []string{url.QueryEscape(strings.Join(codes, ",")), fullyEscaped.String()} {
		query := url.Values{"search": {strings.Repeat("z", 256)}, "level": {"info"}, "limit": {"1"}, "offset": {"0"}}
		response := authenticatedRequest(handler, http.MethodGet, "/api/v1/logs/panel?"+query.Encode()+"&search_codes="+encoded, "", "")
		var page store.PanelLogPage
		if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &page) != nil || page.Total != 1 || len(page.Items) != 1 {
			t.Fatalf("documented search range rejected: %d %s", response.Code, response.Body.String())
		}
	}
	response := authenticatedRequest(handler, http.MethodGet, "/api/v1/logs/panel?search_codes="+strings.Repeat("a", 64<<10), "", "")
	assertCoreHTTPProblem(t, response, http.StatusBadRequest, "query_invalid")
}

type cancelingLogRecorder struct {
	*httptest.ResponseRecorder
	cancel context.CancelFunc
}

func (r *cancelingLogRecorder) Flush() { r.ResponseRecorder.Flush(); r.cancel() }
