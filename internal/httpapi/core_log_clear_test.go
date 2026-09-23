// SPDX-License-Identifier: GPL-3.0-or-later
package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/rehuony/sing-box-panel/internal/corelogs"
)

func TestClearCoreLogPersistsAndRequiresAuthenticationAndCSRF(t *testing.T) {
	handler, _ := newCoreHTTPFixture(t)
	logs, err := corelogs.New(handler.settings.DataDir)
	if err != nil {
		t.Fatal(err)
	}
	writer := logs.Writer()
	if _, err := writer.Write([]byte("INFO old output\nERROR hidden output\n")); err != nil {
		t.Fatal(err)
	}
	files, err := logs.List()
	if err != nil || len(files) != 1 {
		t.Fatal(files, err)
	}
	name := files[0].Name
	archive := time.Now().UTC().AddDate(0, 0, -1).Format("2006-01-02") + "-000.log"
	archivePath := filepath.Join(handler.settings.DataDir, "logs", "core", archive)
	if err := os.WriteFile(archivePath, []byte("INFO unrelated\n"), 0600); err != nil {
		t.Fatal(err)
	}
	path := "/api/v1/core/logs/content?file=" + name
	unauthed := httptest.NewRecorder()
	handler.ServeHTTP(unauthed, httptest.NewRequest(http.MethodDelete, path, nil))
	if unauthed.Code != http.StatusUnauthorized {
		t.Fatal(unauthed.Code)
	}
	login := httptest.NewRecorder()
	handler.ServeHTTP(login, httptest.NewRequest(http.MethodPost, "/api/v1/auth/session", strings.NewReader(`{"token":"correct-management-token"}`)))
	var session struct {
		CSRF string `json:"csrfToken"`
	}
	if login.Code != http.StatusOK || json.Unmarshal(login.Body.Bytes(), &session) != nil {
		t.Fatal(login.Code, login.Body.String())
	}
	request := httptest.NewRequest(http.MethodDelete, path, nil)
	request.AddCookie(login.Result().Cookies()[0])
	request.Header.Set("Origin", "http://example.com")
	denied := httptest.NewRecorder()
	handler.ServeHTTP(denied, request)
	if denied.Code != http.StatusForbidden {
		t.Fatal(denied.Code, denied.Body.String())
	}
	for _, test := range []struct {
		query, body string
		status      int
	}{
		{"", "", http.StatusBadRequest},
		{"file=..%2Fnative.log", "", http.StatusBadRequest},
		{"file=2000-01-01-000.log", "", http.StatusNotFound},
		{"file=" + name + "&file=" + archive, "", http.StatusBadRequest},
		{"file=" + name + "&level=info", "", http.StatusBadRequest},
		{"file=" + name + "&search=old", "", http.StatusBadRequest},
		{"file=" + name + "&offset=0", "", http.StatusBadRequest},
		{"file=" + name, `{}`, http.StatusUnprocessableEntity},
	} {
		response := authenticatedRequest(handler, http.MethodDelete, "/api/v1/core/logs/content?"+test.query, test.body, "")
		if response.Code != test.status {
			t.Fatalf("%s: %d %s", test.query, response.Code, response.Body.String())
		}
	}
	chunk, err := logs.Read(name, 0, "")
	if err != nil || chunk.Text != "INFO old output\nERROR hidden output\n" {
		t.Fatalf("rejected requests changed contents: %+v, %v", chunk, err)
	}
	request = httptest.NewRequest(http.MethodDelete, path, nil)
	request.AddCookie(login.Result().Cookies()[0])
	request.Header.Set("Origin", "http://example.com")
	request.Header.Set("X-CSRF-Token", session.CSRF)
	cleared := httptest.NewRecorder()
	handler.ServeHTTP(cleared, request)
	if cleared.Code != http.StatusNoContent {
		t.Fatal(cleared.Code, cleared.Body.String())
	}
	read := func(want string) {
		t.Helper()
		for _, suffix := range []string{"", "&offset=0"} {
			response := authenticatedRequest(handler, http.MethodGet, path+suffix, "", "")
			var got corelogs.Chunk
			if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &got) != nil || got.Text != want || got.Size != int64(len(want)) || got.NextOffset != int64(len(want)) {
				t.Fatalf("reread: %d %s", response.Code, response.Body.String())
			}
		}
	}
	read("")
	if _, err := writer.Write([]byte("INFO new output\n")); err != nil {
		t.Fatal(err)
	}
	read("INFO new output\n")
	if content, err := os.ReadFile(archivePath); err != nil || string(content) != "INFO unrelated\n" {
		t.Fatalf("archive changed: %q, %v", content, err)
	}
	response := authenticatedRequest(handler, http.MethodGet, "/api/v1/logs/panel?search=Core%20log%20clearing", "", "")
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "core.log.clear.completed") || !strings.Contains(response.Body.String(), "core.log.clear.failed") {
		t.Fatal(response.Code, response.Body.String())
	}
}

