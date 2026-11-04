// SPDX-License-Identifier: GPL-3.0-or-later

package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"testing"
	"time"

	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/store"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/telemetry"
)

func TestRuntimeHistoryHTTPUsesStablePairedCursor(t *testing.T) {
	handler, database := newCoreHTTPFixture(t)
	initialized, err := database.LatestRuntimeTransition(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	now := initialized.OccurredAt.Add(time.Minute)
	for index, reason := range []string{"startup_reconciled_stopped", "stop_succeeded"} {
		if _, err := database.AppendRuntimeTransition(context.Background(), store.RuntimeTransitionInput{
			DedupeKey:  "runtime-http-" + strconv.Itoa(index),
			State:      store.RuntimeTransitionStopped,
			Reason:     reason,
			OccurredAt: now.Add(time.Duration(index) * time.Second),
		}); err != nil {
			t.Fatal(err)
		}
	}
	query := url.Values{
		"state": []string{string(store.RuntimeTransitionStopped)},
		"limit": []string{"1"},
	}
	response := authenticatedRequest(t,
		handler,
		http.MethodGet,
		"/api/v1/core/runtime/history?"+query.Encode(),
		"",
		"",
	)
	if response.Code != http.StatusOK {
		t.Fatalf("runtime history status=%d body=%s", response.Code, response.Body.String())
	}
	var page telemetry.RuntimeHistoryPage
	if err := json.Unmarshal(response.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 || page.Items[0].Reason != "stop_succeeded" ||
		page.Next == nil || page.HistoryStartedAt.IsZero() {
		t.Fatalf("runtime history page = %+v", page)
	}
	nextQuery := url.Values{
		"state":       []string{string(store.RuntimeTransitionStopped)},
		"before_time": []string{page.Next.OccurredAt.Format(time.RFC3339Nano)},
		"before_id":   []string{strconv.FormatInt(page.Next.ID, 10)},
		"limit":       []string{"1"},
	}
	nextResponse := authenticatedRequest(t,
		handler,
		http.MethodGet,
		"/api/v1/core/runtime/history?"+nextQuery.Encode(),
		"",
		"",
	)
	if nextResponse.Code != http.StatusOK {
		t.Fatalf("next runtime history status=%d body=%s", nextResponse.Code, nextResponse.Body.String())
	}
	var next telemetry.RuntimeHistoryPage
	if err := json.Unmarshal(nextResponse.Body.Bytes(), &next); err != nil {
		t.Fatal(err)
	}
	if len(next.Items) != 1 || next.Items[0].Reason != "startup_reconciled_stopped" || next.Next != nil {
		t.Fatalf("next runtime history page = %+v", next)
	}

	for _, target := range []string{
		"/api/v1/core/runtime/history?before_time=" + url.QueryEscape(now.Format(time.RFC3339Nano)),
		"/api/v1/core/runtime/history?state=starting",
	} {
		invalid := authenticatedRequest(t, handler, http.MethodGet, target, "", "")
		if invalid.Code != http.StatusBadRequest {
			t.Fatalf("invalid runtime history %q status=%d body=%s", target, invalid.Code, invalid.Body.String())
		}
	}
}
