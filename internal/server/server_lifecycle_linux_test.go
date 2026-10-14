//go:build linux

// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/rehuony/sing-box-panel/internal/buildinfo"
	"github.com/rehuony/sing-box-panel/internal/catalog"
	"github.com/rehuony/sing-box-panel/internal/console"
	"github.com/rehuony/sing-box-panel/internal/settings"
	"github.com/rehuony/sing-box-panel/internal/store"
	"github.com/rehuony/sing-box-panel/internal/testutil"
)

func TestServerStopsWithOpenMetricsStream(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := listener.Addr().String()
	port := listener.Addr().(*net.TCPAddr).Port
	listener.Close()
	value := settings.Defaults()
	value.Server.Host, value.Server.Port = "127.0.0.1", port
	value.DataDir = filepath.Join(t.TempDir(), "data")
	value.Auth.Email = testutil.AdminEmail
	value.Auth.PasswordHash = testutil.PasswordHash
	settingsPath := filepath.Join(t.TempDir(), "setting.json")
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if err := settings.Replace(settingsPath, raw); err != nil {
		t.Fatal(err)
	}
	// Keep this stream shutdown check independent of live GitHub responses and
	// catalog decoding that can outlast cancellation under the race detector.
	if err := prepareDataDirectory(value.DataDir); err != nil {
		t.Fatal(err)
	}
	database, err := store.Open(t.Context(), filepath.Join(value.DataDir, "panel.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	catalogJSON, err := json.Marshal(catalog.Catalog{RepositoryID: catalog.OfficialRepositoryID})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.SaveCatalogState(t.Context(), store.CatalogState{
		Catalog: catalogJSON, RefreshedAt: time.Now(),
	}); err != nil {
		t.Fatal(err)
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(console.WithOutput(t.Context(), io.Discard, false))
	done := make(chan struct{})
	var runErr error
	go func() {
		defer close(done)
		runErr = Run(ctx, settingsPath, buildinfo.Info{}, nil)
	}()
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
			if runErr != nil {
				t.Errorf("stop with connected browser: %v", runErr)
			}
		case <-time.After(shutdownTimeout + 5*time.Second):
			t.Error("server cleanup did not finish after stream shutdown")
		}
	})
	client := &http.Client{Timeout: 5 * time.Second}
	var response *http.Response
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		request, _ := http.NewRequestWithContext(t.Context(), http.MethodGet, "http://"+address+"/api/v1/health", nil)
		response, err = client.Do(request)
		if err == nil {
			break
		}
		select {
		case <-done:
			t.Fatalf("server exited before ready: %v", runErr)
		case <-time.After(10 * time.Millisecond):
		}
	}
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("health response: %d", response.StatusCode)
	}
	client, _ = loginPanel(t, t.Context(), address)
	request, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "http://"+address+"/api/v1/metrics/stream", nil)
	if err != nil {
		t.Fatal(err)
	}
	response, err = client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK || !strings.HasPrefix(response.Header.Get("Content-Type"), "text/event-stream") {
		t.Fatalf("stream response: %d %s", response.StatusCode, response.Header.Get("Content-Type"))
	}
	streamDone := make(chan error, 1)
	go func() {
		_, err := io.Copy(io.Discard, response.Body)
		streamDone <- err
	}()
	cancel()
	select {
	case err := <-streamDone:
		// A client timeout or disconnect must not count as a graceful server EOF.
		if err != nil {
			t.Fatalf("read metrics stream through server shutdown: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("metrics stream did not close after server cancellation")
	}
}
