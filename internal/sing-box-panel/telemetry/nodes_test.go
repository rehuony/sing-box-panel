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

// TestRecentMetricsReturnsTheLastTwoMinutesAsRates reads the stored samples a
// live chart opens with: those of the last two minutes, each with the rates
// of the interval ending at it, and none for a sample a new process starts;
// then the span after the core stopped, which holds no samples.
func TestRecentMetricsReturnsTheLastTwoMinutesAsRates(t *testing.T) {
	ctx := t.Context()
	database, err := store.Open(ctx, filepath.Join(t.TempDir(), "panel.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	now := time.Date(2026, time.June, 10, 12, 2, 0, 0, time.UTC)
	records := New(database, nil, Options{Now: func() time.Time { return now }})
	nodeID := testutil.CreateNode(t, database, "edge")
	record := func(at time.Time, pid int, connections, upload, download int64) {
		t.Helper()
		if err := records.RecordNodeSample(ctx, nodeID, "instance", NodeSample{SampledAt: at, Traffic: &NodeTraffic{
			ActivationBundleID: "bundle", PID: pid, ProcessStartToken: "start", Connections: connections,
			UploadTotal: upload, DownloadTotal: download,
		}}); err != nil {
			t.Fatal(err)
		}
	}
	at := func(minute, second int) time.Time {
		return time.Date(2026, time.June, 10, 12, minute, second, 0, time.UTC)
	}
	record(at(-1, 50), 7, 1, 0, 0) // 11:59:50, before the window
	record(at(0, 10), 7, 2, 200, 400)
	record(at(0, 20), 7, 3, 300, 600)
	record(at(1, 0), 8, 4, 0, 0)
	if err := database.RecordRuntimeTransition(ctx, store.RuntimeTransitionInput{NodeID: nodeID, InstanceID: "instance",
		SourceID: 1, State: store.RuntimeTransitionStopped, Reason: "stop_succeeded", OccurredAt: at(1, 30)}); err != nil {
		t.Fatal(err)
	}

	recent, err := records.RecentMetrics(ctx, nodeID)
	if err != nil {
		t.Fatal(err)
	}
	if !recent.From.Equal(at(0, 0)) || !recent.To.Equal(now) {
		t.Fatalf("window = %s to %s; want %s to %s", recent.From, recent.To, at(0, 0), now)
	}
	value := func(pointer *float64) any {
		if pointer == nil {
			return nil
		}
		return *pointer
	}
	type point struct {
		at                        time.Time
		seconds, upload, download any
		connections               int64
	}
	got := make([]point, 0, len(recent.Samples))
	for _, sample := range recent.Samples {
		got = append(got, point{sample.SampledAt, value(sample.IntervalSeconds), value(sample.UploadBytesPerSecond),
			value(sample.DownloadBytesPerSecond), sample.ActiveConnections})
	}
	want := []point{
		{at(0, 10), 20.0, 10.0, 20.0, 2},
		{at(0, 20), 10.0, 10.0, 20.0, 3},
		{at(1, 0), nil, nil, nil, 4},
	}
	if len(recent.Idle) != 1 || !recent.Idle[0].From.Equal(at(1, 30)) || !recent.Idle[0].To.Equal(now) {
		t.Fatalf("idle = %+v; want one span from %s to %s", recent.Idle, at(1, 30), now)
	}
	if len(got) != len(want) {
		t.Fatalf("samples = %+v; want %+v", got, want)
	}
	for index := range want {
		if !got[index].at.Equal(want[index].at) || got[index].seconds != want[index].seconds || got[index].upload != want[index].upload ||
			got[index].download != want[index].download || got[index].connections != want[index].connections {
			t.Fatalf("sample %d = %+v; want %+v", index, got[index], want[index])
		}
	}
}

// TestLiveRatesSurviveRepeatedAndMissingReadings keeps a live rate when the
// node reports one reading twice, and derives the next rate across a sample
// that carried no traffic.
func TestLiveRatesSurviveRepeatedAndMissingReadings(t *testing.T) {
	ctx := t.Context()
	database, err := store.Open(ctx, filepath.Join(t.TempDir(), "panel.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	start := time.Date(2026, time.June, 10, 12, 0, 0, 0, time.UTC)
	now := start
	records := New(database, nil, Options{Now: func() time.Time { return now }})
	nodeID := testutil.CreateNode(t, database, "edge")
	node, err := database.Node(ctx, nodeID)
	if err != nil {
		t.Fatal(err)
	}
	record := func(seconds int, traffic *NodeTraffic) {
		t.Helper()
		now = start.Add(time.Duration(seconds) * time.Second)
		if err := records.RecordNodeSample(ctx, nodeID, "instance", NodeSample{SampledAt: now, Traffic: traffic}); err != nil {
			t.Fatal(err)
		}
	}
	reading := func(upload, download int64) *NodeTraffic {
		return &NodeTraffic{ActivationBundleID: "bundle", PID: 7, ProcessStartToken: "start", UploadTotal: upload, DownloadTotal: download}
	}
	rates := func() (any, any) {
		t.Helper()
		metrics, err := records.NodeMetrics(ctx, node, nil)
		if err != nil {
			t.Fatal(err)
		}
		live := metrics.LiveSample
		if live == nil {
			return nil, nil
		}
		if live.UploadBytesPerSecond == nil || live.DownloadBytesPerSecond == nil {
			return "none", "none"
		}
		return *live.UploadBytesPerSecond, *live.DownloadBytesPerSecond
	}

	record(0, reading(0, 0))
	record(1, reading(100, 200))
	now = start.Add(1500 * time.Millisecond)
	if err := records.RecordNodeSample(ctx, nodeID, "instance", NodeSample{SampledAt: start.Add(time.Second), Traffic: reading(100, 200)}); err != nil {
		t.Fatal(err)
	}
	if upload, download := rates(); upload != 100.0 || download != 200.0 {
		t.Fatalf("rates after a repeated reading = %v/%v; want 100/200", upload, download)
	}
	record(2, nil)
	if upload, download := rates(); upload != nil || download != nil {
		t.Fatalf("rates after a sample without traffic = %v/%v; want no live sample", upload, download)
	}
	record(3, reading(300, 600))
	if upload, download := rates(); upload != 100.0 || download != 200.0 {
		t.Fatalf("rates across the missing reading = %v/%v; want 100/200", upload, download)
	}
}
