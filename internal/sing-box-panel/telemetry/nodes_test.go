// SPDX-License-Identifier: GPL-3.0-or-later

package telemetry

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/store"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/testutil"
)

// TestTrafficLedgerAnswersFromMemoryAndFollowsChanges reads recorded traffic
// from the ledger, and shows each change that must reach it: a stored
// sample, a changed period, a rollover, a read that started before a stored
// sample, and a forgotten node.
func TestTrafficLedgerAnswersFromMemoryAndFollowsChanges(t *testing.T) {
	ctx := t.Context()
	database, err := store.Open(ctx, filepath.Join(t.TempDir(), "panel.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	now := time.Date(2026, time.June, 10, 12, 0, 30, 0, time.UTC)
	records := New(database, nil, Options{Now: func() time.Time { return now }})
	nodeID := testutil.CreateNode(t, database, "edge")
	node, err := database.Node(ctx, nodeID)
	if err != nil {
		t.Fatal(err)
	}
	limited := &AppliedBundle{ID: "bundle", MonitoringTier: "limited"}
	sample := func(at time.Time, upload, download int64) NodeSample {
		return NodeSample{SampledAt: at, Traffic: &NodeTraffic{ActivationBundleID: "bundle", PID: 7, ProcessStartToken: "start",
			UploadTotal: upload, DownloadTotal: download}}
	}
	record := func(at time.Time, upload, download int64) {
		t.Helper()
		if err := records.RecordNodeSample(ctx, nodeID, "instance", sample(at, upload, download)); err != nil {
			t.Fatal(err)
		}
	}
	// storeBehindTelemetry stores a sample without telemetry, which only a
	// read of the store can see.
	storeBehindTelemetry := func(at time.Time, upload, download int64) {
		t.Helper()
		start, end, err := TrafficPeriod(at, node.Settings.Traffic)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := database.RecordTrafficSample(ctx, store.TrafficSampleInput{NodeID: nodeID, InstanceID: "instance",
			ActivationBundleID: "bundle", PID: 7, ProcessStartToken: "start", SampledAt: at, PeriodStart: start, PeriodEnd: end,
			UploadTotal: upload, DownloadTotal: download}); err != nil {
			t.Fatal(err)
		}
	}
	read := func(node store.Node, wantStart time.Time, wantInbound, wantOutbound int64) MetricsSnapshot {
		t.Helper()
		metrics, err := records.NodeMetrics(ctx, node, limited)
		period := metrics.CurrentTrafficData
		if err != nil || !period.PeriodStart.Equal(wantStart) || period.InboundBytes != wantInbound || period.OutboundBytes != wantOutbound {
			t.Fatalf("period = %s %d/%d, %v; want %s %d/%d", period.PeriodStart, period.InboundBytes, period.OutboundBytes, err,
				wantStart, wantInbound, wantOutbound)
		}
		return metrics
	}
	june, mayFifteenth := time.Date(2026, time.June, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, time.May, 15, 0, 0, 0, 0, time.UTC)

	// Ten bytes up and thirty down on May 20, then 300 up and 1000 down on June 10.
	record(time.Date(2026, time.May, 20, 12, 0, 0, 0, time.UTC), 100, 200)
	record(time.Date(2026, time.May, 20, 12, 0, 10, 0, time.UTC), 110, 230)
	record(time.Date(2026, time.June, 10, 12, 0, 0, 0, time.UTC), 410, 1230)
	if metrics := read(node, june, 1000, 300); metrics.LatestSample == nil || metrics.LatestSample.UploadTotal != 410 {
		t.Fatalf("latest sample = %+v; want the one stored last", metrics.LatestSample)
	}

	// The ledger answers: a sample stored behind telemetry stays unseen until
	// telemetry stores one, whose transaction reads both.
	storeBehindTelemetry(time.Date(2026, time.June, 10, 12, 0, 10, 0, time.UTC), 420, 1240)
	read(node, june, 1000, 300)
	record(time.Date(2026, time.June, 10, 12, 0, 20, 0, time.UTC), 430, 1250)
	if metrics := read(node, june, 1020, 320); metrics.LatestSample == nil || metrics.LatestSample.UploadTotal != 430 {
		t.Fatalf("latest sample = %+v; want the one stored last", metrics.LatestSample)
	}

	// A rejected sample changes no totals, though the store sums its period
	// only up to its own earlier day.
	if err := records.RecordNodeSample(ctx, nodeID, "instance", NodeSample{
		SampledAt: time.Date(2026, time.June, 9, 12, 0, 0, 0, time.UTC),
		Traffic:   &NodeTraffic{ActivationBundleID: "bundle", PID: 8, ProcessStartToken: "earlier", UploadTotal: 1, DownloadTotal: 1},
	}); err != nil {
		t.Fatal(err)
	}
	if metrics := read(node, june, 1020, 320); metrics.LatestSample == nil || metrics.LatestSample.UploadTotal != 430 {
		t.Fatalf("latest sample = %+v; want the one accepted last", metrics.LatestSample)
	}

	// A changed reset day is another range, read once.
	node.Settings.Traffic.ResetDay = 15
	read(node, mayFifteenth, 1050, 330)
	node.Settings.Traffic.ResetDay = 1
	read(node, june, 1020, 320)

	// A rollover starts an empty period.
	now = time.Date(2026, time.July, 1, 0, 0, 1, 0, time.UTC)
	read(node, time.Date(2026, time.July, 1, 0, 0, 0, 0, time.UTC), 0, 0)
	now = time.Date(2026, time.June, 10, 12, 0, 30, 0, time.UTC)

	// A read that began before a sample was stored keeps nothing it loaded.
	records.live.mu.Lock()
	live := records.live.nodes[nodeID]
	version := live.ledger.version
	records.live.mu.Unlock()
	record(time.Date(2026, time.June, 10, 12, 0, 30, 0, time.UTC), 440, 1260)
	older := store.TrafficPeriod{PeriodStart: june, PeriodEnd: time.Date(2026, time.July, 1, 0, 0, 0, 0, time.UTC)}
	records.live.keepLoaded(nodeID, live, version, func(ledger *trafficLedger) { ledger.period, ledger.latest = &older, nil })
	if metrics := read(node, june, 1030, 330); metrics.LatestSample == nil || metrics.LatestSample.UploadTotal != 440 {
		t.Fatalf("latest sample = %+v; want the one stored last", metrics.LatestSample)
	}

	// Forgetting a node drops its ledger, so the next read sees the store.
	storeBehindTelemetry(time.Date(2026, time.June, 10, 12, 0, 40, 0, time.UTC), 450, 1270)
	read(node, june, 1030, 330)
	records.ForgetNode(nodeID)
	if metrics := read(node, june, 1040, 340); metrics.LatestSample == nil || metrics.LatestSample.UploadTotal != 450 {
		t.Fatalf("latest sample = %+v; want the one stored last", metrics.LatestSample)
	}
}
