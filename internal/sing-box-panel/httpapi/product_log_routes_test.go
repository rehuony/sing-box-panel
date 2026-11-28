// SPDX-License-Identifier: GPL-3.0-or-later
package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/fleet"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/store"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/telemetry"
)

func TestPanelLogsIncludeCompletedOperations(t *testing.T) {
	handler, _ := newCoreHTTPFixture(t)
	handler.services.Telemetry.RecordOperation(context.Background(), "catalog.refresh", "Catalog refresh", nil, telemetry.OperationLogContext{})
	response := authenticatedRequest(t, handler, "GET", "/api/v1/logs/panel?limit=5", "", "")
	var page store.PanelLogPage
	if response.Code != 200 || json.Unmarshal(response.Body.Bytes(), &page) != nil {
		t.Fatal(response.Code, response.Body.String())
	}
	found := false
	for _, entry := range page.Items {
		if entry.Code == "catalog.refresh.completed" {
			found = true
		}
	}
	if !found {
		t.Fatal(page)
	}
	for _, path := range []string{"/api/v1/tasks", "/api/v1/tasks/old/retry", "/api/v1/tasks/old/cancel"} {
		if response := authenticatedRequest(t, handler, http.MethodPost, path, "", ""); response.Code != 404 {
			t.Fatalf("removed endpoint: %s %d", path, response.Code)
		}
	}
}

func TestPanelLogSearchCodesValidationAndMatching(t *testing.T) {
	handler, _ := newCoreHTTPFixture(t)
	handler.services.Telemetry.RecordOperation(t.Context(), "runtime.start", "Core start", nil, telemetry.OperationLogContext{})
	for _, query := range []string{
		"search_codes=", "search_codes=runtime.start.completed,", "search_codes=INVALID",
		"search_codes=a&search_codes=b", "search_codes=a%27OR1", "search_codes=" + strings.Repeat("a", 129),
		"search_codes=" + strings.Repeat("a,", store.MaximumPanelLogSearchCodes) + "a",
		"search_codes=" + strings.Repeat("a", store.MaximumPanelLogSearchCodes*(store.MaximumLogCodeBytes+1)+1),
	} {
		response := authenticatedRequest(t, handler, http.MethodGet, "/api/v1/logs/panel?"+query, "", "")
		if response.Code != http.StatusBadRequest {
			t.Fatalf("invalid codes accepted: %d %s", response.Code, response.Body.String())
		}
	}
	response := authenticatedRequest(t, handler, http.MethodGet, "/api/v1/logs/panel?search=%E6%A0%B8%E5%BF%83&search_codes=runtime.start.completed&level=info", "", "")
	var page store.PanelLogPage
	if response.Code != 200 || json.Unmarshal(response.Body.Bytes(), &page) != nil || page.Total != 1 || len(page.Items) != 1 || page.Items[0].Code != "runtime.start.completed" {
		t.Fatalf("translated search: %d %s", response.Code, response.Body.String())
	}
	unauthenticated := httptest.NewRecorder()
	handler.ServeHTTP(unauthenticated, httptest.NewRequest(http.MethodGet, "/api/v1/logs/panel?search_codes=runtime.start.completed", nil))
	if unauthenticated.Code != http.StatusUnauthorized {
		t.Fatal(unauthenticated.Code)
	}
}

// A log request reads, counts and clears the panel's own entries or one
// node's; clearing a node's entries leaves the record of it among them.
func TestPanelLogsScopeToThePanelOrOneNode(t *testing.T) {
	handler, _ := newCoreHTTPFixture(t)
	for _, node := range []string{"node-a", "node-b", ""} {
		handler.services.Telemetry.RecordOperation(t.Context(), "runtime.start", "Core start", nil, telemetry.OperationLogContext{NodeID: node})
	}
	list := func(query string) store.PanelLogPage {
		t.Helper()
		response := authenticatedRequest(t, handler, http.MethodGet, "/api/v1/logs/panel?"+query, "", "")
		var page store.PanelLogPage
		if response.Code != 200 || json.Unmarshal(response.Body.Bytes(), &page) != nil {
			t.Fatalf("list %s: %d %s", query, response.Code, response.Body.String())
		}
		return page
	}
	if page := list("node_id=node-a"); page.Total != 1 {
		t.Fatalf("node scope: %+v", page)
	}
	if page := list("scope=panel"); page.Total != 1 || len(page.Items) != 1 || strings.Contains(string(page.Items[0].Metadata), "node_id") {
		t.Fatalf("panel scope: %+v", page)
	}
	for _, query := range []string{"node_id=" + strings.Repeat("a", 129), "scope=node", "scope=panel&node_id=node-a"} {
		if response := authenticatedRequest(t, handler, http.MethodGet, "/api/v1/logs/panel?"+query, "", ""); response.Code != http.StatusBadRequest {
			t.Fatalf("%s accepted: %d", query, response.Code)
		}
	}
	usage := authenticatedRequest(t, handler, http.MethodGet, "/api/v1/logs/usage?node_id=node-b", "", "")
	if usage.Code != http.StatusOK || !strings.Contains(usage.Body.String(), `"entries":1`) {
		t.Fatalf("node usage: %d %s", usage.Code, usage.Body.String())
	}
	cleared := authenticatedRequest(t, handler, http.MethodDelete, "/api/v1/logs?node_id=node-a", "", "")
	if cleared.Code != http.StatusOK || !strings.Contains(cleared.Body.String(), `"deleted":1`) {
		t.Fatalf("node clear: %d %s", cleared.Code, cleared.Body.String())
	}
	if page := list("node_id=node-a"); page.Total != 1 || page.Items[0].Code != "logs.cleared" {
		t.Fatalf("node-a after clearing: %+v", page)
	}
	if list("node_id=node-b").Total != 1 || list("scope=panel").Total != 1 {
		t.Fatal("clearing one node touched other entries")
	}
}

