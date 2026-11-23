// SPDX-License-Identifier: GPL-3.0-or-later
package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/store"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/telemetry"
)

func TestPanelLogsIncludeCompletedOperations(t *testing.T) {
	handler, _ := newCoreHTTPFixture(t)
	handler.services.Telemetry.RecordOperation(context.Background(), "catalog.refresh", "Catalog refresh", nil, telemetry.OperationLogContext{})
	response := authenticatedRequest(t, handler, "GET", "/api/v1/logs/panel?limit=5", "", "")
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
		if response := authenticatedRequest(t, handler, http.MethodPost, path, "", ""); response.Code != 404 {
			t.Fatalf("removed endpoint: %s %d", path, response.Code)
		}
	}
}

func TestPanelLogSearchCodesValidationAndMatching(t *testing.T) {
	handler, _ := newCoreHTTPFixture(t)
	handler.services.Telemetry.RecordOperation(t.Context(), "runtime.start", "Core start", nil, telemetry.OperationLogContext{})
	for _, query := range []string{
		"search_codes=", "search_codes=runtime.start.completed,", "search_codes=INVALID",
		"search_codes=a&search_codes=b", "search_codes=a%27OR1", "search_codes=" + strings.Repeat("a", 129),
		"search_codes=" + strings.Repeat("a,", 128) + "a",
		"search_codes=" + strings.Repeat("a", store.MaximumPanelLogSearchCodes*(store.MaximumLogCodeBytes+1)+1),
	} {
		response := authenticatedRequest(t, handler, http.MethodGet, "/api/v1/logs/panel?"+query, "", "")
		if response.Code != http.StatusBadRequest {
			t.Fatalf("invalid codes accepted: %d %s", response.Code, response.Body.String())
		}
	}
	response := authenticatedRequest(t, handler, http.MethodGet, "/api/v1/logs/panel?search=%E6%A0%B8%E5%BF%83&search_codes=runtime.start.completed&level=info", "", "")
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

func TestPanelLogsFilterByNode(t *testing.T) {
	handler, _ := newCoreHTTPFixture(t)
	for _, node := range []string{"node-a", "node-b"} {
		handler.services.Telemetry.RecordOperation(t.Context(), "runtime.start", "Core start", nil, telemetry.OperationLogContext{NodeID: node})
	}
	response := authenticatedRequest(t, handler, http.MethodGet, "/api/v1/logs/panel?node_id=node-a", "", "")
	var page store.PanelLogPage
	if response.Code != 200 || json.Unmarshal(response.Body.Bytes(), &page) != nil || page.Total != 1 || len(page.Items) != 1 {
		t.Fatalf("node filter: %d %s", response.Code, response.Body.String())
	}
	if response := authenticatedRequest(t, handler, http.MethodGet, "/api/v1/logs/panel?node_id="+strings.Repeat("a", 129), "", ""); response.Code != http.StatusBadRequest {
		t.Fatalf("long node ID accepted: %d", response.Code)
	}
}

func TestPanelLogSearchCodesAcceptsFullDocumentedRange(t *testing.T) {
	handler, _ := newCoreHTTPFixture(t)
	codes := make([]string, store.MaximumPanelLogSearchCodes)
	for i := range codes {
		codes[i] = fmt.Sprintf("x%03d%s", i, strings.Repeat("a", store.MaximumLogCodeBytes-4))
	}
	_, err := handler.services.Telemetry.RecordLog(t.Context(), telemetry.LogRecordRequest{
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
		response := authenticatedRequest(t, handler, http.MethodGet, "/api/v1/logs/panel?"+query.Encode()+"&search_codes="+encoded, "", "")
		var page store.PanelLogPage
		if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &page) != nil || page.Total != 1 || len(page.Items) != 1 {
			t.Fatalf("documented search range rejected: %d %s", response.Code, response.Body.String())
		}
	}
	response := authenticatedRequest(t, handler, http.MethodGet, "/api/v1/logs/panel?search_codes="+strings.Repeat("a", 64<<10), "", "")
	assertCoreHTTPProblem(t, response, http.StatusBadRequest, "query_invalid")
}

type cancelingLogRecorder struct {
	*httptest.ResponseRecorder
	cancel context.CancelFunc
}

func (r *cancelingLogRecorder) Flush() { r.ResponseRecorder.Flush(); r.cancel() }
