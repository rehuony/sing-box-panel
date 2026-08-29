// SPDX-License-Identifier: GPL-3.0-or-later

package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/rehuony/sing-box-panel/internal/application"
	"github.com/rehuony/sing-box-panel/internal/store"
)

func TestTaskHTTPUsesPairedKeysetCursor(t *testing.T) {
	handler, database := newCoreHTTPFixture(t)
	now := time.Date(2026, time.August, 29, 8, 0, 0, 0, time.UTC)
	for index, identifier := range []string{"task-old", "task-new"} {
		if _, err := database.EnqueueTask(context.Background(), store.EnqueueTaskInput{
			ID:        identifier,
			Lane:      store.TaskLaneMaintenance,
			Kind:      store.TaskKindCatalogRefresh,
			Payload:   json.RawMessage(`{"force":false}`),
			CreatedAt: now.Add(time.Duration(index) * time.Second),
		}); err != nil {
			t.Fatal(err)
		}
	}

	firstResponse := authenticatedRequest(handler, http.MethodGet, "/api/v1/tasks?limit=1", "", "")
	if firstResponse.Code != http.StatusOK {
		t.Fatalf("first task page status=%d body=%s", firstResponse.Code, firstResponse.Body.String())
	}
	var first application.TaskPage
	if err := json.Unmarshal(firstResponse.Body.Bytes(), &first); err != nil {
		t.Fatal(err)
	}
	if len(first.Items) != 1 || first.Items[0].ID != "task-new" || first.Next == nil {
		t.Fatalf("first task page = %+v", first)
	}

	cursorQuery := url.Values{
		"before_time": []string{first.Next.CreatedAt.Format(time.RFC3339Nano)},
		"before_id":   []string{first.Next.ID},
		"limit":       []string{"1"},
	}
	secondResponse := authenticatedRequest(handler, http.MethodGet, "/api/v1/tasks?"+cursorQuery.Encode(), "", "")
	if secondResponse.Code != http.StatusOK {
		t.Fatalf("second task page status=%d body=%s", secondResponse.Code, secondResponse.Body.String())
	}
	var second application.TaskPage
	if err := json.Unmarshal(secondResponse.Body.Bytes(), &second); err != nil {
		t.Fatal(err)
	}
	if len(second.Items) != 1 || second.Items[0].ID != "task-old" || second.Next != nil {
		t.Fatalf("second task page = %+v", second)
	}

	for _, target := range []string{
		"/api/v1/tasks?before_id=task-new",
		"/api/v1/tasks?before_time=" + url.QueryEscape(now.Format(time.RFC3339Nano)),
		"/api/v1/tasks?before_time=not-a-time&before_id=task-new",
	} {
		response := authenticatedRequest(handler, http.MethodGet, target, "", "")
		if response.Code != http.StatusBadRequest {
			t.Fatalf("invalid cursor %q status=%d body=%s", target, response.Code, response.Body.String())
		}
	}
	invalidFilter := authenticatedRequest(handler, http.MethodGet, "/api/v1/tasks?lane=background", "", "")
	if invalidFilter.Code != http.StatusBadRequest {
		t.Fatalf("invalid filter status=%d body=%s", invalidFilter.Code, invalidFilter.Body.String())
	}

	if err := database.Close(); err != nil {
		t.Fatal(err)
	}
	databaseFailure := authenticatedRequest(handler, http.MethodGet, "/api/v1/tasks", "", "")
	if databaseFailure.Code != http.StatusInternalServerError {
		t.Fatalf("database failure status=%d body=%s", databaseFailure.Code, databaseFailure.Body.String())
	}
}
