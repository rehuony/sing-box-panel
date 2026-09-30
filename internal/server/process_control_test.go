// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux

package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/cookiejar"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/rehuony/sing-box-panel/internal/application"
	"github.com/rehuony/sing-box-panel/internal/buildinfo"
	"github.com/rehuony/sing-box-panel/internal/panelprocess"
	"github.com/rehuony/sing-box-panel/internal/settings"
	"github.com/rehuony/sing-box-panel/internal/store"
	"github.com/rehuony/sing-box-panel/internal/testutil"
)

func TestForegroundPanelCanBeStoppedFromAnotherClient(t *testing.T) {
	t.Setenv("INVOCATION_ID", "")
	value, path := processSettings(t)
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := prepareDataDirectory(value.DataDir); err != nil {
		t.Fatal(err)
	}
	database, err := store.Open(t.Context(), filepath.Join(value.DataDir, "panel.db"))
	if err != nil {
		t.Fatal(err)
	}
	_, appendErr := database.AppendLogEntry(t.Context(), store.LogEntry{
		ID: "historical_event", Time: time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC),
		Source: store.LogSourcePanel, Level: store.LogLevelInfo, Code: "test.history", Message: "Historical event",
		Metadata: json.RawMessage(`{}`),
	})
	closeErr := database.Close()
	if appendErr != nil || closeErr != nil {
		t.Fatalf("seed historical event: %v, %v", appendErr, closeErr)
	}
	var initialConfiguration application.ConfigurationFile
	for start := range 2 {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		result := make(chan error, 1)
		go func() { result <- Run(ctx, path, buildinfo.Info{Version: "test"}, fstest.MapFS{}) }()
		wait, stop := context.WithTimeout(context.Background(), 10*time.Second)
		defer stop()
		var status panelprocess.Status
		for {
			status, err = panelprocess.Inspect(wait, value.DataDir)
			if err == nil && status.State == "ready" {
				break
			}
			select {
			case err := <-result:
				cancel()
				stop()
				t.Fatalf("startup exited: %v", err)
			case <-wait.Done():
				cancel()
				stop()
				t.Fatalf("panel never ready: %v", err)
			case <-time.After(10 * time.Millisecond):
			}
		}
		if status.SettingsPath != path || status.DataDir != value.DataDir || status.PID != os.Getpid() {
			t.Fatalf("live status=%+v", status)
		}
		client, csrf := loginPanel(t, wait, status.Listen)
		request, err := http.NewRequestWithContext(wait, http.MethodGet, "http://"+status.Listen+"/api/v1/config/file", nil)
		if err != nil {
			t.Fatal(err)
		}
		response, err := client.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		var file application.ConfigurationFile
		decodeErr := json.NewDecoder(response.Body).Decode(&file)
		response.Body.Close()
		if response.StatusCode != http.StatusOK || decodeErr != nil || file.Content != "{}" || file.Revision != 1 ||
			!file.SyntaxValid || file.CanonicalRevisionID == "" || file.UpdatedAt == nil {
			cancel()
			stop()
			<-result
			t.Fatalf("startup configuration: status=%d file=%+v err=%v", response.StatusCode, file, decodeErr)
		}
		if start == 0 {
			initialConfiguration = file
		} else if file.CanonicalRevisionID != initialConfiguration.CanonicalRevisionID || !file.UpdatedAt.Equal(*initialConfiguration.UpdatedAt) {
			t.Fatal("restart rewrote the initialized configuration")
		}
		if err := Run(wait, path, buildinfo.Info{}, fstest.MapFS{}); err == nil || !strings.Contains(err.Error(), "owns this data directory") {
			t.Fatalf("duplicate start error=%v", err)
		}
		_, port, err := net.SplitHostPort(status.Listen)
		if err != nil || port != fmt.Sprint(value.Server.Port) {
			t.Fatalf("listener did not use selected settings after manual start: %s", status.Listen)
		}
		if start == 0 {
			// The Web endpoint persists the new port without moving the listener.
			var view application.PanelSettingsView
			requestPanelSettings(t, wait, client, status.Listen, csrf, nil, &view)
			reserved, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			value.Server.Port = reserved.Addr().(*net.TCPAddr).Port
			reserved.Close()
			view.Preferences.ListenPort = value.Server.Port
			input := application.PanelSettingsWrite{Revision: view.Revision, Preferences: view.Preferences}
			var saved application.PanelSettingsSaveResult
			requestPanelSettings(t, wait, client, status.Listen, csrf, &input, &saved)
			if !saved.Settings.RestartRequired {
				t.Fatal("listener change did not request manual restart")
			}
			requestPanelSettings(t, wait, client, status.Listen, csrf, nil, &view)
			current, err := panelprocess.Inspect(wait, value.DataDir)
			if err != nil || current.Listen != status.Listen || current.State != "ready" {
				t.Fatalf("Web save changed running listener: %+v %v", current, err)
			}
		}
		status, err = panelprocess.Stop(wait, value.DataDir)
		if err != nil || status.State != "stopped" {
			cancel()
			stop()
			t.Fatalf("stop=%+v err=%v", status, err)
		}
		lease, err := panelprocess.AcquireLease(value.DataDir)
		if err != nil {
			t.Fatalf("stop acknowledged before lease release: %v", err)
		}
		_ = lease.Close()
		if err := <-result; err != nil {
			t.Fatalf("foreground exit=%v", err)
		}
		cancel()
		stop()
		database, err := store.Open(t.Context(), filepath.Join(value.DataDir, "panel.db"))
		if err != nil {
			t.Fatal(err)
		}
		_, historyErr := database.GetLogEntry(t.Context(), "historical_event")
		closeErr := database.Close()
		if historyErr != nil || closeErr != nil {
			t.Fatalf("panel start or restart removed historical event: %v, %v", historyErr, closeErr)
		}
	}
}

