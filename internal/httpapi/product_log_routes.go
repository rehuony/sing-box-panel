// SPDX-License-Identifier: GPL-3.0-or-later
package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/rehuony/sing-box-panel/internal/corelogs"
	"github.com/rehuony/sing-box-panel/internal/store"
	"net/http"
	"os"
	"strconv"
	"time"
)

func (handler *Handler) listCoreLogFiles(w http.ResponseWriter, r *http.Request) {
	if !handler.requireCommands(w, r) {
		return
	}
	if _, ok := strictCoreQuery(w, r); !ok {
		return
	}
	files, err := handler.commands.CoreLogFiles()
	if err != nil {
		writeProblem(w, r, 503, "core_logs_unavailable", "Core logs unavailable", "Captured process output could not be read.")
		return
	}
	writeJSON(w, 200, map[string]any{"items": files})
}
func (handler *Handler) readCoreLog(w http.ResponseWriter, r *http.Request, stream bool) {
	if !handler.requireCommands(w, r) {
		return
	}
	query, ok := strictCoreQuery(w, r, "file", "offset")
	if !ok {
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
	chunk, err := handler.commands.CoreLogContent(query.Get("file"), offset)
	if err != nil {
		status := 503
		if errors.Is(err, corelogs.ErrInvalidFile) {
			status = 400
		} else if errors.Is(err, os.ErrNotExist) {
			status = 404
		}
		writeProblem(w, r, status, "core_log_unavailable", "Core log unavailable", "The selected captured output file is unavailable.")
		return
	}
	if !stream {
		writeJSON(w, 200, chunk)
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		return
	}
	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Accel-Buffering", "no")
	send := func() error {
		data, err := json.Marshal(chunk)
		if err != nil {
			return err
		}
		_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(10 * time.Second))
		_, err = fmt.Fprintf(w, "event: output\ndata: %s\n\n", data)
		flusher.Flush()
		return err
	}
	if err := send(); err != nil {
		return
	}
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	expiry := time.NewTimer(time.Minute)
	defer expiry.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-expiry.C:
			return
		case <-ticker.C:
			chunk, err = handler.commands.CoreLogContent(chunk.File, chunk.NextOffset)
			if err != nil {
				return
			}
			if err := send(); err != nil {
				return
			}
		}
	}
}
func (handler *Handler) listPanelLogs(w http.ResponseWriter, r *http.Request) {
	if !handler.requireCommands(w, r) {
		return
	}
	query, ok := strictCoreQuery(w, r, "level", "since", "until", "before_time", "before_id", "limit", "search", "offset")
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
	result, err := handler.commands.PanelLogs(r.Context(), store.PanelLogFilter{Cursor: cursor, Limit: limit, Offset: offset, Level: query.Get("level"), Since: since, Until: until, Search: query.Get("search")})
	if err != nil {
		writeProblem(w, r, 503, "panel_logs_unavailable", "Panel logs unavailable", "The panel activity could not be read.")
		return
	}
	writeJSON(w, 200, result)
}
