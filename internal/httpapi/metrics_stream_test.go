// SPDX-License-Identifier: GPL-3.0-or-later

package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMetricsStreamReportsInitialCollectionFailure(t *testing.T) {
	handler, database := newCoreHTTPFixture(t)
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/metrics/stream", nil)
	request.Header.Set("Authorization", "Bearer correct-management-token")
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusInternalServerError || !strings.HasPrefix(response.Header().Get("Content-Type"), "application/problem+json") {
		t.Fatalf("status=%d headers=%v", response.Code, response.Header())
	}
	var problem Problem
	if err := json.Unmarshal(response.Body.Bytes(), &problem); err != nil {
		t.Fatal(err)
	}
	if problem.Code != "metrics_snapshot_unavailable" {
		t.Fatalf("problem code = %q", problem.Code)
	}
}

func TestMetricsStreamClearsDeadlineAndStopsAfterFlushFailure(t *testing.T) {
	handler, _ := newCoreHTTPFixture(t)
	response := &dashboardDeadlineRecorder{
		ResponseRecorder: httptest.NewRecorder(),
		flushError:       errors.New("disconnected"),
	}
	request := httptest.NewRequest(http.MethodGet, "/api/v1/metrics/stream", nil)
	request.Header.Set("Authorization", "Bearer correct-management-token")
	handler.ServeHTTP(response, request)
	if strings.Count(response.Body.String(), "event: metrics\n") != 1 {
		t.Fatalf("body=%q", response.Body.String())
	}
	if len(response.deadlines) != 2 || response.deadlines[0].IsZero() || !response.deadlines[1].IsZero() {
		t.Fatalf("write deadline was not cleared: %v", response.deadlines)
	}
}
