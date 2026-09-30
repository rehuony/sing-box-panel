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
	"github.com/rehuony/sing-box-panel/internal/console"
	"github.com/rehuony/sing-box-panel/internal/settings"
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
	ctx, cancel := context.WithCancel(console.WithOutput(t.Context(), io.Discard, false))
	defer cancel()
	result := make(chan error, 1)
	go func() { result <- Run(ctx, settingsPath, buildinfo.Info{}, nil) }()
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
		case err := <-result:
			t.Fatalf("server exited before ready: %v", err)
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
	cancel()
	select {
	case err := <-result:
		if err != nil {
			t.Fatalf("stop with connected browser: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("connected metrics stream prevented server shutdown")
	}
}
