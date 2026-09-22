// SPDX-License-Identifier: GPL-3.0-or-later

package httpapi

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/rehuony/sing-box-panel/internal/application"
)

func TestDashboardStreamClosesTransportCleanly(t *testing.T) {
	for _, http2 := range []bool{false, true} {
		t.Run(fmt.Sprint(http2), func(t *testing.T) {
			cancelStream := make(chan context.CancelFunc, 1)
			server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
				ctx, cancel := context.WithCancel(request.Context())
				defer cancel()
				cancelStream <- cancel
				streamDashboardSnapshots(w, request.WithContext(ctx), func(context.Context) (application.DashboardStreamSnapshot, error) {
					return application.DashboardStreamSnapshot{}, nil
				}, dashboardStreamInterval, dashboardStreamLifetime)
			}))
			server.EnableHTTP2 = http2
			server.StartTLS()
			t.Cleanup(server.Close)
			client := server.Client()
			client.Timeout = 5 * time.Second
			response, err := client.Get(server.URL)
			if err != nil {
				t.Fatal(err)
			}
			defer response.Body.Close()
			cancel := <-cancelStream
			defer cancel()
			reader := bufio.NewReader(response.Body)
			for {
				line, err := reader.ReadString('\n')
				if err != nil {
					t.Fatalf("read initial frame: %v", err)
				}
				if line == "\n" {
					break
				}
			}
			cancel()
			if _, err := io.ReadAll(reader); err != nil {
				t.Fatalf("dashboard response did not end with a clean EOF: %v", err)
			}
			wantProtocol := 1
			if http2 {
				wantProtocol = 2
			}
			if response.ProtoMajor != wantProtocol {
				t.Fatalf("HTTP protocol=%s", response.Proto)
			}
		})
	}
}

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

func TestDashboardStreamReportsInitialSnapshotFailure(t *testing.T) {
	handler, database := newCoreHTTPFixture(t)
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/dashboard/stream", nil)
	request.Header.Set("Authorization", "Bearer correct-management-token")
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusInternalServerError || !strings.HasPrefix(response.Header().Get("Content-Type"), "application/problem+json") {
		t.Fatalf("failed snapshot status=%d content-type=%q", response.Code, response.Header().Get("Content-Type"))
	}
	var problem Problem
	if err := json.Unmarshal(response.Body.Bytes(), &problem); err != nil {
		t.Fatal(err)
	}
	if problem.Code != "dashboard_snapshot_unavailable" {
		t.Fatalf("problem code=%q", problem.Code)
	}
}

func TestDashboardStreamUsesThirtySecondUpdatesAndExpiresForReauthentication(t *testing.T) {
	if dashboardStreamInterval != 30*time.Second || dashboardStreamLifetime != time.Minute {
		t.Fatalf("dashboard schedule interval=%s lifetime=%s", dashboardStreamInterval, dashboardStreamLifetime)
	}
	synctest.Test(t, func(t *testing.T) {
		start := time.Now()
		var collected []time.Duration
		load := func(ctx context.Context) (application.DashboardStreamSnapshot, error) {
			deadline, ok := ctx.Deadline()
			if !ok || !deadline.Equal(start.Add(dashboardStreamLifetime)) {
				t.Fatalf("snapshot deadline=%s present=%t", deadline, ok)
			}
			collected = append(collected, time.Since(start))
			return application.DashboardStreamSnapshot{}, nil
		}
		response := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodGet, "/api/v1/dashboard/stream", nil)
		streamDashboardSnapshots(response, request, load, dashboardStreamInterval, dashboardStreamLifetime)
		if len(collected) != 2 || collected[0] != 0 || collected[1] != 30*time.Second {
			t.Fatalf("collection times=%v; want immediate and 30 seconds", collected)
		}
		if time.Since(start) != time.Minute || strings.Count(response.Body.String(), "event: dashboard\n") != 2 {
			t.Fatalf("stream duration=%s events=%d", time.Since(start), strings.Count(response.Body.String(), "event: dashboard\n"))
		}
	})
}

func TestDashboardStreamRequiresAuthentication(t *testing.T) {
	handler, _ := newCoreHTTPFixture(t)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/dashboard/stream", nil))
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated dashboard stream status=%d", response.Code)
	}
}

func TestDashboardStreamLifetimeCancelsSnapshotCollection(t *testing.T) {
	for _, blockedCall := range []int{1, 2} {
		t.Run(fmt.Sprint(blockedCall), func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				start := time.Now()
				calls := 0
				load := func(ctx context.Context) (application.DashboardStreamSnapshot, error) {
					calls++
					if calls == blockedCall {
						<-ctx.Done()
						if !errors.Is(ctx.Err(), context.DeadlineExceeded) {
							t.Fatalf("collection context error=%v", ctx.Err())
						}
						// Even a loader returning a late success must not publish it.
					}
					return application.DashboardStreamSnapshot{}, nil
				}
				response := httptest.NewRecorder()
				request := httptest.NewRequest(http.MethodGet, "/api/v1/dashboard/stream", nil)
				streamDashboardSnapshots(response, request, load, dashboardStreamInterval, dashboardStreamLifetime)
				if time.Since(start) != time.Minute || calls != blockedCall {
					t.Fatalf("stream duration=%s collections=%d", time.Since(start), calls)
				}
				if events := strings.Count(response.Body.String(), "event: dashboard\n"); events != blockedCall-1 {
					t.Fatalf("stream published %d snapshots, want %d", events, blockedCall-1)
				}
				if blockedCall == 1 && response.Code != http.StatusInternalServerError {
					t.Fatalf("initial snapshot timeout status=%d", response.Code)
				}
			})
		})
	}
}

func TestDashboardStreamClearsWriteDeadlinesAndStopsOnFlushFailure(t *testing.T) {
	for _, failFlush := range []bool{false, true} {
		t.Run(fmt.Sprint(failFlush), func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				response := &dashboardDeadlineRecorder{ResponseRecorder: httptest.NewRecorder()}
				if failFlush {
					response.flushError = errors.New("disconnected")
				}
				start := time.Now()
				load := func(context.Context) (application.DashboardStreamSnapshot, error) {
					return application.DashboardStreamSnapshot{}, nil
				}
				request := httptest.NewRequest(http.MethodGet, "/api/v1/dashboard/stream", nil)
				streamDashboardSnapshots(response, request, load, 30*time.Second, 35*time.Second)
				wantWrites := 2
				if failFlush {
					wantWrites = 1
					if time.Since(start) != 0 {
						t.Fatalf("flush failure did not stop the stream immediately")
					}
				}
				if len(response.deadlines) != wantWrites*2 {
					t.Fatalf("write deadlines=%v", response.deadlines)
				}
				for index := range wantWrites {
					wantDeadline := start.Add(10 * time.Second)
					if index == 1 {
						wantDeadline = start.Add(35 * time.Second)
					}
					if !response.deadlines[index*2].Equal(wantDeadline) || !response.deadlines[index*2+1].IsZero() {
						t.Fatalf("write deadlines=%v", response.deadlines)
					}
				}
			})
		})
	}
}

type dashboardDeadlineRecorder struct {
	*httptest.ResponseRecorder
	deadlines  []time.Time
	flushError error
}

func (w *dashboardDeadlineRecorder) SetWriteDeadline(deadline time.Time) error {
	w.deadlines = append(w.deadlines, deadline)
	return nil
}

func (w *dashboardDeadlineRecorder) FlushError() error {
	w.ResponseRecorder.Flush()
	return w.flushError
}
