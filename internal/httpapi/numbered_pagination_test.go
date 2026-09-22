// SPDX-License-Identifier: GPL-3.0-or-later

package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/rehuony/sing-box-panel/internal/application"
	"github.com/rehuony/sing-box-panel/internal/store"
)

func TestNumberedPaginationPreservesCursorQueriesAndReportsTotals(t *testing.T) {
	handler, db := newCoreHTTPFixture(t)
	for index := range 5 {
		if _, err := handler.commands.CreateSubscriptionToken(t.Context(), application.CreateSubscriptionTokenRequest{Label: fmt.Sprintf("Key %d", index)}); err != nil {
			t.Fatal(err)
		}
		if _, err := db.AppendLogEntry(t.Context(), store.LogEntry{
			ID: fmt.Sprintf("pagination-%d", index), Time: time.Now().UTC().Add(time.Duration(index) * time.Second),
			Source: store.LogSourcePanel, Level: store.LogLevelInfo, Code: "pagination.fixture", Message: "pagination fixture", Metadata: json.RawMessage(`{}`),
		}); err != nil {
			t.Fatal(err)
		}
	}
	for _, endpoint := range []string{"/api/v1/subscription/tokens?", "/api/v1/logs/panel?search=pagination&level=info&"} {
		t.Run(endpoint, func(t *testing.T) {
			type page struct {
				Items []struct{ ID string }
				Total int
				Next  map[string]string
			}
			read := func(query string) page {
				t.Helper()
				response := authenticatedRequest(handler, http.MethodGet, endpoint+query, "", "")
				var result page
				if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &result) != nil {
					t.Fatalf("query %s: %d %s", query, response.Code, response.Body.String())
				}
				return result
			}
			all := read("limit=10")
			first := read("limit=2&offset=0")
			last := read("limit=2&offset=4")
			if first.Total != 5 || len(first.Items) != 2 || len(first.Next) == 0 || last.Total != 5 || len(last.Items) != 1 || len(last.Next) != 0 || last.Items[0].ID != all.Items[4].ID {
				t.Fatalf("numbered pages: first=%+v last=%+v", first, last)
			}
			if empty := read("limit=2&offset=100"); empty.Total != 5 || len(empty.Items) != 0 {
				t.Fatalf("out-of-range total: %+v", empty)
			}
			cursorTime := first.Next["created_at"]
			if cursorTime == "" {
				cursorTime = first.Next["time"]
			}
			query := "limit=2&before_time=" + url.QueryEscape(cursorTime) + "&before_id=" + url.QueryEscape(first.Next["id"])
			second := read(query)
			if second.Total != 5 || len(second.Items) != 2 || second.Items[0].ID != all.Items[2].ID {
				t.Fatalf("cursor query changed: %+v", second)
			}
			for _, invalid := range []string{"offset=-1", "offset=1.5", "offset=2147483648", "offset=", "offset=1&offset=2", query + "&offset=0"} {
				response := authenticatedRequest(handler, http.MethodGet, endpoint+invalid, "", "")
				if response.Code != http.StatusBadRequest {
					t.Fatalf("invalid query %q: %d %s", invalid, response.Code, response.Body.String())
				}
			}
		})
	}
	response := authenticatedRequest(handler, http.MethodGet, "/api/v1/logs/panel?search=pagination&level=error&offset=0", "", "")
	var filtered store.PanelLogPage
	if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &filtered) != nil || filtered.Total != 0 || len(filtered.Items) != 0 {
		t.Fatalf("filtered total: %d %s", response.Code, response.Body.String())
	}
}