func TestPanelLogSearchCodesAcceptsFullDocumentedRange(t *testing.T) {
	handler, _ := newCoreHTTPFixture(t)
	codes := make([]string, store.MaximumPanelLogSearchCodes)
	for i := range codes {
		codes[i] = fmt.Sprintf("x%03d%s", i, strings.Repeat("a", store.MaximumLogCodeBytes-4))
	}
	_, err := handler.services.Telemetry.RecordLog(t.Context(), telemetry.LogRecordRequest{
		Source: store.LogSourcePanel, Level: store.LogLevelInfo, Code: codes[len(codes)-1],
		Message: "Boundary search event", Metadata: json.RawMessage(`{}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	var fullyEscaped strings.Builder
	for _, b := range []byte(strings.Join(codes, ",")) {
		fmt.Fprintf(&fullyEscaped, "%%%02X", b)
	}
	for _, encoded := range []string{url.QueryEscape(strings.Join(codes, ",")), fullyEscaped.String()} {
		query := url.Values{"search": {strings.Repeat("z", 256)}, "level": {"info"}, "limit": {"1"}, "offset": {"0"}}
		response := authenticatedRequest(t, handler, http.MethodGet, "/api/v1/logs/panel?"+query.Encode()+"&search_codes="+encoded, "", "")
		var page store.PanelLogPage
		if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &page) != nil || page.Total != 1 || len(page.Items) != 1 {
			t.Fatalf("documented search range rejected: %d %s", response.Code, response.Body.String())
		}
	}
	response := authenticatedRequest(t, handler, http.MethodGet, "/api/v1/logs/panel?search_codes="+strings.Repeat("a", maximumPanelLogQueryBytes), "", "")
	assertCoreHTTPProblem(t, response, http.StatusBadRequest, "query_invalid")
}

type cancelingLogRecorder struct {
	*httptest.ResponseRecorder
	cancel context.CancelFunc
}

func (r *cancelingLogRecorder) Flush() { r.ResponseRecorder.Flush(); r.cancel() }

// coreLogFile is an append-only core log file with the node's read rules.
type coreLogFile struct {
	generation string
	content    []byte
	// generations holds each generation's content, which only grows.
	generations map[string][]byte
}

func (file *coreLogFile) read(cursor coreLogCursor) fleet.CoreLogChunk {
	offset, reset := cursor.offset, cursor.generation != "" && cursor.generation != file.generation
	if reset {
		offset = 0
	}
	data := file.content[offset:min(len(file.content), int(offset)+48)]
	if end := bytes.LastIndexByte(data, '\n'); end >= 0 {
		data = data[:end+1]
	} else {
		data = nil
	}
	return fleet.CoreLogChunk{
		File: "core.log", Text: strings.ToValidUTF8(string(data), "�"), NextOffset: offset + int64(len(data)),
		Size: int64(len(file.content)), Generation: file.generation, Reset: reset,
	}
}

// Streams that take the shared reads late, miss some or read for themselves
// assemble the file as the node holds it, across clears.
func TestCoreLogStreamsAssembleTheFileFromSharedReads(t *testing.T) {
	random := rand.New(rand.NewPCG(7, 11))
	file := &coreLogFile{generation: "g0", generations: map[string][]byte{}}
	type stream struct {
		cursor  coreLogCursor
		text    string
		pending *coreLogRead
	}
	streams := []*stream{{cursor: coreLogCursor{generation: "g0"}}, {cursor: coreLogCursor{generation: "g0"}}, {cursor: coreLogCursor{generation: "g0"}}}
	shared := coreLogCursor{generation: "g0"}
	apply := func(target *stream, chunk fleet.CoreLogChunk) {
		if chunk.Reset {
			target.text = ""
		}
		target.text += chunk.Text
		target.cursor = coreLogCursor{offset: chunk.NextOffset, generation: chunk.Generation}
		if want := strings.ToValidUTF8(string(file.generations[target.cursor.generation][:target.cursor.offset]), "�"); target.text != want {
			t.Fatalf("stream text=%q, want %q", target.text, want)
		}
	}
	follow := func(target *stream, read coreLogRead) {
		next, ok := read.continuation(target.cursor)
		if !ok {
			next = file.read(target.cursor)
		}
		apply(target, next)
	}
	pieces := []string{"a", "bc", "é", "\n", "\xff", "\xf8\xf9\xfa\xfb\xfc", "line\n", "\n\n"}
	for range 5000 {
		switch step := random.IntN(20); {
		case step < 10:
			file.content = append(file.content, pieces[random.IntN(len(pieces))]...)
		case step == 10 && random.IntN(10) == 0:
			file.generation, file.content = fmt.Sprint("g", random.Uint64()), nil
		case step < 15:
			read := coreLogRead{from: shared, chunk: file.read(shared)}
			shared = coreLogCursor{offset: read.chunk.NextOffset, generation: read.chunk.Generation}
			for _, target := range streams {
				// A stream that has not taken the last read misses it.
				target.pending = &read
			}
		default:
			target := streams[random.IntN(len(streams))]
			if target.pending != nil {
				follow(target, *target.pending)
				target.pending = nil
			}
		}
		file.generations[file.generation] = file.content
	}
	for index, target := range streams {
		for range 1000 {
			next := file.read(target.cursor)
			if next.Text == "" && !next.Reset {
				break
			}
			apply(target, next)
		}
		if complete := bytes.LastIndexByte(file.content, '\n') + 1; target.cursor != (coreLogCursor{offset: int64(complete), generation: file.generation}) {
			t.Fatalf("stream %d ended at %+v, want offset %d of %s", index, target.cursor, complete, file.generation)
		}
	}
}

// Streams of one file share one reader, which stops with the last of them.
func TestCoreLogStreamsShareOneReader(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		registry := coreLogRegistry{readers: map[coreLogKey]*coreLogReader{}}
		key := coreLogKey{nodeID: "node_edge", file: "core.log"}
		var reads atomic.Int32
		read := func(_ context.Context, cursor coreLogCursor) (fleet.CoreLogChunk, error) {
			reads.Add(1)
			return fleet.CoreLogChunk{File: "core.log", NextOffset: cursor.offset, Generation: cursor.generation}, nil
		}
		first, leaveFirst := registry.join(key, coreLogCursor{generation: "g0"}, read)
		second, leaveSecond := registry.join(key, coreLogCursor{generation: "g0"}, read)
		time.Sleep(3 * coreLogReadInterval)
		synctest.Wait()
		if reads.Load() != 3 || len(first) != 1 || len(second) != 1 {
			t.Fatalf("reads=%d for two streams over three intervals; want one reader", reads.Load())
		}
		leaveFirst()
		leaveSecond()
		time.Sleep(3 * coreLogReadInterval)
		synctest.Wait()
		if reads.Load() != 3 || len(registry.readers) != 0 {
			t.Fatalf("reads=%d readers=%d after the last stream left", reads.Load(), len(registry.readers))
		}
	})
}

// Every stream of an empty file, one that joins late included, gets its own
// first frame and no other until the file grows; then each gets the output.
func TestCoreLogStreamsOfAnEmptyFileOpenWithOneFrame(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var mu sync.Mutex
		file := &coreLogFile{generation: "g0"}
		read := func(_ context.Context, cursor coreLogCursor) (fleet.CoreLogChunk, error) {
			mu.Lock()
			defer mu.Unlock()
			return file.read(cursor), nil
		}
		key := coreLogKey{nodeID: "node_empty", file: "core.log"}
		ctx, cancel := context.WithCancel(t.Context())
		var streams sync.WaitGroup
		open := func() *streamRecorder {
			response := &streamRecorder{header: http.Header{}}
			first, _ := read(ctx, coreLogCursor{})
			streams.Go(func() { followCoreLog(ctx, response, key, first, read) })
			return response
		}
		frames := func(response *streamRecorder) int { return strings.Count(response.String(), "event: output\n") }
		early := open()
		time.Sleep(3 * coreLogReadInterval)
		late := open()
		time.Sleep(3 * coreLogReadInterval)
		synctest.Wait()
		if frames(early) != 1 || frames(late) != 1 {
			t.Fatalf("frames early=%d late=%d; want each stream's first frame only", frames(early), frames(late))
		}
		mu.Lock()
		file.content = append(file.content, "started\n"...)
		mu.Unlock()
		time.Sleep(coreLogReadInterval)
		synctest.Wait()
		for _, response := range []*streamRecorder{early, late} {
			if frames(response) != 2 || !strings.Contains(response.String(), `"text":"started\n"`) {
				t.Fatalf("stream after output: %s", response.String())
			}
		}
		cancel()
		streams.Wait()
	})
}
