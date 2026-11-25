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
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/rehuony/sing-box-panel/internal/buildinfo"
	"github.com/rehuony/sing-box-panel/internal/cliui"
	"github.com/rehuony/sing-box-panel/internal/procctl"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/auth"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/corecache"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/settings"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/store"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/testutil"
)

func TestServerStopsWithOpenLogStream(t *testing.T) {
	address, settingsPath, _ := prepareTestPanel(t)
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
	waitForPanel(t, address, done)
	client, _ := loginPanel(t, t.Context(), address)
	request, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "http://"+address+"/api/v1/logs/stream", nil)
	if err != nil {
		t.Fatal(err)
	}
	response, err := client.Do(request)
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

// A restart request is answered, then the server stops the way a stop does,
// and only then is the panel's own program run again, with the arguments and
// environment it started with.
func TestRestartStopsTheServerBeforeRunningThePanelAgain(t *testing.T) {
	address, settingsPath, dataDir := prepareTestPanel(t)
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	type call struct {
		path      string
		args, env []string
		leaseErr  error
	}
	calls := make(chan call, 1)
	exec := func(path string, args, env []string) error {
		// The stopped server has released its data directory.
		lease, err := procctl.AcquireLease(dataDir)
		if err == nil {
			err = lease.Close()
		}
		calls <- call{path, args, env, err}
		return nil
	}
	ctx := cliui.WithConsole(t.Context(), io.Discard, false)
	done := make(chan struct{})
	var runErr error
	go func() {
		defer close(done)
		runErr = run(ctx, settingsPath, buildinfo.Info{}, nil, exec)
	}()
	waitForPanel(t, address, done)
	client, csrf := loginPanel(t, t.Context(), address)
	var status struct {
		StartedAt time.Time `json:"started_at"`
	}
	response, err := client.Get("http://" + address + "/api/v1/system/status")
	if err == nil {
		err = json.NewDecoder(response.Body).Decode(&status)
		response.Body.Close()
	}
	if err != nil || status.StartedAt.IsZero() {
		t.Fatalf("status: %+v, %v", status, err)
	}
	request, _ := http.NewRequestWithContext(t.Context(), http.MethodPost, "http://"+address+"/api/v1/system/restart", nil)
	request.Header.Set("Origin", "http://"+address)
	request.Header.Set("X-CSRF-Token", csrf)
	response, err = client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	var restarted struct {
		StartedAt time.Time `json:"started_at"`
	}
	err = json.NewDecoder(response.Body).Decode(&restarted)
	response.Body.Close()
	if response.StatusCode != http.StatusAccepted || err != nil || !restarted.StartedAt.Equal(status.StartedAt) {
		t.Fatalf("restart response: %d %+v, %v", response.StatusCode, restarted, err)
	}
	select {
	case <-done:
	case <-time.After(shutdownTimeout + 5*time.Second):
		t.Fatal("the server did not stop after the restart request")
	}
	if runErr != nil {
		t.Fatalf("restart: %v", runErr)
	}
	select {
	case got := <-calls:
		if got.path != executable || !slices.Equal(got.args, os.Args) || !slices.Equal(got.env, os.Environ()) || got.leaseErr != nil {
			t.Fatalf("ran %s %v (lease: %v), want %s %v with the same environment", got.path, got.args, got.leaseErr, executable, os.Args)
		}
	default:
		t.Fatal("the panel was not run again")
	}
}

// waitForPanel waits until the panel at address answers its health check.
func waitForPanel(t *testing.T, address string, done <-chan struct{}) {
	t.Helper()
	client := &http.Client{Timeout: 5 * time.Second}
	deadline := time.Now().Add(5 * time.Second)
	for {
		request, _ := http.NewRequestWithContext(t.Context(), http.MethodGet, "http://"+address+"/api/v1/health", nil)
		response, err := client.Do(request)
		if err == nil {
			response.Body.Close()
			if response.StatusCode != http.StatusOK {
				t.Fatalf("health response: %d", response.StatusCode)
			}
			return
		}
		if time.Now().After(deadline) {
			t.Fatal(err)
		}
		select {
		case <-done:
			t.Fatal("server exited before ready")
		case <-time.After(10 * time.Millisecond):
		}
	}
}

// prepareTestPanel writes the settings of a panel on a free loopback port
// with a cached catalog, so starting it reads nothing from GitHub, and
// returns its address, settings path and data directory.
func prepareTestPanel(t *testing.T) (string, string, string) {
	t.Helper()
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
	// Keep startup independent of live GitHub responses and catalog decoding
	// that can outlast cancellation under the race detector.
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
	return address, settingsPath, value.DataDir
}

// loginPanel signs in as the fixture administrator and returns a client that
// carries the session cookie, and the session's CSRF token.
func loginPanel(t *testing.T, ctx context.Context, address string) (*http.Client, string) {
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
	var session struct {
		CSRFToken string `json:"csrfToken"`
	}
	if err := json.NewDecoder(response.Body).Decode(&session); err != nil || session.CSRFToken == "" {
		t.Fatalf("login session: %+v, %v", session, err)
	}
	return client, session.CSRFToken
}
