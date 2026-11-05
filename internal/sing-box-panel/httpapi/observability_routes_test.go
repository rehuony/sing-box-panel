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

	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/store"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/telemetry"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/testutil"
)

func TestObservabilityHTTPReadsOnlyPersistedEvidence(t *testing.T) {
	handler, database := newCoreHTTPFixture(t)
	entry, err := handler.services.Telemetry.RecordLog(context.Background(), telemetry.LogRecordRequest{
		Source: store.LogSourcePanel, Level: store.LogLevelWarn, Code: "runtime.sample",
		Message: "token=must-not-leak", Metadata: json.RawMessage(`{"authorization":"Bearer secret"}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	nodeID := testutil.CreateNode(t, database, "edge")
	nodePath := "/api/v1/nodes/" + nodeID
	now := time.Now().UTC().Truncate(time.Second)
	thisMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	lastMonth := thisMonth.AddDate(0, -1, 0)
	record := func(at time.Time, upload, download int64) {
		t.Helper()
		start := time.Date(at.Year(), at.Month(), 1, 0, 0, 0, 0, time.UTC)
		if _, err := database.RecordTrafficSample(context.Background(), store.TrafficSampleInput{
			NodeID: nodeID, InstanceID: "instance", ActivationBundleID: "bundle", PID: 7, ProcessStartToken: "process",
			SampledAt: at, PeriodStart: start, PeriodEnd: start.AddDate(0, 1, 0), UploadTotal: upload, DownloadTotal: download,
		}); err != nil {
			t.Fatal(err)
		}
	}
	record(lastMonth.Add(time.Hour), 0, 0)
	record(lastMonth.Add(time.Hour+10*time.Second), 8, 5)
	record(thisMonth, 42, 17)
	periodID := "traffic_" + thisMonth.Format("20060102") + "_" + thisMonth.AddDate(0, 1, 0).Format("20060102")
	olderPeriodID := "traffic_" + lastMonth.Format("20060102") + "_" + thisMonth.Format("20060102")

	logs := authenticatedRequest(t, handler, http.MethodGet, "/api/v1/logs?source=panel&level=warn&limit=1", "", "")
	if logs.Code != http.StatusOK || !strings.Contains(logs.Body.String(), entry.ID) {
		t.Fatalf("logs status=%d body=%s", logs.Code, logs.Body.String())
	}
	if strings.Contains(logs.Body.String(), "must-not-leak") || strings.Contains(logs.Body.String(), "Bearer secret") {
		t.Fatalf("logs leaked credentials: %s", logs.Body.String())
	}
	shownLog := authenticatedRequest(t, handler, http.MethodGet, "/api/v1/logs/"+entry.ID, "", "")
	if shownLog.Code != http.StatusOK || !strings.Contains(shownLog.Body.String(), `"code":"runtime.sample"`) {
		t.Fatalf("log status=%d body=%s", shownLog.Code, shownLog.Body.String())
	}
	deletedLog := authenticatedRequest(t, handler, http.MethodDelete, "/api/v1/logs/"+entry.ID, "", "")
	if deletedLog.Code != http.StatusOK || !strings.Contains(deletedLog.Body.String(), `"deleted":true`) {
		t.Fatalf("delete log status=%d body=%s", deletedLog.Code, deletedLog.Body.String())
	}
	if _, err := handler.services.Telemetry.RecordLog(context.Background(), telemetry.LogRecordRequest{
		Source: store.LogSourcePanel, Level: store.LogLevelInfo, Code: "retention.sample",
		Message: "safe metadata", Metadata: json.RawMessage(`{}`),
	}); err != nil {
		t.Fatal(err)
	}
	clearBefore := url.QueryEscape(time.Now().UTC().Add(time.Hour).Format(time.RFC3339Nano))
	clearedLogs := authenticatedRequest(t, handler, http.MethodDelete, "/api/v1/logs?source=panel&before="+clearBefore, "", "")
	if clearedLogs.Code != http.StatusOK || !strings.Contains(clearedLogs.Body.String(), `"deleted":1`) {
		t.Fatalf("clear logs status=%d body=%s", clearedLogs.Code, clearedLogs.Body.String())
	}

	// A node that never reported state has no live evidence, only its
	// recorded traffic; the fleet's status sums every node.
	metrics := authenticatedRequest(t, handler, http.MethodGet, nodePath+"/metrics", "", "")
	if metrics.Code != http.StatusOK || !strings.Contains(metrics.Body.String(), `"reason_code":"not_applied"`) ||
		!strings.Contains(metrics.Body.String(), `"outbound_bytes":34`) || strings.Contains(metrics.Body.String(), "live_sample") {
		t.Fatalf("node metrics status=%d body=%s", metrics.Code, metrics.Body.String())
	}
	// A quota belongs to its node: the node's metrics report it, and saving
	// it with a stale revision fails.
	settings := authenticatedRequest(t, handler, http.MethodGet, nodePath+"/traffic/settings", "", "")
	if settings.Code != http.StatusOK || settings.Body.String() != `{"revision":1,"quota_gib":null,"period_months":1,"reset_day":1}`+"\n" {
		t.Fatalf("traffic settings status=%d body=%s", settings.Code, settings.Body.String())
	}
	saved := authenticatedRequest(t, handler, http.MethodPut, nodePath+"/traffic/settings", `{"revision":1,"quota_gib":1,"period_months":1,"reset_day":1}`, "")
	if saved.Code != http.StatusOK || !strings.Contains(saved.Body.String(), `"revision":2`) {
		t.Fatalf("save traffic settings status=%d body=%s", saved.Code, saved.Body.String())
	}
	for body, code := range map[string]int{
		`{"revision":1,"quota_gib":2,"period_months":1,"reset_day":1}`:  http.StatusPreconditionFailed,
		`{"revision":2,"quota_gib":2,"period_months":1,"reset_day":29}`: http.StatusUnprocessableEntity,
	} {
		if rejected := authenticatedRequest(t, handler, http.MethodPut, nodePath+"/traffic/settings", body, ""); rejected.Code != code {
			t.Fatalf("save %s status=%d body=%s, want %d", body, rejected.Code, rejected.Body.String(), code)
		}
	}
	metrics = authenticatedRequest(t, handler, http.MethodGet, nodePath+"/metrics", "", "")
	if metrics.Code != http.StatusOK || !strings.Contains(metrics.Body.String(), `"quota_bytes":1073741824`) || !strings.Contains(metrics.Body.String(), `"quota_exceeded":false`) {
		t.Fatalf("node metrics with a quota status=%d body=%s", metrics.Code, metrics.Body.String())
	}
	status := authenticatedRequest(t, handler, http.MethodGet, "/api/v1/traffic/status", "", "")
	if status.Code != http.StatusOK || !strings.Contains(status.Body.String(), `"upload_bytes":34`) || !strings.Contains(status.Body.String(), `"download_bytes":12`) ||
		!strings.Contains(status.Body.String(), `"quota_bytes":1073741824`) {
		t.Fatalf("traffic status=%d body=%s", status.Code, status.Body.String())
	}
	if missing := authenticatedRequest(t, handler, http.MethodGet, "/api/v1/nodes/node_missing/metrics", "", ""); missing.Code != http.StatusNotFound {
		t.Fatalf("metrics of a missing node status=%d body=%s", missing.Code, missing.Body.String())
	}

	from := url.QueryEscape(lastMonth.Add(-time.Hour).Format(time.RFC3339Nano))
	to := url.QueryEscape(now.Add(2 * time.Hour).Format(time.RFC3339Nano))
	periods := authenticatedRequest(t, handler, http.MethodGet, nodePath+"/traffic/periods?from="+from+"&to="+to+"&limit=1", "", "")
	if periods.Code != http.StatusOK || !strings.Contains(periods.Body.String(), periodID) {
		t.Fatalf("periods status=%d body=%s", periods.Code, periods.Body.String())
	}
	var firstPage store.TrafficPeriodPage
	if err := json.Unmarshal(periods.Body.Bytes(), &firstPage); err != nil || firstPage.Next == nil {
		t.Fatalf("traffic first page = %+v, err=%v", firstPage, err)
	}
	cursorTime := url.QueryEscape(firstPage.Next.PeriodStart.Format(time.RFC3339Nano))
	cursorID := url.QueryEscape(firstPage.Next.ID)
	nextPeriods := authenticatedRequest(t, handler, http.MethodGet, nodePath+"/traffic/periods?before_time="+cursorTime+"&before_id="+cursorID+"&limit=1", "", "")
	if nextPeriods.Code != http.StatusOK || !strings.Contains(nextPeriods.Body.String(), olderPeriodID) {
		t.Fatalf("next periods status=%d body=%s", nextPeriods.Code, nextPeriods.Body.String())
	}
	shownPeriod := authenticatedRequest(t, handler, http.MethodGet, nodePath+"/traffic/periods/"+periodID, "", "")
	if shownPeriod.Code != http.StatusOK || !strings.Contains(shownPeriod.Body.String(), `"outbound_bytes":34`) {
		t.Fatalf("period status=%d body=%s", shownPeriod.Code, shownPeriod.Body.String())
	}
	history := authenticatedRequest(t,
		handler,
		http.MethodGet,
		nodePath+"/metrics/history?from="+url.QueryEscape(now.Add(-time.Hour).Format(time.RFC3339Nano))+"&to="+url.QueryEscape(now.Format(time.RFC3339Nano))+"&bucket_seconds=900",
		"",
		"",
	)
	if history.Code != http.StatusOK || !strings.Contains(history.Body.String(), `"coverage":"missing"`) ||
		!strings.Contains(history.Body.String(), `"bucket_seconds":900`) {
		t.Fatalf("metrics history status=%d body=%s", history.Code, history.Body.String())
	}
}

func TestObservabilityHTTPIsAuthenticatedAndStrict(t *testing.T) {
	handler, database := newCoreHTTPFixture(t)
	nodePath := "/api/v1/nodes/" + testutil.CreateNode(t, database, "edge")
	unauthenticated := httptest.NewRecorder()
	handler.ServeHTTP(unauthenticated, httptest.NewRequest(http.MethodGet, nodePath+"/metrics", nil))
	assertCoreHTTPProblem(t, unauthenticated, http.StatusUnauthorized, "authentication_required")

	tests := []struct {
		target string
		code   string
	}{
		{target: "/api/v1/logs?since=not-a-time", code: "time_invalid"},
		{target: "/api/v1/logs?after_time=2026-08-26T00%3A00%3A00Z", code: "log_cursor_invalid"},
		{target: "/api/v1/logs?source=other", code: "log_filter_invalid"},
		{target: nodePath + "/traffic/periods?from=2026-08-27T00%3A00%3A00Z&to=2026-08-26T00%3A00%3A00Z", code: "traffic_range_invalid"},
		{target: nodePath + "/traffic/periods?before_time=2026-08-27T00%3A00%3A00Z", code: "traffic_cursor_invalid"},
		{target: nodePath + "/metrics?latest=true", code: "query_invalid"},
		{target: nodePath + "/metrics/history?from=2026-08-26T00%3A00%3A00Z&to=2026-08-27T00%3A00%3A00Z", code: "metrics_history_filter_invalid"},
		{target: nodePath + "/metrics/history?from=2026-08-26T00%3A00%3A00Z&to=2026-11-27T00%3A00%3A00Z&bucket_seconds=3600", code: "metrics_history_filter_invalid"},
	}
	for _, test := range tests {
		response := authenticatedRequest(t, handler, http.MethodGet, test.target, "", "")
		assertCoreHTTPProblem(t, response, http.StatusBadRequest, test.code)
	}
	method := authenticatedRequest(t, handler, http.MethodPost, nodePath+"/metrics", "", "")
	assertCoreHTTPProblem(t, method, http.StatusMethodNotAllowed, "method_not_allowed")
	missing := authenticatedRequest(t, handler, http.MethodGet, nodePath+"/traffic/periods/missing", "", "")
	assertCoreHTTPProblem(t, missing, http.StatusNotFound, "traffic_period_not_found")
}

func TestDurableLogSSEStreamsSanitizedEntriesAndReconnectCursor(t *testing.T) {
	handler, _ := newCoreHTTPFixture(t)
	entry, err := handler.services.Telemetry.RecordLog(context.Background(), telemetry.LogRecordRequest{
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
	testutil.Authorize(t, handler, request)
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
