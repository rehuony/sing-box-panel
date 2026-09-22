// SPDX-License-Identifier: GPL-3.0-or-later

package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/rehuony/sing-box-panel/internal/application"
)

func TestDashboardStreamWritesImmediateAuthenticatedSnapshot(t *testing.T) {
	handler, _ := newCoreHTTPFixture(t)
	if _, err := handler.commands.DashboardSnapshot(t.Context()); err != nil {
		t.Fatalf("build dashboard snapshot: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	response := newFlushRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/dashboard/stream", nil).WithContext(ctx)
	request.Header.Set("Authorization", "Bearer correct-management-token")

	done := make(chan struct{})
	go func() {
		defer close(done)
		handler.ServeHTTP(response, request)
	}()
	select {
	case <-response.flushed:
		cancel()
	case <-time.After(2 * time.Second):
		cancel()
		t.Fatal("dashboard SSE did not flush its initial snapshot")
	}
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("dashboard SSE did not stop after request cancellation")
	}
	if response.Code != http.StatusOK || response.Header().Get("Content-Type") != "text/event-stream; charset=utf-8" {
		t.Fatalf("dashboard stream status=%d headers=%v", response.Code, response.Header())
	}
	if !strings.HasPrefix(response.Body.String(), "event: dashboard\ndata: ") {
		t.Fatalf("dashboard stream body=%q", response.Body.String())
	}
	raw := strings.TrimSuffix(strings.TrimPrefix(response.Body.String(), "event: dashboard\ndata: "), "\n\n")
	var snapshot application.DashboardStreamSnapshot
	if err := json.Unmarshal([]byte(raw), &snapshot); err != nil {
		t.Fatal(err)
	}
	if snapshot.History1H.BucketSeconds != 60 || snapshot.History24H.BucketSeconds != 300 {
		t.Fatalf("dashboard snapshot=%+v", snapshot)
	}
}

func TestDashboardStreamUsesThirtySecondUpdatesAndExpiresForReauthentication(t *testing.T) {
	if dashboardStreamInterval != 30*time.Second || dashboardStreamLifetime != time.Minute {
		t.Fatalf("dashboard schedule interval=%s lifetime=%s", dashboardStreamInterval, dashboardStreamLifetime)
	}
	handler, _ := newCoreHTTPFixture(t)
	unauthenticated := httptest.NewRecorder()
	handler.ServeHTTP(unauthenticated, httptest.NewRequest(http.MethodGet, "/api/v1/dashboard/stream", nil))
	if unauthenticated.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated dashboard stream status=%d", unauthenticated.Code)
	}

	response := newFlushRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/dashboard/stream", nil)
	handler.streamDashboardWithSchedule(response, request, 5*time.Millisecond, 24*time.Millisecond)
	if events := strings.Count(response.Body.String(), "event: dashboard\n"); events < 3 {
		t.Fatalf("dashboard stream emitted %d events before scheduled expiry: %q", events, response.Body.String())
	}
}
