// SPDX-License-Identifier: GPL-3.0-or-later
package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

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

// readCoreLog reads a node's core log. The stream follows the file until it
// closes after a minute for the client to reconnect.
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
	fleetService, file := handler.services.Fleet, chunk.File
	followCoreLog(r.Context(), w, coreLogKey{fleet: fleetService, nodeID: nodeID, file: file}, chunk,
		func(ctx context.Context, cursor coreLogCursor) (fleet.CoreLogChunk, error) {
			return fleetService.ReadCoreLog(ctx, nodeID, file, cursor.offset, cursor.generation)
		})
}

// followCoreLog streams first, the stream's own read of the file, even when
// it is empty, since it opens the stream and gives the client its cursor;
// then what is appended to the file, from the reads every stream of key
// shares, or from read when they do not show what follows this stream's
// cursor, such as after it fell behind them.
func followCoreLog(
	ctx context.Context, w http.ResponseWriter, key coreLogKey, first fleet.CoreLogChunk,
	read func(context.Context, coreLogCursor) (fleet.CoreLogChunk, error),
) {
	ctx, cancel := context.WithTimeout(ctx, coreLogStreamLifetime)
	defer cancel()
	deadline, _ := ctx.Deadline()
	events, ok := newEventStream(ctx, w, deadline, maximumSnapshotFrameBytes)
	if !ok {
		return
	}
	send := func(chunk fleet.CoreLogChunk) error {
		data, err := json.Marshal(chunk)
		if err != nil {
			return err
		}
		return events.event("output", data)
	}
	if err := send(first); err != nil {
		return
	}
	cursor := coreLogCursor{offset: first.NextOffset, generation: first.Generation}
	reads, leave := coreLogReaders.join(key, cursor, read)
	defer leave()
	heartbeat := time.NewTicker(10 * time.Second)
	defer heartbeat.Stop()
	rotation := rotationTimer(coreLogStreamLifetime)
	defer rotation.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-rotation.C:
			_ = events.reconnect()
			return
		case <-heartbeat.C:
			if events.heartbeat() != nil {
				return
			}
		case shared := <-reads:
			if shared.err != nil {
				return
			}
			next, ok := shared.continuation(cursor)
			if !ok {
				var err error
				if next, err = read(ctx, cursor); err != nil {
					return
				}
			}
			cursor = coreLogCursor{offset: next.NextOffset, generation: next.Generation}
			if next.Text == "" && !next.Reset {
				continue
			}
			if send(next) != nil {
				return
			}
		}
	}
}

// coreLogStreamLifetime is how long one core log stream runs before it asks
// its client to reconnect.
const coreLogStreamLifetime = time.Minute

// coreLogReadInterval is how often a streamed core log file is read.
const coreLogReadInterval = time.Second

// coreLogCursor is a position in a node's core log file: a byte offset in
// one generation of the file.
type coreLogCursor struct {
	offset     int64
	generation string
}

// coreLogRead is one read of a shared core log reader: the cursor it read
// from and the chunk the node returned.
type coreLogRead struct {
	from  coreLogCursor
	chunk fleet.CoreLogChunk
	err   error
}

// continuation returns the chunk that reading from cursor at the time of the
// read returns, or the part of it the read holds, and false when the read
// holds none of what follows cursor. Within a generation the file is only
// appended to, and a stale generation restarts at zero.
func (read coreLogRead) continuation(cursor coreLogCursor) (fleet.CoreLogChunk, bool) {
	chunk := read.chunk
	if cursor == read.from {
		return chunk, true
	}
	if cursor.generation != chunk.Generation {
		if chunk.Reset && cursor.generation != "" {
			// A read in a stale generation restarts at zero, as this one did.
			return chunk, true
		}
		return fleet.CoreLogChunk{}, false
	}
	start := read.from.offset
	if chunk.Reset {
		start = 0
	}
	switch {
	case cursor.offset >= chunk.NextOffset:
		// The cursor is past what the read reached: nothing new yet.
		return fleet.CoreLogChunk{File: chunk.File, NextOffset: cursor.offset, Size: chunk.Size, Generation: chunk.Generation}, true
	case cursor.offset < start:
		return fleet.CoreLogChunk{}, false
	}
	// The text is the file's bytes from start unless the node replaced
	// invalid UTF-8; only then does an offset into the file index it.
	if strings.ContainsRune(chunk.Text, utf8.RuneError) || int64(len(chunk.Text)) != chunk.NextOffset-start {
		return fleet.CoreLogChunk{}, false
	}
	return fleet.CoreLogChunk{
		File: chunk.File, Text: strings.ToValidUTF8(chunk.Text[cursor.offset-start:], string(utf8.RuneError)),
		NextOffset: chunk.NextOffset, Size: chunk.Size, Generation: chunk.Generation,
	}, true
}

// coreLogKey names one node's core log file as one fleet reaches it.
type coreLogKey struct {
	fleet  *fleet.Service
	nodeID string
	file   string
}

