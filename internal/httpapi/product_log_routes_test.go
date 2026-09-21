// SPDX-License-Identifier: GPL-3.0-or-later
package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/rehuony/sing-box-panel/internal/corelogs"
	"github.com/rehuony/sing-box-panel/internal/store"
)

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
	handler.commands.RecordOperation(context.Background(), "catalog.refresh", "Catalog refresh", nil)
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

type cancelingLogRecorder struct {
	*httptest.ResponseRecorder
	cancel context.CancelFunc
}

func (r *cancelingLogRecorder) Flush() { r.ResponseRecorder.Flush(); r.cancel() }