func TestCoreLogStreamAndReconnectResetAfterClear(t *testing.T) {
	handler, _ := newCoreHTTPFixture(t)
	logs, err := corelogs.New(handler.settings.DataDir)
	if err != nil {
		t.Fatal(err)
	}
	handler.commands.SetCoreLogs(logs)
	writer := logs.Writer()
	if _, err := writer.Write([]byte("INFO old\n")); err != nil {
		t.Fatal(err)
	}
	files, err := logs.List()
	if err != nil || len(files) != 1 {
		t.Fatal(files, err)
	}
	name := files[0].Name
	want := strings.Repeat("INFO fresh output after clear\n", 8)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/core/logs/stream?file="+name, nil).WithContext(ctx)
	request.Header.Set("Authorization", "Bearer correct-management-token")
	flushes := 0
	stream := &coreLogFlushRecorder{ResponseRecorder: httptest.NewRecorder()}
	stream.onFlush = func() {
		flushes++
		if flushes != 1 {
			cancel()
			return
		}
		response := authenticatedRequest(handler, http.MethodDelete, "/api/v1/core/logs/content?file="+name, "", "")
		if response.Code != http.StatusNoContent {
			t.Fatal(response.Code, response.Body.String())
		}
		if _, err := writer.Write([]byte(want)); err != nil {
			t.Fatal(err)
		}
	}
	handler.ServeHTTP(stream, request)
	var chunks []corelogs.Chunk
	for line := range strings.SplitSeq(stream.Body.String(), "\n") {
		if data, ok := strings.CutPrefix(line, "data: "); ok {
			var chunk corelogs.Chunk
			if err := json.Unmarshal([]byte(data), &chunk); err != nil {
				t.Fatal(err)
			}
			chunks = append(chunks, chunk)
		}
	}
	if len(chunks) != 2 || chunks[0].Reset || !chunks[1].Reset || chunks[1].Text != want || chunks[0].Generation == chunks[1].Generation {
		t.Fatalf("stream did not restart after clear: %+v", chunks)
	}
	// Both a disconnected stream and a paused one-shot reader can resume an old
	// cursor after the file has already grown beyond that cursor.
	for _, endpoint := range []string{"content", "stream"} {
		path := fmt.Sprintf("/api/v1/core/logs/%s?file=%s&offset=%d&generation=%s", endpoint, name, chunks[0].NextOffset, chunks[0].Generation)
		request := httptest.NewRequest(http.MethodGet, path, nil)
		request.Header.Set("Authorization", "Bearer correct-management-token")
		ctx, cancel := context.WithCancel(request.Context())
		response := &cancelingLogRecorder{ResponseRecorder: httptest.NewRecorder(), cancel: cancel}
		handler.ServeHTTP(response, request.WithContext(ctx))
		cancel()
		if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"reset":true`) || !strings.Contains(response.Body.String(), `INFO fresh output after clear\n`) {
			t.Fatalf("%s stale resume: %d %s", endpoint, response.Code, response.Body.String())
		}
	}
}

type coreLogFlushRecorder struct {
	*httptest.ResponseRecorder
	onFlush func()
}

func (r *coreLogFlushRecorder) Flush() {
	r.ResponseRecorder.Flush()
	r.onFlush()
}

func TestClearCoreLogSurfacesStorageFailure(t *testing.T) {
	handler, _ := newCoreHTTPFixture(t)
	// A regular file blocks capture-directory creation; no real logs are used.
	if err := os.WriteFile(filepath.Join(handler.settings.DataDir, "logs"), []byte("blocked"), 0600); err != nil {
		t.Fatal(err)
	}
	response := authenticatedRequest(handler, http.MethodDelete, "/api/v1/core/logs/content?file=2026-09-23-000.log", "", "")
	if response.Code != http.StatusServiceUnavailable || !strings.Contains(response.Body.String(), "core_log_clear_failed") {
		t.Fatal(response.Code, response.Body.String())
	}
}
