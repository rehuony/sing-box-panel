// SPDX-License-Identifier: GPL-3.0-or-later

package telemetry

import (
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/store"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/testutil"
)

func TestDashboardRuntimeHistoryKeepsCursorAtTheRecordLimit(t *testing.T) {
	database, err := store.Open(t.Context(), filepath.Join(t.TempDir(), "panel.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	app := New(database, nil, Options{})
	nodeID := testutil.CreateNode(t, database, "edge")
	from := time.Date(2025, time.January, 1, 0, 0, 0, 0, time.UTC)
	for index := range 4200 {
		err := database.RecordRuntimeTransition(t.Context(), store.RuntimeTransitionInput{
			NodeID: nodeID, InstanceID: "instance", SourceID: int64(index + 1),
			State: store.RuntimeTransitionStopped, Reason: "controlled_stop",
			OccurredAt: from.Add(time.Duration(index) * time.Second),
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	for _, count := range []int{4095, 4096, 4097, 4100, 4200} {
		t.Run(fmt.Sprint(count), func(t *testing.T) {
			to := from.Add(time.Duration(count) * time.Second)
			page, err := app.dashboardRuntimeHistory(t.Context(), nodeID, from, to)
			if err != nil {
				t.Fatal(err)
			}
			if len(page.Items) != min(count, dashboardRuntimeHistoryLimit) {
				t.Fatalf("history items=%d for %d transitions", len(page.Items), count)
			}
			if (page.Next != nil) != (count > dashboardRuntimeHistoryLimit) {
				t.Fatalf("history cursor=%+v for %d transitions", page.Next, count)
			}
			if page.Next == nil {
				return
			}
			last := page.Items[len(page.Items)-1]
			if page.Next.ID != last.ID || !page.Next.OccurredAt.Equal(last.OccurredAt) {
				t.Fatalf("cursor=%+v does not match last retained transition=%+v", page.Next, last)
			}
			remainder, err := app.RuntimeHistory(t.Context(), RuntimeHistoryRequest{
				NodeID: nodeID, From: &from, To: &to, Limit: 200,
				Cursor: &store.RuntimeTransitionCursor{OccurredAt: page.Next.OccurredAt, ID: page.Next.ID},
			})
			if err != nil {
				t.Fatal(err)
			}
			if len(remainder.Items) != count-dashboardRuntimeHistoryLimit || remainder.Next != nil {
				t.Fatalf("cursor skipped history: remaining=%d next=%+v", len(remainder.Items), remainder.Next)
			}
		})
	}
}

func TestDashboardSnapshotUsesStableRangesWithoutInventingEvidence(t *testing.T) {
	database, err := store.Open(t.Context(), filepath.Join(t.TempDir(), "panel.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	app := New(database, nil, Options{})
	nodeID := testutil.CreateNode(t, database, "edge")
	initialized := time.Now().UTC().Add(-time.Minute)
	if err := database.RecordRuntimeTransition(t.Context(), store.RuntimeTransitionInput{
		NodeID: nodeID, InstanceID: "instance", SourceID: 1, State: store.RuntimeTransitionUnknown,
		Reason: "history_initialized", OccurredAt: initialized, UncertainSince: &initialized,
	}); err != nil {
		t.Fatal(err)
	}
	now := initialized.Add(time.Minute)
	app.now = func() time.Time { return now }

	snapshot, err := app.DashboardSnapshot(t.Context(), nodeID)
	if err != nil {
		t.Fatal(err)
	}
	if !snapshot.CollectedAt.Equal(now) || snapshot.History1H.BucketSeconds != 60 || snapshot.History24H.BucketSeconds != 300 {
		t.Fatalf("dashboard snapshot=%+v", snapshot)
	}
	if !snapshot.History1H.From.Equal(now.Add(-time.Hour)) || !snapshot.History24H.From.Equal(now.Add(-24*time.Hour)) {
		t.Fatalf("dashboard ranges 1h=%s 24h=%s", snapshot.History1H.From, snapshot.History24H.From)
	}
	for _, bucket := range append(snapshot.History1H.Buckets, snapshot.History24H.Buckets...) {
		if bucket.UploadBytes != nil || bucket.DownloadBytes != nil || bucket.ActiveConnectionsAverage != nil || bucket.Coverage != store.CoverageMissing {
			t.Fatalf("empty dashboard invented metric evidence: %+v", bucket)
		}
	}
	if len(snapshot.Runtime24H.Items) != 1 || len(snapshot.Activity.Items) != 0 {
		t.Fatalf("dashboard did not include initialized runtime evidence: %+v", snapshot)
	}
}
