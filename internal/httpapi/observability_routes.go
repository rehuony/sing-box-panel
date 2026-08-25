// SPDX-License-Identifier: GPL-3.0-or-later

package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/rehuony/sing-box-panel/internal/application"
	"github.com/rehuony/sing-box-panel/internal/store"
)

func matchObservabilityRoute(path string) (resource string, identifier string, matched bool) {
	switch path {
	case "/api/v1/logs":
		return "logs", "", true
	case "/api/v1/logs/stream":
		return "log-stream", "", true
	case "/api/v1/metrics":
		return "metrics", "", true
	case "/api/v1/traffic/status":
		return "traffic-status", "", true
	case "/api/v1/traffic/periods":
		return "traffic-periods", "", true
	}
	for _, candidate := range []struct {
		prefix   string
		resource string
	}{
		{prefix: "/api/v1/logs/", resource: "logs"},
		{prefix: "/api/v1/traffic/periods/", resource: "traffic-periods"},
	} {
		if strings.HasPrefix(path, candidate.prefix) {
			identifier := strings.TrimPrefix(path, candidate.prefix)
			if identifier == "" || strings.Contains(identifier, "/") {
				return "invalid", "", true
			}
			return candidate.resource, identifier, true
		}
	}
	return "", "", false
}

func (handler *Handler) observabilityHandler(method, resource, identifier string) http.HandlerFunc {
	switch {
	case resource == "log-stream" && method == http.MethodGet:
		return handler.streamDurableLogs
	case resource == "logs" && identifier == "" && method == http.MethodGet:
		return handler.listDurableLogs
	case resource == "logs" && identifier == "" && method == http.MethodDelete:
		return handler.clearDurableLogs
	case resource == "logs" && identifier != "" && method == http.MethodGet:
		return func(w http.ResponseWriter, request *http.Request) { handler.getDurableLog(w, request, identifier) }
	case resource == "logs" && identifier != "" && method == http.MethodDelete:
		return func(w http.ResponseWriter, request *http.Request) { handler.deleteDurableLog(w, request, identifier) }
	case resource == "metrics" && method == http.MethodGet:
		return handler.currentMetrics
	case resource == "traffic-status" && method == http.MethodGet:
		return handler.trafficStatus
	case resource == "traffic-periods" && identifier == "" && method == http.MethodGet:
		return handler.listTrafficPeriods
	case resource == "traffic-periods" && identifier != "" && method == http.MethodGet:
		return func(w http.ResponseWriter, request *http.Request) { handler.getTrafficPeriod(w, request, identifier) }
	default:
		return methodNotAllowed
	}
}

