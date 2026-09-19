// SPDX-License-Identifier: GPL-3.0-or-later
package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/rehuony/sing-box-panel/internal/application"
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

func TestPanelLogsAndRetryUseCurrentTaskState(t *testing.T) {
	handler, db := newCoreHTTPFixture(t)
	ctx := context.Background()
	queued, err := handler.commands.QueueCatalogRefresh(ctx, application.CatalogRefreshOptions{})
	if err != nil {
		t.Fatal(err)
	}
	path := "/api/v1/tasks/" + queued.ID + "/retry"
	response := authenticatedRequest(handler, "POST", path, "", "")
	if response.Code != 409 {
		t.Fatal(response.Code, response.Body.String())
	}
	if _, err = handler.commands.CancelTask(ctx, queued.ID); err != nil {
		t.Fatal(err)
	}
	response = authenticatedRequest(handler, "POST", path, "", "")
	var retried application.Task
	if response.Code != 202 || json.Unmarshal(response.Body.Bytes(), &retried) != nil || retried.ID == queued.ID || retried.Status != "queued" {
		t.Fatal(response.Code, response.Body.String())
	}
	response = authenticatedRequest(handler, "POST", path, "", "")
	var duplicate application.Task
	if response.Code != 202 || json.Unmarshal(response.Body.Bytes(), &duplicate) != nil || duplicate.ID != retried.ID {
		t.Fatal(response.Code, response.Body.String())
	}
	_, err = db.AppendLogEntry(ctx, store.LogEntry{ID: "retry-worker", Time: time.Now(), Source: store.LogSourceTask, Level: store.LogLevelInfo, Code: "task.queued", Message: "task queued", Metadata: json.RawMessage(`{"task_id":"` + retried.ID + `"}`)})
	if err != nil {
		t.Fatal(err)
	}
	response = authenticatedRequest(handler, "GET", "/api/v1/logs/panel?limit=5", "", "")
	var page store.PanelLogPage
	if response.Code != 200 || json.Unmarshal(response.Body.Bytes(), &page) != nil {
		t.Fatal(response.Code, response.Body.String())
	}
	count := 0
	for _, item := range page.Items {
		if item.TaskID == retried.ID {
			count++
		}
		if item.ID == "log:retry-worker" {
			t.Fatal("duplicated worker event")
		}
	}
	if count != 1 {
		t.Fatal(page)
	}
	if res := authenticatedRequest(handler, http.MethodPost, path, "{}", ""); res.Code < 400 {
		t.Fatal("accepted an unexpected retry payload")
	}
}

type cancelingLogRecorder struct {
	*httptest.ResponseRecorder
	cancel context.CancelFunc
}

func (r *cancelingLogRecorder) Flush() { r.ResponseRecorder.Flush(); r.cancel() }
