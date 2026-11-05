package store

import (
	"testing"
	"time"
)

func TestMonthlyTrafficSurvivesPeriodChangesRetentionAndNewInstances(t *testing.T) {
	db := openTestStore(t, t.Context())
	august := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	bundle := seedTrafficActivationBundle(t, t.Context(), db, august)
	input := TrafficSampleInput{NodeID: testNodeID, InstanceID: "instance-a", ActivationBundleID: bundle.ID, PID: 101, ProcessStartToken: "first-process", PeriodStart: august, PeriodEnd: august.AddDate(0, 1, 0), SampledAt: august.Add(24 * time.Hour), UploadTotal: 1000, DownloadTotal: 2000}
	record := func() TrafficSampleResult {
		t.Helper()
		result, err := db.RecordTrafficSample(t.Context(), input)
		if err != nil {
			t.Fatal(err)
		}
		input.SampledAt = input.SampledAt.Add(time.Second)
		return result
	}
	record()
	input.UploadTotal, input.DownloadTotal = 1100, 2200
	record()
	input.PeriodStart, input.PeriodEnd = august.AddDate(0, -1, 0), august.AddDate(0, 2, 0)
	input.UploadTotal, input.DownloadTotal = 1200, 2400
	if got := record(); got.Period.OutboundBytes != 200 || got.Period.InboundBytes != 400 {
		t.Fatalf("expanded range reset or doubled totals: %+v", got.Period)
	}
	input.PeriodStart, input.PeriodEnd = august, august.AddDate(0, 1, 0)
	input.UploadTotal, input.DownloadTotal = 1250, 2500
	if got := record(); got.Period.OutboundBytes != 250 {
		t.Fatalf("switch back: %+v", got.Period)
	}
	expanded, err := db.GetTrafficPeriod(t.Context(), testNodeID, trafficPeriodID(august.AddDate(0, -1, 0), august.AddDate(0, 2, 0)))
	if err != nil || expanded.OutboundBytes != 250 {
		t.Fatalf("previously materialized range became stale: %+v %v", expanded, err)
	}
	page, err := db.ListTrafficPeriodPage(t.Context(), TrafficPeriodFilter{NodeID: testNodeID, Limit: 1})
	if err != nil || len(page.Items) != 1 || page.Items[0].OutboundBytes != 250 || page.Next == nil {
		t.Fatalf("monthly first page: %+v %v", page, err)
	}
	next, err := db.ListTrafficPeriodPage(t.Context(), TrafficPeriodFilter{NodeID: testNodeID, Cursor: page.Next, Limit: 1})
	if err != nil || len(next.Items) != 1 || next.Items[0].OutboundBytes != 250 || next.Next != nil {
		t.Fatalf("stale expanded period in next page: %+v %v", next, err)
	}
	input.InstanceID = "instance-b"
	input.UploadTotal, input.DownloadTotal = 50000, 80000
	if got := record(); got.Period.OutboundBytes != 250 {
		t.Fatalf("a new instance's first counters were counted: %+v", got.Period)
	}
	input.UploadTotal, input.DownloadTotal = 50010, 80020
	record()
	if _, err := db.DeleteTrafficSamplesBefore(t.Context(), input.SampledAt); err != nil {
		t.Fatal(err)
	}
	got, err := db.AggregateTrafficPeriod(t.Context(), testNodeID, august, august.AddDate(0, 1, 0), input.SampledAt)
	if err != nil || got.OutboundBytes != 260 || got.InboundBytes != 520 {
		t.Fatalf("durable monthly aggregate: %+v %v", got, err)
	}
}

func TestMonthlyTrafficBooksCrossMonthDeltaAtObservation(t *testing.T) {
	db := openTestStore(t, t.Context())
	start := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	bundle := seedTrafficActivationBundle(t, t.Context(), db, start)
	input := TrafficSampleInput{NodeID: testNodeID, InstanceID: "instance-a", ActivationBundleID: bundle.ID, PID: 101, ProcessStartToken: "same-process", PeriodStart: start, PeriodEnd: start.AddDate(0, 3, 0), SampledAt: start.AddDate(0, 1, 0).Add(-time.Second), UploadTotal: 100, DownloadTotal: 200}
	if _, err := db.RecordTrafficSample(t.Context(), input); err != nil {
		t.Fatal(err)
	}
	input.SampledAt = input.SampledAt.Add(2 * time.Second)
	input.UploadTotal, input.DownloadTotal = 1000, 2000
	got, err := db.RecordTrafficSample(t.Context(), input)
	if err != nil || got.Period.OutboundBytes != 900 || got.Period.InboundBytes != 1800 {
		t.Fatalf("cross-month delta was lost: %+v %v", got, err)
	}
	input.SampledAt = input.SampledAt.Add(time.Second)
	input.UploadTotal, input.DownloadTotal = 1007, 2011
	got, err = db.RecordTrafficSample(t.Context(), input)
	if err != nil || got.Period.OutboundBytes != 907 || got.Period.InboundBytes != 1811 {
		t.Fatalf("proven August delta: %+v %v", got, err)
	}
}