func processSettings(t *testing.T) (settings.Settings, string) {
	t.Helper()
	dir, err := os.MkdirTemp("/tmp", "sbp-server-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	value := settings.Defaults()
	value.DataDir = filepath.Join(dir, "data")
	value.Auth.Email = testutil.AdminEmail
	value.Auth.PasswordHash = testutil.PasswordHash
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	value.Server.Port = listener.Addr().(*net.TCPAddr).Port
	_ = listener.Close()
	stored := value
	stored.DataDir = "data"
	data, err := json.Marshal(stored)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "setting.json")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return value, path
}

func TestCanceledStartupReleasesControlAndLease(t *testing.T) {
	value, path := processSettings(t)
	if err := prepareDataDirectory(value.DataDir); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := Run(ctx, path, buildinfo.Info{}, fstest.MapFS{}); err != nil {
		t.Fatalf("canceled startup: %v", err)
	}
	status, err := panelprocess.Inspect(context.Background(), value.DataDir)
	if err != nil || status.State != "stopped" {
		t.Fatalf("status=%+v err=%v", status, err)
	}
	lease, err := panelprocess.AcquireLease(value.DataDir)
	if err != nil {
		t.Fatal(err)
	}
	_ = lease.Close()
}

func loginPanel(t *testing.T, ctx context.Context, address string) (*http.Client, string) {
	t.Helper()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Jar: jar, Timeout: 5 * time.Second}
	body, err := json.Marshal(application.LoginInput{Email: testutil.AdminEmail, Password: testutil.AdminPassword})
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
	var session application.AuthSession
	if err := json.NewDecoder(response.Body).Decode(&session); err != nil {
		t.Fatal(err)
	}
	if session.CSRFToken == "" {
		t.Fatal("login response is missing the CSRF token")
	}
	return client, session.CSRFToken
}

func requestPanelSettings(t *testing.T, ctx context.Context, client *http.Client, address, csrf string, input *application.PanelSettingsWrite, output any) {
	t.Helper()
	method := http.MethodGet
	var body []byte
	if input != nil {
		method = http.MethodPut
		var err error
		body, err = json.Marshal(input)
		if err != nil {
			t.Fatal(err)
		}
	}
	request, err := http.NewRequestWithContext(ctx, method, "http://"+address+"/api/v1/panel/settings", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Origin", "http://"+address)
	request.Header.Set("X-CSRF-Token", csrf)
	request.Header.Set("Content-Type", "application/json")
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("settings response: %d", response.StatusCode)
	}
	if err := json.NewDecoder(response.Body).Decode(output); err != nil {
		t.Fatal(err)
	}
}
