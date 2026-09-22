// SPDX-License-Identifier: GPL-3.0-or-later

package httpapi

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/rehuony/sing-box-panel/internal/application"
	"github.com/rehuony/sing-box-panel/internal/store"
)

func TestDashboardSnapshotFitsFrameLimitAndPreservesOmittedHistory(t *testing.T) {
	handler, database := newCoreHTTPFixture(t)
	now := time.Now().UTC()
	started := now.Add(-25 * time.Hour)
	for index := range 4100 {
		at := now.Add(-24*time.Hour + time.Duration(index+1)*20*time.Second)
		_, err := database.AppendRuntimeTransition(t.Context(), store.RuntimeTransitionInput{
			DedupeKey: fmt.Sprintf("dashboard-size-%d", index),
			State:     store.RuntimeTransitionUnknown, Reason: "inspection_unavailable",
			Generation: int64(index + 1), PID: 12345, ProcessStartToken: "12345:67890",
			ProcessStartedAt: &started, OccurredAt: at, UncertainSince: &at,
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	snapshot, err := handler.commands.DashboardSnapshot(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	unbounded, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if len(unbounded)+dashboardFrameOverhead <= dashboardFrameLimit {
		t.Fatalf("fixture frame=%d bytes must exceed %d", len(unbounded)+dashboardFrameOverhead, dashboardFrameLimit)
	}
	data, err := encodeDashboardSnapshot(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if len(data)+dashboardFrameOverhead > dashboardFrameLimit {
		t.Fatalf("encoded frame=%d bytes exceeds %d", len(data)+dashboardFrameOverhead, dashboardFrameLimit)
	}
	var bounded application.DashboardStreamSnapshot
	if err := json.Unmarshal(data, &bounded); err != nil {
		t.Fatal(err)
	}
	count := len(bounded.Runtime24H.Items)
	if count < 1 || count >= len(snapshot.Runtime24H.Items) {
		t.Fatalf("retained %d of %d transitions", count, len(snapshot.Runtime24H.Items))
	}
	if !reflect.DeepEqual(bounded.Runtime24H.Items, snapshot.Runtime24H.Items[:count]) {
		t.Fatal("bounded snapshot did not preserve the newest transitions")
	}
	last := bounded.Runtime24H.Items[count-1]
	next := bounded.Runtime24H.Next
	if next == nil || next.ID != last.ID || !next.OccurredAt.Equal(last.OccurredAt) {
		t.Fatalf("bounded cursor=%+v does not identify last retained transition=%d", next, last.ID)
	}
	from, to := snapshot.History24H.From, snapshot.History24H.To
	page, err := handler.commands.RuntimeHistory(t.Context(), application.RuntimeHistoryRequest{
		From: &from, To: &to, Limit: 1,
		Cursor: &store.RuntimeTransitionCursor{OccurredAt: next.OccurredAt, ID: next.ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 || page.Items[0].ID != snapshot.Runtime24H.Items[count].ID {
		t.Fatal("bounded snapshot cursor skipped the first omitted transition")
	}
	// No other projection evidence, including the preceding state, is changed.
	bounded.Runtime24H.Items = snapshot.Runtime24H.Items
	bounded.Runtime24H.Next = snapshot.Runtime24H.Next
	if !reflect.DeepEqual(bounded, snapshot) {
		t.Fatal("frame bounding changed evidence outside the retained history prefix")
	}
	// Adding one more transition must exceed the budget: keep all that fits.
	bounded.Runtime24H.Items = snapshot.Runtime24H.Items[:count+1]
	last = bounded.Runtime24H.Items[count]
	bounded.Runtime24H.Next = &application.RuntimeTransitionCursor{OccurredAt: last.OccurredAt, ID: last.ID}
	data, err = json.Marshal(bounded)
	if err != nil {
		t.Fatal(err)
	}
	if len(data)+dashboardFrameOverhead <= dashboardFrameLimit {
		t.Fatal("frame bounding discarded a transition that still fits")
	}
}

func TestDashboardSnapshotEncodingPreservesSmallSnapshots(t *testing.T) {
	handler, _ := newCoreHTTPFixture(t)
	snapshot, err := handler.commands.DashboardSnapshot(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	want, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	got, err := encodeDashboardSnapshot(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(want) {
		t.Fatal("small snapshot was changed")
	}
}

func TestDashboardSnapshotEncodingRejectsUnshrinkableFrames(t *testing.T) {
	snapshot := application.DashboardStreamSnapshot{
		Activity: store.PanelLogPage{Items: []store.PanelLog{{Message: strings.Repeat("x", dashboardFrameLimit)}}},
	}
	if _, err := encodeDashboardSnapshot(snapshot); err == nil {
		t.Fatal("unshrinkable snapshot exceeded the frame limit without an error")
	}
	snapshot.Activity.Items[0].Message = ""
	snapshot.Activity.Items[0].Metadata = json.RawMessage(`not-json`)
	if _, err := encodeDashboardSnapshot(snapshot); err == nil {
		t.Fatal("invalid snapshot JSON did not produce an encoding error")
	}
}