func TestTrafficAccountingKeepsLongGapsAndRejectsDuplicateSamplesAtomically(t *testing.T) {
	ctx := t.Context()
	db := openTestStore(t, ctx)
	start := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	empty, err := db.AggregateTrafficPeriod(ctx, testNodeID, start, start.AddDate(0, 1, 0), start)
	if err != nil || empty.InboundBytes != 0 || empty.OutboundBytes != 0 || !empty.PeriodStart.Equal(start) {
		t.Fatalf("empty ledger: %+v %v", empty, err)
	}
	bundle := seedTrafficActivationBundle(t, ctx, db, start)
	input := TrafficSampleInput{NodeID: testNodeID, InstanceID: "instance-a", ActivationBundleID: bundle.ID, PID: 101, ProcessStartToken: "gap-process",
		PeriodStart: start, PeriodEnd: start.AddDate(0, 1, 0), SampledAt: start.Add(time.Hour), UploadTotal: 100, DownloadTotal: 200}
	if _, err := db.RecordTrafficSample(ctx, input); err != nil {
		t.Fatal(err)
	}
	input.SampledAt = input.SampledAt.Add(time.Hour)
	input.UploadTotal, input.DownloadTotal = 130, 260
	// A failure after updating the month must roll back both the sample and sum.
	if _, err := db.db.ExecContext(ctx, `CREATE TRIGGER reject_checkpoint BEFORE UPDATE ON traffic_checkpoint BEGIN SELECT RAISE(ABORT,'checkpoint failure'); END`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.RecordTrafficSample(ctx, input); err == nil {
		t.Fatal("checkpoint failure was ignored")
	}
	if _, err := db.db.ExecContext(ctx, `DROP TRIGGER reject_checkpoint`); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		got, err := db.RecordTrafficSample(ctx, input)
		if err != nil || got.Period.OutboundBytes != 30 || got.Period.InboundBytes != 60 || got.Sample.Accepted != (i == 0) {
			t.Fatalf("long gap or duplicate: %+v %v", got, err)
		}
		if got.Sample.Coverage != CoveragePartial {
			t.Fatal("chart gap was incorrectly marked complete")
		}
	}
	path := db.Path()
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	input.SampledAt = input.SampledAt.Add(time.Minute)
	input.UploadTotal, input.DownloadTotal = 150, 300
	got, err := reopened.RecordTrafficSample(ctx, input)
	if err != nil || got.Period.OutboundBytes != 50 || got.Period.InboundBytes != 100 {
		t.Fatalf("reopened ledger: %+v %v", got, err)
	}
}

func TestTrafficIsAccountedPerNodeAndInTheFleetTotal(t *testing.T) {
	ctx := t.Context()
	db := openTestStore(t, ctx)
	now := time.Now().UTC()
	if _, err := db.CreateNode(ctx, NewNode{ID: "node_other", Name: "other", CreatedAt: now, TokenHash: [32]byte{9}, ExpiresAt: now.Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}
	start := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	for _, node := range []struct {
		id       string
		upload   int64
		download int64
	}{{testNodeID, 100, 200}, {"node_other", 10, 20}} {
		input := TrafficSampleInput{NodeID: node.id, InstanceID: "instance-" + node.id, ActivationBundleID: "bundle", PID: 7,
			ProcessStartToken: "process", PeriodStart: start, PeriodEnd: start.AddDate(0, 1, 0), SampledAt: start.Add(time.Hour)}
		if _, err := db.RecordTrafficSample(ctx, input); err != nil {
			t.Fatal(err)
		}
		input.SampledAt, input.UploadTotal, input.DownloadTotal = input.SampledAt.Add(10*time.Second), node.upload, node.download
		if _, err := db.RecordTrafficSample(ctx, input); err != nil {
			t.Fatal(err)
		}
	}
	at := start.Add(2 * time.Hour)
	for _, want := range []struct {
		node              string
		outbound, inbound int64
	}{{testNodeID, 100, 200}, {"node_other", 10, 20}, {"", 110, 220}} {
		got, err := db.AggregateTrafficPeriod(ctx, want.node, start, start.AddDate(0, 1, 0), at)
		if err != nil || got.OutboundBytes != want.outbound || got.InboundBytes != want.inbound {
			t.Fatalf("traffic of %q = %+v, %v; want %d/%d", want.node, got, err, want.outbound, want.inbound)
		}
	}
	if err := db.DeleteNode(ctx, "node_other"); err != nil {
		t.Fatal(err)
	}
	if got, err := db.AggregateTrafficPeriod(ctx, "", start, start.AddDate(0, 1, 0), at); err != nil || got.OutboundBytes != 100 {
		t.Fatalf("fleet traffic after removing a node = %+v, %v", got, err)
	}
}
