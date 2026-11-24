//go:build linux

// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/cookiejar"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/rehuony/sing-box-panel/internal/buildinfo"
	"github.com/rehuony/sing-box-panel/internal/cliui"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/auth"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/corecache"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/settings"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/store"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/testutil"
)

func TestServerStopsWithOpenLogStream(t *testing.T) {
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
	catalogJSON, err := json.Marshal(corecache.Catalog{RepositoryID: corecache.OfficialRepositoryID})
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
	ctx, cancel := context.WithCancel(cliui.WithConsole(t.Context(), io.Discard, false))
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
	client = loginPanel(t, t.Context(), address)
	request, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "http://"+address+"/api/v1/logs/stream", nil)
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
			t.Fatalf("read log stream through server shutdown: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("log stream did not close after server cancellation")
	}
}

// loginPanel signs in as the fixture administrator and returns a client that
// carries the session cookie.
func loginPanel(t *testing.T, ctx context.Context, address string) *http.Client {
	t.Helper()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Jar: jar, Timeout: 5 * time.Second}
	body, err := json.Marshal(auth.LoginInput{Email: testutil.AdminEmail, Password: testutil.AdminPassword})
	if err != nil {
		t.Fatal(err)
	}
	origin := "http://" + address
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, origin+"/api/v1/auth/session", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Origin", origin)
	request.Header.Set("Content-Type", "application/json")
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("login response: %d", response.StatusCode)
	}
	return client
}
