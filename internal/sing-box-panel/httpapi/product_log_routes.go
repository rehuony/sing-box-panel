// SPDX-License-Identifier: GPL-3.0-or-later
package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/fleet"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/store"
)

func (handler *Handler) listCoreLogFiles(w http.ResponseWriter, r *http.Request) {
	if _, ok := strictCoreQuery(w, r); !ok {
		return
	}
	files, err := handler.services.Fleet.CoreLogFiles(r.Context(), requestNodeID(r))
	if err != nil {
		writeCoreLogProblem(w, r, "core_logs_unavailable", err)
		return
	}
	writeJSON(w, 200, map[string]any{"items": files})
}

func (handler *Handler) deleteCoreLogFile(w http.ResponseWriter, r *http.Request) {
	query, ok := strictCoreQuery(w, r, "file")
	if !ok || !requireEmptyCoreBody(w, r) {
		return
	}
	if err := handler.services.Fleet.DeleteCoreLog(r.Context(), requestNodeID(r), query.Get("file")); err != nil {
		writeCoreLogProblem(w, r, "core_log_delete_failed", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (handler *Handler) clearCoreLog(w http.ResponseWriter, r *http.Request) {
	query, ok := strictCoreQuery(w, r, "file")
	if !ok || !requireEmptyCoreBody(w, r) {
		return
	}
	if err := handler.services.Fleet.ClearCoreLog(r.Context(), requestNodeID(r), query.Get("file")); err != nil {
		writeCoreLogProblem(w, r, "core_log_clear_failed", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// writeCoreLogProblem names a missing file as a missing log file.
func writeCoreLogProblem(w http.ResponseWriter, r *http.Request, code string, err error) {
	if fleet.IsProblem(err, "filesystem_not_found") {
		writeProblem(w, r, http.StatusNotFound, "core_log_not_found", "Log file not found", "The selected log file no longer exists.")
		return
	}
	writeNodeProblem(w, r, code, err)
}

// readCoreLog reads a node's core log. The stream reads the node again every
// second until it closes after a minute for the client to reconnect.
func (handler *Handler) readCoreLog(w http.ResponseWriter, r *http.Request, stream bool) {
	query, ok := strictCoreQuery(w, r, "file", "offset", "generation")
	if !ok {
		return
	}
	if len(query.Get("generation")) > 128 {
		writeProblem(w, r, 400, "log_generation_invalid", "Invalid generation", "Use the generation returned by the previous read.")
		return
	}
	offset := int64(-1)
	var err error
	if query.Get("offset") != "" {
		offset, err = strconv.ParseInt(query.Get("offset"), 10, 64)
		if err != nil {
			writeProblem(w, r, 400, "log_offset_invalid", "Invalid offset", "Use a valid byte offset.")
			return
		}
	}
	nodeID := requestNodeID(r)
	chunk, err := handler.services.Fleet.ReadCoreLog(r.Context(), nodeID, query.Get("file"), offset, query.Get("generation"))
	if err != nil {
		writeCoreLogProblem(w, r, "core_log_unavailable", err)
		return
	}
	if !stream {
		writeJSON(w, 200, chunk)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), coreLogStreamLifetime)
	defer cancel()
	deadline, _ := ctx.Deadline()
	events, ok := newEventStream(ctx, w, deadline, maximumSnapshotFrameBytes)
	if !ok {
		return
	}
	send := func() error {
		data, err := json.Marshal(chunk)
		if err != nil {
			return err
		}
		return events.event("output", data)
	}
	if err := send(); err != nil {
		return
	}
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	rotation := rotationTimer(coreLogStreamLifetime)
	defer rotation.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-rotation.C:
			_ = events.reconnect()
			return
		case <-ticker.C:
			chunk, err = handler.services.Fleet.ReadCoreLog(ctx, nodeID, chunk.File, chunk.NextOffset, chunk.Generation)
			if err != nil {
				return
			}
			if err := send(); err != nil {
				return
			}
		}
	}
}

// coreLogStreamLifetime is how long one core log stream runs before it asks
// its client to reconnect.
const coreLogStreamLifetime = time.Minute

// Covers all 128 codes of 128 bytes even when every byte is percent-encoded,
// plus the other filters. Other endpoints retain their existing query limit.
const maximumPanelLogQueryBytes = 64 << 10

func (handler *Handler) listPanelLogs(w http.ResponseWriter, r *http.Request) {
	query, ok := strictQueryWithLimit(w, r, maximumPanelLogQueryBytes, "level", "since", "until", "before_time", "before_id", "limit", "search", "search_codes", "offset")
	if !ok {
		return
	}
	limit, ok := optionalLimit(w, r)
	if !ok {
		return
	}
	offset, ok := optionalPageOffset(w, r)
	if !ok {
		return
	}
	since, ok := optionalHTTPTime(w, r, query.Get("since"), "since")
	if !ok {
		return
	}
	until, ok := optionalHTTPTime(w, r, query.Get("until"), "until")
	if !ok {
		return
	}
	if since != nil && until != nil && !until.After(*since) {
		writeProblem(w, r, 400, "log_range_invalid", "Invalid range", "The end must be after the start.")
		return
	}
	cursor, ok := optionalLogCursor(w, r, query.Get("before_time"), query.Get("before_id"))
	if !ok {
		return
	}
	if len(query.Get("search")) > 256 {
		writeProblem(w, r, 400, "search_too_long", "Invalid search", "Search may contain at most 256 bytes.")
		return
	}
	var searchCodes []string
	if raw, present := query["search_codes"]; present {
		if len(raw[0]) > store.MaximumPanelLogSearchCodes*(store.MaximumLogCodeBytes+1) {
			writeProblem(w, r, 400, "search_codes_invalid", "Invalid search codes", "The event code alternatives exceed the search limit.")
			return
		}
		searchCodes = strings.Split(raw[0], ",")
		if err := store.ValidatePanelLogSearchCodes(searchCodes); err != nil {
			writeProblem(w, r, 400, "search_codes_invalid", "Invalid search codes", err.Error())
			return
		}
	}
	result, err := handler.services.Telemetry.PanelLogs(r.Context(), store.PanelLogFilter{Cursor: cursor, Limit: limit, Offset: offset, Level: query.Get("level"), Since: since, Until: until, Search: query.Get("search"), SearchCodes: searchCodes})
	if err != nil {
		writeProblem(w, r, 503, "panel_logs_unavailable", "Panel logs unavailable", "The panel activity could not be read.")
		return
	}
	writeJSON(w, 200, result)
}
