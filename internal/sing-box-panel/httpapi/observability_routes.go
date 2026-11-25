// SPDX-License-Identifier: GPL-3.0-or-later

package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/store"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/telemetry"
)

func (handler *Handler) clearDurableLogs(w http.ResponseWriter, request *http.Request) {
	query, ok := strictCoreQuery(w, request, "source", "before")
	if !ok || !requireEmptyCoreBody(w, request) {
		return
	}
	before, ok := optionalHTTPTime(w, request, query.Get("before"), "before")
	if !ok {
		return
	}
	result, err := handler.services.Telemetry.ClearLogs(request.Context(), telemetry.LogClearRequest{
		Source: store.LogSource(query.Get("source")),
		Before: before,
	})
	if err != nil {
		writeQueryProblem(w, request, "log_clear_filter_invalid", "Log clear filter invalid", err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (handler *Handler) deleteDurableLog(w http.ResponseWriter, request *http.Request, identifier string) {
	if _, ok := strictCoreQuery(w, request); !ok || !requireEmptyCoreBody(w, request) {
		return
	}
	result, err := handler.services.Telemetry.DeleteLog(request.Context(), identifier)
	if err != nil {
		if telemetry.IsLogNotFound(err) {
			writeProblem(w, request, http.StatusNotFound, "log_not_found", "Log entry not found", "The requested sanitized log entry does not exist.")
			return
		}
		writeProblem(w, request, http.StatusBadRequest, "log_id_invalid", "Log ID invalid", "The log entry ID is invalid.")
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (handler *Handler) streamDurableLogs(w http.ResponseWriter, request *http.Request) {
	query, ok := strictCoreQuery(
		w,
		request,
		"source", "level", "code", "since", "after_time", "after_id", "limit",
	)
	if !ok {
		return
	}
	limit, ok := optionalLimit(w, request)
	if !ok {
		return
	}
	since, ok := optionalHTTPTime(w, request, query.Get("since"), "since")
	if !ok {
		return
	}
	cursor, ok := optionalLogCursor(w, request, query.Get("after_time"), query.Get("after_id"))
	if !ok {
		return
	}
	if since != nil && cursor != nil {
		writeProblem(w, request, http.StatusBadRequest, "log_stream_position_invalid", "Log stream position invalid", "Use either since or an after cursor, not both.")
		return
	}
	if cursor == nil && query.Get("after_time") == "" {
		rawLastEventID := strings.TrimSpace(request.Header.Get("Last-Event-ID"))
		if rawLastEventID != "" {
			parsed, err := parseLogEventID(rawLastEventID)
			if err != nil {
				writeProblem(w, request, http.StatusBadRequest, "log_cursor_invalid", "Log cursor invalid", "Last-Event-ID is not a valid durable log cursor.")
				return
			}
			cursor = parsed
		}
	}
	if cursor == nil && since == nil {
		connectedAt := time.Now().UTC()
		since = &connectedAt
	}

	filter := telemetry.LogTailRequest{
		Source: store.LogSource(query.Get("source")), Level: store.LogLevel(query.Get("level")),
		Code: query.Get("code"), Since: since, After: cursor, Limit: limit,
	}
	ctx, cancel := context.WithTimeout(request.Context(), logStreamLifetime)
	defer cancel()
	initial, err := handler.services.Telemetry.TailLogs(ctx, filter)
	if err != nil {
		writeQueryProblem(w, request, "log_filter_invalid", "Log filter invalid", err)
		return
	}
	deadline, _ := ctx.Deadline()
	stream, ok := newEventStream(ctx, w, deadline, maximumSnapshotFrameBytes)
	if !ok {
		writeProblem(w, request, http.StatusInternalServerError, "log_stream_unavailable", "Log stream unavailable", "The HTTP transport does not support streaming responses.")
		return
	}
	send := func(entries []store.LogEntry) error {
		for _, entry := range entries {
			encoded, err := json.Marshal(entry)
			if err != nil {
				return err
			}
			if err := stream.eventWithID(logEventID(entry), "log", encoded); err != nil {
				return err
			}
			filter.After = &store.LogCursor{Time: entry.Time, ID: entry.ID}
			filter.Since = nil
		}
		return nil
	}
	// The first event sends the headers with it; without one, the headers
	// go at once so the client knows the stream is open.
	if len(initial) == 0 && stream.open() != nil || send(initial) != nil {
		return
	}
	poll := time.NewTicker(500 * time.Millisecond)
	defer poll.Stop()
	heartbeat := time.NewTicker(15 * time.Second)
	defer heartbeat.Stop()
	rotation := rotationTimer(logStreamLifetime)
	defer rotation.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-heartbeat.C:
			if stream.heartbeat() != nil {
				return
			}
		case <-rotation.C:
			_ = stream.reconnect()
			return
		case <-poll.C:
			entries, err := handler.services.Telemetry.TailLogs(ctx, filter)
			if err != nil || send(entries) != nil {
				return
			}
		}
	}
}

// logStreamLifetime is how long one log stream runs before it asks its
// client to reconnect, which authenticates it again; Last-Event-ID resumes
// it after the last entry it delivered.
const logStreamLifetime = time.Minute

func logEventID(entry store.LogEntry) string {
	return entry.Time.UTC().Format(time.RFC3339Nano) + "|" + entry.ID
}

func parseLogEventID(value string) (*store.LogCursor, error) {
	rawTime, identifier, ok := strings.Cut(value, "|")
	if !ok || !store.ValidIdentifier(identifier) {
		return nil, fmt.Errorf("invalid log event id")
	}
	parsed, err := time.Parse(time.RFC3339Nano, rawTime)
	if err != nil {
		return nil, fmt.Errorf("invalid log event time: %w", err)
	}
	return &store.LogCursor{Time: parsed.UTC(), ID: identifier}, nil
}

func (handler *Handler) listDurableLogs(w http.ResponseWriter, request *http.Request) {
	query, ok := strictCoreQuery(w, request, "source", "level", "code", "since", "until", "after_time", "after_id", "limit")
	if !ok {
		return
	}
	limit, ok := optionalLimit(w, request)
	if !ok {
		return
	}
	since, ok := optionalHTTPTime(w, request, query.Get("since"), "since")
	if !ok {
		return
	}
	until, ok := optionalHTTPTime(w, request, query.Get("until"), "until")
	if !ok {
		return
	}
	if since != nil && until != nil && !until.After(*since) {
		writeProblem(w, request, http.StatusBadRequest, "log_range_invalid", "Log range invalid", "until must be later than since.")
		return
	}
	cursor, ok := optionalLogCursor(w, request, query.Get("after_time"), query.Get("after_id"))
	if !ok {
		return
	}
	page, err := handler.services.Telemetry.ListLogs(request.Context(), telemetry.LogListRequest{
		Source: store.LogSource(query.Get("source")), Level: store.LogLevel(query.Get("level")),
		Code: query.Get("code"), Since: since, Until: until, Cursor: cursor, Limit: limit,
	})
	if err != nil {
		writeQueryProblem(w, request, "log_filter_invalid", "Log filter invalid", err)
		return
	}
	writeJSON(w, http.StatusOK, page)
}

func (handler *Handler) getDurableLog(w http.ResponseWriter, request *http.Request, identifier string) {
	if _, ok := strictCoreQuery(w, request); !ok {
		return
	}
	entry, err := handler.services.Telemetry.Log(request.Context(), identifier)
	if err != nil {
		if telemetry.IsLogNotFound(err) {
			writeProblem(w, request, http.StatusNotFound, "log_not_found", "Log entry not found", "The requested sanitized log entry does not exist.")
			return
		}
		writeProblem(w, request, http.StatusBadRequest, "log_id_invalid", "Log ID invalid", "The log entry ID is invalid.")
		return
	}
	writeJSON(w, http.StatusOK, entry)
}

func (handler *Handler) currentMetrics(w http.ResponseWriter, request *http.Request) {
	if _, ok := strictCoreQuery(w, request); !ok {
		return
	}
	result, err := handler.services.Fleet.NodeMetrics(request.Context(), requestNodeID(request))
	if err != nil {
		writeProblem(w, request, http.StatusServiceUnavailable, "metrics_unavailable", "Metrics unavailable", "The node's metrics could not be read.")
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (handler *Handler) metricsHistory(w http.ResponseWriter, request *http.Request) {
	query, ok := strictCoreQuery(w, request, "from", "to", "bucket_seconds", "activation_bundle_id")
	if !ok {
		return
	}
	from, ok := optionalHTTPTime(w, request, query.Get("from"), "from")
	if !ok {
		return
	}
	to, ok := optionalHTTPTime(w, request, query.Get("to"), "to")
	if !ok {
		return
	}
	bucketSeconds, err := strconv.ParseInt(query.Get("bucket_seconds"), 10, 64)
	if from == nil || to == nil || err != nil || bucketSeconds < 1 {
		writeProblem(w, request, http.StatusBadRequest, "metrics_history_filter_invalid", "Metrics history filter invalid", "from, to, and a positive integer bucket_seconds are required.")
		return
	}
	bundleID := query.Get("activation_bundle_id")
	if bundleID != "" && !store.ValidIdentifier(bundleID) {
		writeProblem(w, request, http.StatusBadRequest, "metrics_history_filter_invalid", "Metrics history filter invalid", "activation_bundle_id is invalid.")
		return
	}
	result, err := handler.services.Telemetry.MetricsHistory(request.Context(), store.MetricsHistoryFilter{
		NodeID: requestNodeID(request), From: *from, To: *to, BucketSeconds: bucketSeconds, ActivationBundleID: bundleID,
	})
	if err != nil {
		writeQueryProblem(w, request, "metrics_history_filter_invalid", "Metrics history filter invalid", err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// recentMetrics returns the node's stored samples of the last two minutes,
// which a live chart opens with.
func (handler *Handler) recentMetrics(w http.ResponseWriter, request *http.Request) {
	if _, ok := strictCoreQuery(w, request); !ok {
		return
	}
	result, err := handler.services.Telemetry.RecentMetrics(request.Context(), requestNodeID(request))
	if err != nil {
		writeProblem(w, request, http.StatusServiceUnavailable, "recent_metrics_unavailable", "Recent metrics unavailable", "The node's recent samples could not be read.")
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (handler *Handler) trafficStatus(w http.ResponseWriter, request *http.Request) {
	if _, ok := strictCoreQuery(w, request); !ok {
		return
	}
	status, err := handler.services.Telemetry.TrafficStatus(request.Context(), time.Now().UTC(), nil)
	if err != nil {
		writeProblem(w, request, http.StatusServiceUnavailable, "traffic_status_unavailable", "Traffic status unavailable", "The recorded traffic could not be read.")
		return
	}
	writeJSON(w, http.StatusOK, status)
}

func (handler *Handler) listTrafficPeriods(w http.ResponseWriter, request *http.Request) {
	query, ok := strictCoreQuery(w, request, "from", "to", "before_time", "before_id", "limit")
	if !ok {
		return
	}
	limit, ok := optionalLimit(w, request)
	if !ok {
		return
	}
	from, ok := optionalHTTPTime(w, request, query.Get("from"), "from")
	if !ok {
		return
	}
	to, ok := optionalHTTPTime(w, request, query.Get("to"), "to")
	if !ok {
		return
	}
	if from != nil && to != nil && !to.After(*from) {
		writeProblem(w, request, http.StatusBadRequest, "traffic_range_invalid", "Traffic range invalid", "to must be later than from.")
		return
	}
	cursor, ok := optionalTrafficPeriodCursor(
		w, request, query.Get("before_time"), query.Get("before_id"),
	)
	if !ok {
		return
	}
	page, err := handler.services.Telemetry.ListTrafficPeriodPage(request.Context(), store.TrafficPeriodFilter{
		NodeID: requestNodeID(request), OverlapsStart: from, OverlapsEnd: to, Cursor: cursor, Limit: limit,
	})
	if err != nil {
		writeQueryProblem(w, request, "traffic_filter_invalid", "Traffic filter invalid", err)
		return
	}
	writeJSON(w, http.StatusOK, page)
}

func optionalTrafficPeriodCursor(
	w http.ResponseWriter,
	request *http.Request,
	rawTime, identifier string,
) (*store.TrafficPeriodCursor, bool) {
	if rawTime == "" && identifier == "" {
		return nil, true
	}
	if rawTime == "" || identifier == "" || !store.ValidIdentifier(identifier) {
		writeProblem(w, request, http.StatusBadRequest, "traffic_cursor_invalid", "Traffic cursor invalid", "before_time and before_id must be supplied together.")
		return nil, false
	}
	value, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(rawTime))
	if err != nil {
		writeProblem(w, request, http.StatusBadRequest, "traffic_cursor_invalid", "Traffic cursor invalid", "before_time must be an RFC3339 instant.")
		return nil, false
	}
	return &store.TrafficPeriodCursor{PeriodStart: value.UTC(), ID: identifier}, true
}

func (handler *Handler) getTrafficPeriod(w http.ResponseWriter, request *http.Request, identifier string) {
	if _, ok := strictCoreQuery(w, request); !ok {
		return
	}
	period, err := handler.services.Telemetry.TrafficPeriod(request.Context(), requestNodeID(request), identifier)
	if err != nil {
		if telemetry.IsTrafficPeriodNotFound(err) {
			writeProblem(w, request, http.StatusNotFound, "traffic_period_not_found", "Traffic period not found", "The requested traffic period does not exist.")
			return
		}
		writeProblem(w, request, http.StatusBadRequest, "traffic_period_id_invalid", "Traffic period ID invalid", "The traffic period ID is invalid.")
		return
	}
	writeJSON(w, http.StatusOK, period)
}

func optionalHTTPTime(w http.ResponseWriter, request *http.Request, raw, field string) (*time.Time, bool) {
	if raw == "" {
		return nil, true
	}
	value, err := time.Parse(time.RFC3339Nano, raw)
	if err != nil {
		writeProblem(w, request, http.StatusBadRequest, "time_invalid", "Time invalid", field+" must be an RFC3339 instant.")
		return nil, false
	}
	value = value.UTC()
	return &value, true
}

func optionalLogCursor(w http.ResponseWriter, request *http.Request, rawTime, identifier string) (*store.LogCursor, bool) {
	if rawTime == "" && identifier == "" {
		return nil, true
	}
	if rawTime == "" || identifier == "" {
		writeProblem(w, request, http.StatusBadRequest, "log_cursor_invalid", "Log cursor invalid", "after_time and after_id must be supplied together.")
		return nil, false
	}
	value, err := time.Parse(time.RFC3339Nano, rawTime)
	if err != nil || !store.ValidIdentifier(identifier) {
		writeProblem(w, request, http.StatusBadRequest, "log_cursor_invalid", "Log cursor invalid", "The log cursor is invalid.")
		return nil, false
	}
	return &store.LogCursor{Time: value.UTC(), ID: identifier}, true
}