// coreLogReaders shares one reader per streamed core log file. A reader runs
// while a stream follows its file and is dropped with the last one.
var coreLogReaders = coreLogRegistry{readers: map[coreLogKey]*coreLogReader{}}

type coreLogRegistry struct {
	mu      sync.Mutex
	readers map[coreLogKey]*coreLogReader
}

// coreLogReader reads one file every coreLogReadInterval and hands each
// stream the newest read; a stream that misses one reads for itself.
type coreLogReader struct {
	streams map[chan coreLogRead]struct{}
	stop    context.CancelFunc
}

// join follows key with the streams already following it, or starts a reader
// from cursor with read, and returns the reads and a function that leaves.
func (registry *coreLogRegistry) join(
	key coreLogKey, cursor coreLogCursor, read func(context.Context, coreLogCursor) (fleet.CoreLogChunk, error),
) (<-chan coreLogRead, func()) {
	registry.mu.Lock()
	defer registry.mu.Unlock()
	reader := registry.readers[key]
	if reader == nil {
		ctx, stop := context.WithCancel(context.Background())
		reader = &coreLogReader{streams: map[chan coreLogRead]struct{}{}, stop: stop}
		registry.readers[key] = reader
		go registry.run(ctx, reader, cursor, read)
	}
	reads := make(chan coreLogRead, 1)
	reader.streams[reads] = struct{}{}
	var once sync.Once
	return reads, func() {
		once.Do(func() {
			registry.mu.Lock()
			defer registry.mu.Unlock()
			delete(reader.streams, reads)
			if len(reader.streams) == 0 {
				reader.stop()
				delete(registry.readers, key)
			}
		})
	}
}

func (registry *coreLogRegistry) run(ctx context.Context, reader *coreLogReader, cursor coreLogCursor, read func(context.Context, coreLogCursor) (fleet.CoreLogChunk, error)) {
	ticker := time.NewTicker(coreLogReadInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		chunk, err := read(ctx, cursor)
		if ctx.Err() != nil {
			return
		}
		result := coreLogRead{from: cursor, chunk: chunk, err: err}
		if err == nil {
			cursor = coreLogCursor{offset: chunk.NextOffset, generation: chunk.Generation}
		}
		registry.mu.Lock()
		for reads := range reader.streams {
			select {
			case <-reads:
			default:
			}
			reads <- result
		}
		registry.mu.Unlock()
	}
}

// Covers all 256 codes of 128 bytes even when every byte is percent-encoded,
// plus the other filters. Other endpoints retain their existing query limit.
const maximumPanelLogQueryBytes = 128 << 10

func (handler *Handler) listPanelLogs(w http.ResponseWriter, r *http.Request) {
	query, ok := strictQueryWithLimit(w, r, maximumPanelLogQueryBytes, "level", "since", "until", "before_time", "before_id", "limit", "search", "search_codes", "node_id", "scope", "offset")
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
	scope, ok := logScope(w, r, query)
	if !ok {
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
	result, err := handler.services.Telemetry.PanelLogs(r.Context(), store.PanelLogFilter{Cursor: cursor, Limit: limit, Offset: offset, Level: query.Get("level"), Since: since, Until: until, Search: query.Get("search"), SearchCodes: searchCodes, NodeID: scope.NodeID, PanelOnly: scope.Panel})
	if err != nil {
		writeProblem(w, r, 503, "panel_logs_unavailable", "Panel logs unavailable", "The panel activity could not be read.")
		return
	}
	writeJSON(w, 200, result)
}

// logScope reads a log request's scope: the panel's own entries with
// scope=panel, one node's with node_id, never both.
func logScope(w http.ResponseWriter, r *http.Request, query url.Values) (store.LogScope, bool) {
	nodeID := query.Get("node_id")
	if len(nodeID) > 128 {
		writeProblem(w, r, 400, "node_id_invalid", "Invalid node", "The node ID may contain at most 128 bytes.")
		return store.LogScope{}, false
	}
	scope := query.Get("scope")
	if scope != "" && scope != "panel" {
		writeProblem(w, r, 400, "log_scope_invalid", "Invalid log scope", "The scope may only be panel.")
		return store.LogScope{}, false
	}
	if scope != "" && nodeID != "" {
		writeProblem(w, r, 400, "log_scope_invalid", "Invalid log scope", "Choose the panel scope or one node, not both.")
		return store.LogScope{}, false
	}
	return store.LogScope{NodeID: nodeID, Panel: scope == "panel"}, true
}

func (handler *Handler) logUsage(w http.ResponseWriter, r *http.Request) {
	query, ok := strictCoreQuery(w, r, "scope", "node_id")
	if !ok {
		return
	}
	scope, ok := logScope(w, r, query)
	if !ok {
		return
	}
	usage, err := handler.services.Telemetry.LogUsage(r.Context(), scope)
	if err != nil {
		writeProblem(w, r, 503, "records_unavailable", "Records unavailable", "The panel could not read its records. Retry later.")
		return
	}
	writeJSON(w, 200, usage)
}
