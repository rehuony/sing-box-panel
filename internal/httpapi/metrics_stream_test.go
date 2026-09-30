// SPDX-License-Identifier: GPL-3.0-or-later

package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/rehuony/sing-box-panel/internal/testutil"
)

func TestMetricsStreamReportsInitialCollectionFailure(t *testing.T) {
	handler, database := newCoreHTTPFixture(t)
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/metrics/stream", nil)
	testutil.Authorize(t, handler, request)
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusServiceUnavailable || !strings.HasPrefix(response.Header().Get("Content-Type"), "application/problem+json") {
		t.Fatalf("status=%d headers=%v", response.Code, response.Header())
	}
	var problem Problem
	if err := json.Unmarshal(response.Body.Bytes(), &problem); err != nil {
		t.Fatal(err)
	}
	if problem.Code != "authentication_unavailable" {
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
	testutil.Authorize(t, handler, request)
	handler.ServeHTTP(response, request)
	if strings.Count(response.Body.String(), "event: metrics\n") != 1 {
		t.Fatalf("body=%q", response.Body.String())
	}
	if len(response.deadlines) != 2 || response.deadlines[0].IsZero() || !response.deadlines[1].IsZero() {
		t.Fatalf("write deadline was not cleared: %v", response.deadlines)
	}
}
