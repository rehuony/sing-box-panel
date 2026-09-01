// SPDX-License-Identifier: GPL-3.0-or-later

package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/rehuony/sing-box-panel/internal/application"
	"github.com/rehuony/sing-box-panel/internal/store"
)

func TestObservabilityHTTPReadsOnlyPersistedEvidence(t *testing.T) {
	handler, database := newCoreHTTPFixture(t)
	entry, err := handler.commands.RecordLog(context.Background(), application.LogRecordRequest{
		Source: store.LogSourcePanel, Level: store.LogLevelWarn, Code: "runtime.sample",
		Message: "token=must-not-leak", Metadata: json.RawMessage(`{"authorization":"Bearer secret"}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Second)
	period, err := database.UpsertTrafficPeriod(context.Background(), store.TrafficPeriod{
		ID: "period-http", PeriodStart: now.Add(-time.Hour), PeriodEnd: now.Add(time.Hour),
		InboundBytes: 12, OutboundBytes: 34, Counters: json.RawMessage(`{"inbound":{"mixed":12}}`), CreatedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	olderPeriod, err := database.UpsertTrafficPeriod(context.Background(), store.TrafficPeriod{
		ID: "period-http-older", PeriodStart: now.Add(-3 * time.Hour), PeriodEnd: now.Add(-2 * time.Hour),
		InboundBytes: 5, OutboundBytes: 8, Counters: json.RawMessage(`{}`), CreatedAt: now.Add(-3 * time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}

	logs := authenticatedRequest(handler, http.MethodGet, "/api/v1/logs?source=panel&level=warn&limit=1", "", "")
	if logs.Code != http.StatusOK || !strings.Contains(logs.Body.String(), entry.ID) {
		t.Fatalf("logs status=%d body=%s", logs.Code, logs.Body.String())
	}
	if strings.Contains(logs.Body.String(), "must-not-leak") || strings.Contains(logs.Body.String(), "Bearer secret") {
		t.Fatalf("logs leaked credentials: %s", logs.Body.String())
	}
	shownLog := authenticatedRequest(handler, http.MethodGet, "/api/v1/logs/"+entry.ID, "", "")
	if shownLog.Code != http.StatusOK || !strings.Contains(shownLog.Body.String(), `"code":"runtime.sample"`) {
		t.Fatalf("log status=%d body=%s", shownLog.Code, shownLog.Body.String())
	}
	deletedLog := authenticatedRequest(handler, http.MethodDelete, "/api/v1/logs/"+entry.ID, "", "")
	if deletedLog.Code != http.StatusOK || !strings.Contains(deletedLog.Body.String(), `"deleted":true`) {
		t.Fatalf("delete log status=%d body=%s", deletedLog.Code, deletedLog.Body.String())
	}
	if _, err := handler.commands.RecordLog(context.Background(), application.LogRecordRequest{
		Source: store.LogSourcePanel, Level: store.LogLevelInfo, Code: "retention.sample",
		Message: "safe metadata", Metadata: json.RawMessage(`{}`),
	}); err != nil {
		t.Fatal(err)
	}
	clearBefore := url.QueryEscape(time.Now().UTC().Add(time.Hour).Format(time.RFC3339Nano))
	clearedLogs := authenticatedRequest(handler, http.MethodDelete, "/api/v1/logs?source=panel&before="+clearBefore, "", "")
	if clearedLogs.Code != http.StatusOK || !strings.Contains(clearedLogs.Body.String(), `"deleted":1`) {
		t.Fatalf("clear logs status=%d body=%s", clearedLogs.Code, clearedLogs.Body.String())
	}

	metrics := authenticatedRequest(handler, http.MethodGet, "/api/v1/metrics", "", "")
	if metrics.Code != http.StatusOK {
		t.Fatalf("metrics status=%d body=%s", metrics.Code, metrics.Body.String())
	}
	var snapshot application.MetricsSnapshot
	if err := json.Unmarshal(metrics.Body.Bytes(), &snapshot); err != nil {
		t.Fatal(err)
	}
	if snapshot.Available || snapshot.ReasonCode != "not_applied" || snapshot.CurrentTrafficData != nil {
		t.Fatalf("metrics fabricated values: %+v", snapshot)
	}
	trafficStatus := authenticatedRequest(handler, http.MethodGet, "/api/v1/traffic/status", "", "")
	if trafficStatus.Code != http.StatusOK || !strings.Contains(trafficStatus.Body.String(), `"reason_code":"not_applied"`) {
		t.Fatalf("traffic status=%d body=%s", trafficStatus.Code, trafficStatus.Body.String())
	}

	from := url.QueryEscape(now.Add(-4 * time.Hour).Format(time.RFC3339Nano))
	to := url.QueryEscape(now.Add(2 * time.Hour).Format(time.RFC3339Nano))
	periods := authenticatedRequest(handler, http.MethodGet, "/api/v1/traffic/periods?from="+from+"&to="+to+"&limit=1", "", "")
	if periods.Code != http.StatusOK || !strings.Contains(periods.Body.String(), period.ID) {
		t.Fatalf("periods status=%d body=%s", periods.Code, periods.Body.String())
	}
	var firstPage store.TrafficPeriodPage
	if err := json.Unmarshal(periods.Body.Bytes(), &firstPage); err != nil || firstPage.Next == nil {
		t.Fatalf("traffic first page = %+v, err=%v", firstPage, err)
	}
	cursorTime := url.QueryEscape(firstPage.Next.PeriodStart.Format(time.RFC3339Nano))
	cursorID := url.QueryEscape(firstPage.Next.ID)
	nextPeriods := authenticatedRequest(handler, http.MethodGet, "/api/v1/traffic/periods?before_time="+cursorTime+"&before_id="+cursorID+"&limit=1", "", "")
	if nextPeriods.Code != http.StatusOK || !strings.Contains(nextPeriods.Body.String(), olderPeriod.ID) {
		t.Fatalf("next periods status=%d body=%s", nextPeriods.Code, nextPeriods.Body.String())
	}
	shownPeriod := authenticatedRequest(handler, http.MethodGet, "/api/v1/traffic/periods/"+period.ID, "", "")
	if shownPeriod.Code != http.StatusOK || !strings.Contains(shownPeriod.Body.String(), `"outbound_bytes":34`) {
		t.Fatalf("period status=%d body=%s", shownPeriod.Code, shownPeriod.Body.String())
	}
	history := authenticatedRequest(
		handler,
		http.MethodGet,
		"/api/v1/metrics/history?from="+url.QueryEscape(now.Add(-time.Hour).Format(time.RFC3339Nano))+"&to="+url.QueryEscape(now.Format(time.RFC3339Nano))+"&bucket_seconds=900",
		"",
		"",
	)
	if history.Code != http.StatusOK || !strings.Contains(history.Body.String(), `"coverage":"missing"`) ||
		!strings.Contains(history.Body.String(), `"bucket_seconds":900`) {
		t.Fatalf("metrics history status=%d body=%s", history.Code, history.Body.String())
	}
}

func TestObservabilityHTTPIsAuthenticatedAndStrict(t *testing.T) {
	handler, _ := newCoreHTTPFixture(t)
	unauthenticated := httptest.NewRecorder()
	handler.ServeHTTP(unauthenticated, httptest.NewRequest(http.MethodGet, "/api/v1/metrics", nil))
	assertCoreHTTPProblem(t, unauthenticated, http.StatusUnauthorized, "authentication_required")

	tests := []struct {
		target string
		code   string
	}{
		{target: "/api/v1/logs?since=not-a-time", code: "time_invalid"},
		{target: "/api/v1/logs?after_time=2026-08-26T00%3A00%3A00Z", code: "log_cursor_invalid"},
		{target: "/api/v1/logs?source=other", code: "log_filter_invalid"},
		{target: "/api/v1/traffic/periods?from=2026-08-27T00%3A00%3A00Z&to=2026-08-26T00%3A00%3A00Z", code: "traffic_range_invalid"},
		{target: "/api/v1/traffic/periods?before_time=2026-08-27T00%3A00%3A00Z", code: "traffic_cursor_invalid"},
		{target: "/api/v1/metrics?latest=true", code: "query_invalid"},
		{target: "/api/v1/metrics/history?from=2026-08-26T00%3A00%3A00Z&to=2026-08-27T00%3A00%3A00Z", code: "metrics_history_filter_invalid"},
		{target: "/api/v1/metrics/history?from=2026-08-26T00%3A00%3A00Z&to=2026-11-27T00%3A00%3A00Z&bucket_seconds=3600", code: "metrics_history_filter_invalid"},
	}
	for _, test := range tests {
		response := authenticatedRequest(handler, http.MethodGet, test.target, "", "")
		assertCoreHTTPProblem(t, response, http.StatusBadRequest, test.code)
	}
	method := authenticatedRequest(handler, http.MethodPost, "/api/v1/metrics", "", "")
	assertCoreHTTPProblem(t, method, http.StatusMethodNotAllowed, "method_not_allowed")
	missing := authenticatedRequest(handler, http.MethodGet, "/api/v1/traffic/periods/missing", "", "")
	assertCoreHTTPProblem(t, missing, http.StatusNotFound, "traffic_period_not_found")
}

func TestDurableLogSSEStreamsSanitizedEntriesAndReconnectCursor(t *testing.T) {
	handler, _ := newCoreHTTPFixture(t)
	entry, err := handler.commands.RecordLog(context.Background(), application.LogRecordRequest{
		Source: store.LogSourceSecurity, Level: store.LogLevelWarn, Code: "auth.sample",
		Message: "token=must-not-stream", Metadata: json.RawMessage(`{"password":"secret","request_id":"request_1"}`),
	})
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	request := httptest.NewRequest(
		http.MethodGet,
		"/api/v1/logs/stream?since="+url.QueryEscape(entry.Time.Add(-time.Second).Format(time.RFC3339Nano))+"&limit=10",
		nil,
	).WithContext(ctx)
	request.Header.Set("Authorization", "Bearer correct-management-token")
	response := newFlushRecorder()
	done := make(chan struct{})
	go func() {
		defer close(done)
		handler.ServeHTTP(response, request)
	}()
	select {
	case <-response.flushed:
		cancel()
	case <-time.After(2 * time.Second):
		cancel()
		t.Fatal("log SSE did not flush its initial event")
	}
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("log SSE did not stop after request cancellation")
	}

	body := response.Body.String()
	if response.Code != http.StatusOK || response.Header().Get("Content-Type") != "text/event-stream; charset=utf-8" ||
		!strings.Contains(body, "id: "+logEventID(entry)) || !strings.Contains(body, "event: log") ||
		!strings.Contains(body, `"request_id":"request_1"`) {
		t.Fatalf("SSE status=%d headers=%v body=%s", response.Code, response.Header(), body)
	}
	if strings.Contains(body, "must-not-stream") || strings.Contains(body, `"password":"secret"`) {
		t.Fatalf("SSE leaked sanitized values: %s", body)
	}
	cursor, err := parseLogEventID(logEventID(entry))
	if err != nil || cursor.ID != entry.ID || !cursor.Time.Equal(entry.Time) {
		t.Fatalf("reconnect cursor = %+v, error = %v", cursor, err)
	}
}

type flushRecorder struct {
	*httptest.ResponseRecorder
	flushed chan struct{}
	once    sync.Once
}

func newFlushRecorder() *flushRecorder {
	return &flushRecorder{ResponseRecorder: httptest.NewRecorder(), flushed: make(chan struct{})}
}

func (recorder *flushRecorder) Flush() {
	recorder.ResponseRecorder.Flush()
	recorder.once.Do(func() { close(recorder.flushed) })
}
