// SPDX-License-Identifier: GPL-3.0-or-later

package store

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"
	"time"
)

// A kept dashboard history answers what a full read answers while samples
// and transitions arrive, late ones reaching into closed buckets included.
func TestDashboardHistoryRecomputesWhatNewRowsReach(t *testing.T) {
	ctx := context.Background()
	database := openTestStore(t, ctx)
	start := time.Date(2026, time.August, 10, 11, 30, 30, 0, time.UTC)
	periodStart := time.Date(2026, time.August, 1, 0, 0, 0, 0, time.UTC)
	input := TrafficSampleInput{
		NodeID: testNodeID, InstanceID: "instance-a", ActivationBundleID: "bundle-dashboard", PID: 909, ProcessStartToken: "dashboard-process",
		PeriodStart: periodStart, PeriodEnd: periodStart.AddDate(0, 1, 0),
	}
	sample := func(at time.Time, total int64) {
		t.Helper()
		input.SampledAt, input.UploadTotal, input.DownloadTotal = at, total, total*2
		input.MemoryBytes, input.ActiveConnections = 1000+total, total%7
		if _, err := database.RecordTrafficSample(ctx, input); err != nil {
			t.Fatal(err)
		}
	}
	transition := func(id int64, state RuntimeTransitionState, at time.Time, uncertainSince *time.Time) {
		t.Helper()
		if err := database.RecordRuntimeTransition(ctx, RuntimeTransitionInput{
			NodeID: testNodeID, InstanceID: "instance-a", SourceID: id, State: state, Reason: "dashboard_test",
			OccurredAt: at, UncertainSince: uncertainSince,
		}); err != nil {
			t.Fatal(err)
		}
	}
	history := &DashboardHistory{}
	read := func(at time.Time, kept *DashboardHistory) []byte {
		t.Helper()
		hour, day, watermark, err := database.DashboardMetricsHistory(ctx, testNodeID, at, kept)
		if err != nil {
			t.Fatal(err)
		}
		encoded, err := json.Marshal([]any{hour, day, watermark})
		if err != nil {
			t.Fatal(err)
		}
		return encoded
	}
	agrees := func(at time.Time) bool {
		t.Helper()
		return bytes.Equal(read(at, history), read(at, nil))
	}

	for index := range 180 {
		sample(start.Add(time.Duration(index)*10*time.Second), int64(index*10))
	}
	now := start.Add(30 * time.Minute)
	if !agrees(now) {
		t.Fatal("first read differs from a full read")
	}
	// No sample for minutes: the buckets close as missing.
	if !agrees(now.Add(3 * time.Minute)) {
		t.Fatal("read without new samples differs from a full read")
	}
	// The next sample's interval reaches back over the closed buckets.
	sample(now.Add(5*time.Minute), 2000)
	if !agrees(now.Add(6 * time.Minute)) {
		t.Fatal("an interval reaching into closed buckets was not read again")
	}
	// An out-of-order sample lands in a bucket closed long ago.
	sample(now.Add(-10*time.Minute+5*time.Second), 1)
	transition(1, RuntimeTransitionStopped, now.Add(7*time.Minute), nil)
	if !agrees(now.Add(8 * time.Minute)) {
		t.Fatal("a late sample or a stop was not read again")
	}
	// A restarted agent cannot say what happened since before the stop.
	uncertain := now.Add(6*time.Minute + 30*time.Second)
	transition(2, RuntimeTransitionUnknown, now.Add(12*time.Minute), &uncertain)
	if !agrees(now.Add(12*time.Minute + time.Second)) {
		t.Fatal("uncertainty reaching back was not read again")
	}

	// A deletion leaves the kept buckets as they were until a reset.
	at := now.Add(13 * time.Minute)
	read(at, history)
	if _, err := database.DeleteTrafficSamplesBefore(ctx, start.Add(10*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if agrees(at) {
		t.Fatal("the kept history read deleted samples again; the reset below proves nothing")
	}
	history.Reset()
	if !agrees(at) {
		t.Fatal("a reset history differs from a full read")
	}
}
