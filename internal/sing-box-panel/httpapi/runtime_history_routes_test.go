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
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/testutil"
)

func TestRuntimeHistoryHTTPUsesStablePairedCursor(t *testing.T) {
	handler, database := newCoreHTTPFixture(t)
	nodeID := testutil.CreateNode(t, database, "edge")
	historyPath := "/api/v1/nodes/" + nodeID + "/core/runtime/history"
	now := time.Now().UTC().Add(-time.Minute)
	for index, reason := range []string{"startup_reconciled_stopped", "stop_succeeded"} {
		if err := database.RecordRuntimeTransition(context.Background(), store.RuntimeTransitionInput{
			NodeID: nodeID, InstanceID: "instance", SourceID: int64(index + 1),
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
		historyPath+"?"+query.Encode(),
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
		historyPath+"?"+nextQuery.Encode(),
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
		historyPath + "?before_time=" + url.QueryEscape(now.Format(time.RFC3339Nano)),
		historyPath + "?state=starting",
	} {
		invalid := authenticatedRequest(t, handler, http.MethodGet, target, "", "")
		if invalid.Code != http.StatusBadRequest {
			t.Fatalf("invalid runtime history %q status=%d body=%s", target, invalid.Code, invalid.Body.String())
		}
	}

	// Clearing keeps the newest entry, the node's current state.
	cleared := authenticatedRequest(t, handler, http.MethodDelete, historyPath, "", "")
	if cleared.Code != http.StatusOK || cleared.Body.String() != `{"deleted":1}`+"\n" {
		t.Fatalf("clear runtime history status=%d body=%s", cleared.Code, cleared.Body.String())
	}
	filtered := authenticatedRequest(t, handler, http.MethodDelete, historyPath+"?state=stopped", "", "")
	if filtered.Code != http.StatusBadRequest {
		t.Fatalf("filtered runtime history clear status=%d body=%s", filtered.Code, filtered.Body.String())
	}
}