func (handler *Handler) clearDurableLogs(w http.ResponseWriter, request *http.Request) {
	if !handler.requireCommands(w, request) {
		return
	}
	query, ok := strictCoreQuery(w, request, "source", "before")
	if !ok || !requireEmptyCoreBody(w, request) {
		return
	}
	before, ok := optionalHTTPTime(w, request, query.Get("before"), "before")
	if !ok {
		return
	}
	result, err := handler.commands.ClearLogs(request.Context(), application.LogClearRequest{
		Source: store.LogSource(query.Get("source")),
		Before: before,
	})
	if err != nil {
		writeProblem(w, request, http.StatusBadRequest, "log_clear_filter_invalid", "Log clear filter invalid", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (handler *Handler) deleteDurableLog(w http.ResponseWriter, request *http.Request, identifier string) {
	if !handler.requireCommands(w, request) {
		return
	}
	if _, ok := strictCoreQuery(w, request); !ok || !requireEmptyCoreBody(w, request) {
		return
	}
	result, err := handler.commands.DeleteLog(request.Context(), identifier)
	if err != nil {
		if application.IsLogNotFound(err) {
			writeProblem(w, request, http.StatusNotFound, "log_not_found", "Log entry not found", "The requested sanitized log entry does not exist.")
			return
		}
		writeProblem(w, request, http.StatusBadRequest, "log_id_invalid", "Log ID invalid", "The log entry ID is invalid.")
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (handler *Handler) streamDurableLogs(w http.ResponseWriter, request *http.Request) {
	if !handler.requireCommands(w, request) {
		return
	}
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

	filter := application.LogTailRequest{
		Source: store.LogSource(query.Get("source")), Level: store.LogLevel(query.Get("level")),
		Code: query.Get("code"), Since: since, After: cursor, Limit: limit,
	}
	initial, err := handler.commands.TailLogs(request.Context(), filter)
	if err != nil {
		writeProblem(w, request, http.StatusBadRequest, "log_filter_invalid", "Log filter invalid", err.Error())
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeProblem(w, request, http.StatusInternalServerError, "log_stream_unavailable", "Log stream unavailable", "The HTTP transport does not support streaming responses.")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	if err := writeLogEvents(w, initial, &filter); err != nil {
		return
	}
	flusher.Flush()

	poll := time.NewTicker(500 * time.Millisecond)
	heartbeat := time.NewTicker(15 * time.Second)
	defer poll.Stop()
	defer heartbeat.Stop()
	for {
		select {
		case <-request.Context().Done():
			return
		case <-heartbeat.C:
			if _, err := fmt.Fprint(w, ": keepalive\n\n"); err != nil {
				return
			}
			flusher.Flush()
		case <-poll.C:
			entries, err := handler.commands.TailLogs(request.Context(), filter)
			if err != nil {
				return
			}
			if len(entries) == 0 {
				continue
			}
			if err := writeLogEvents(w, entries, &filter); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}

func writeLogEvents(
	w http.ResponseWriter,
	entries []store.LogEntry,
	filter *application.LogTailRequest,
) error {
	for _, entry := range entries {
		encoded, err := json.Marshal(entry)
		if err != nil {
			return err
		}
		if _, err := fmt.Fprintf(
			w,
			"id: %s\nevent: log\ndata: %s\n\n",
			logEventID(entry),
			encoded,
		); err != nil {
			return err
		}
		filter.After = &store.LogCursor{Time: entry.Time, ID: entry.ID}
		filter.Since = nil
	}
	return nil
}

func logEventID(entry store.LogEntry) string {
	return entry.Time.UTC().Format(time.RFC3339Nano) + "|" + entry.ID
}

func parseLogEventID(value string) (*store.LogCursor, error) {
	rawTime, identifier, ok := strings.Cut(value, "|")
	if !ok || !validStableIdentifier(identifier) {
		return nil, fmt.Errorf("invalid log event id")
	}
	parsed, err := time.Parse(time.RFC3339Nano, rawTime)
	if err != nil {
		return nil, fmt.Errorf("invalid log event time: %w", err)
	}
	return &store.LogCursor{Time: parsed.UTC(), ID: identifier}, nil
}

func (handler *Handler) listDurableLogs(w http.ResponseWriter, request *http.Request) {
	if !handler.requireCommands(w, request) {
		return
	}
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
	page, err := handler.commands.ListLogs(request.Context(), application.LogListRequest{
		Source: store.LogSource(query.Get("source")), Level: store.LogLevel(query.Get("level")),
		Code: query.Get("code"), Since: since, Until: until, Cursor: cursor, Limit: limit,
	})
	if err != nil {
		writeProblem(w, request, http.StatusBadRequest, "log_filter_invalid", "Log filter invalid", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, page)
}

func (handler *Handler) getDurableLog(w http.ResponseWriter, request *http.Request, identifier string) {
	if !handler.requireCommands(w, request) {
		return
	}
	if _, ok := strictCoreQuery(w, request); !ok {
		return
	}
	entry, err := handler.commands.Log(request.Context(), identifier)
	if err != nil {
		if application.IsLogNotFound(err) {
			writeProblem(w, request, http.StatusNotFound, "log_not_found", "Log entry not found", "The requested sanitized log entry does not exist.")
			return
		}
		writeProblem(w, request, http.StatusBadRequest, "log_id_invalid", "Log ID invalid", "The log entry ID is invalid.")
		return
	}
	writeJSON(w, http.StatusOK, entry)
}

func (handler *Handler) currentMetrics(w http.ResponseWriter, request *http.Request) {
	if !handler.requireCommands(w, request) {
		return
	}
	if _, ok := strictCoreQuery(w, request); !ok {
		return
	}
	result, err := handler.commands.Metrics(request.Context())
	if err != nil {
		writeProblem(w, request, http.StatusServiceUnavailable, "metrics_unavailable", "Metrics unavailable", "Collector-backed metrics could not be read.")
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (handler *Handler) trafficStatus(w http.ResponseWriter, request *http.Request) {
	if !handler.requireCommands(w, request) {
		return
	}
	if _, ok := strictCoreQuery(w, request); !ok {
		return
	}
	result, err := handler.commands.TrafficStatus(request.Context())
	if err != nil {
		writeProblem(w, request, http.StatusServiceUnavailable, "traffic_status_unavailable", "Traffic status unavailable", "Collector-backed traffic status could not be read.")
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (handler *Handler) listTrafficPeriods(w http.ResponseWriter, request *http.Request) {
	if !handler.requireCommands(w, request) {
		return
	}
	query, ok := strictCoreQuery(w, request, "activation_bundle_id", "from", "to", "limit")
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
	periods, err := handler.commands.ListTrafficPeriods(request.Context(), store.TrafficPeriodFilter{
		ActivationBundleID: query.Get("activation_bundle_id"), OverlapsStart: from, OverlapsEnd: to, Limit: limit,
	})
	if err != nil {
		writeProblem(w, request, http.StatusBadRequest, "traffic_filter_invalid", "Traffic filter invalid", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, struct {
		Items []store.TrafficPeriod `json:"items"`
	}{Items: periods})
}

func (handler *Handler) getTrafficPeriod(w http.ResponseWriter, request *http.Request, identifier string) {
	if !handler.requireCommands(w, request) {
		return
	}
	if _, ok := strictCoreQuery(w, request); !ok {
		return
	}
	period, err := handler.commands.TrafficPeriod(request.Context(), identifier)
	if err != nil {
		if application.IsTrafficPeriodNotFound(err) {
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
	if err != nil || !validStableIdentifier(identifier) {
		writeProblem(w, request, http.StatusBadRequest, "log_cursor_invalid", "Log cursor invalid", "The log cursor is invalid.")
		return nil, false
	}
	return &store.LogCursor{Time: value.UTC(), ID: identifier}, true
}
