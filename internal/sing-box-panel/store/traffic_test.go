// SPDX-License-Identifier: GPL-3.0-or-later

package store

import (
	"context"
	"testing"
	"time"
)

func TestTrafficPeriodPaginationUsesPairedStableCursor(t *testing.T) {
	ctx := context.Background()
	database := openTestStore(t, ctx)
	base := time.Date(2026, time.June, 1, 0, 0, 0, 0, time.UTC)
	var ids []string
	for index := range 3 {
		start := base.AddDate(0, index, 0)
		if _, err := database.RecordTrafficSample(ctx, TrafficSampleInput{
			NodeID: testNodeID, InstanceID: "instance-a", ActivationBundleID: "bundle-traffic", PID: 101, ProcessStartToken: "process",
			PeriodStart: start, PeriodEnd: start.AddDate(0, 1, 0), SampledAt: start.Add(time.Hour),
			UploadTotal: int64(index * 10), DownloadTotal: int64(index * 20),
		}); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, trafficPeriodID(start, start.AddDate(0, 1, 0)))
	}
	first, err := database.ListTrafficPeriodPage(ctx, TrafficPeriodFilter{NodeID: testNodeID, Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Items) != 2 || first.Items[0].ID != ids[2] || first.Items[1].ID != ids[1] || first.Next == nil {
		t.Fatalf("first page = %+v", first)
	}
	second, err := database.ListTrafficPeriodPage(ctx, TrafficPeriodFilter{NodeID: testNodeID, Cursor: first.Next, Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(second.Items) != 1 || second.Items[0].ID != ids[0] || second.Next != nil {
		t.Fatalf("second page = %+v", second)
	}
	if _, err := database.ListTrafficPeriodPage(ctx, TrafficPeriodFilter{
		NodeID: testNodeID, Cursor: &TrafficPeriodCursor{PeriodStart: first.Next.PeriodStart}, Limit: 2,
	}); err == nil {
		t.Fatal("incomplete traffic cursor succeeded")
	}
}

func TestTrafficSamplesUseProvenDeltasAcrossPeriodsAndNodeInstances(t *testing.T) {
	ctx := context.Background()
	database := openTestStore(t, ctx)
	now := time.Date(2026, time.August, 31, 23, 59, 40, 0, time.UTC)
	bundle := seedTrafficActivationBundle(t, ctx, database, now.Add(-time.Hour))
	periodOneStart := time.Date(2026, time.August, 1, 0, 0, 0, 0, time.UTC)
	periodTwoStart := periodOneStart.AddDate(0, 1, 0)

	record := func(input TrafficSampleInput) TrafficSampleResult {
		t.Helper()
		result, err := database.RecordTrafficSample(ctx, input)
		if err != nil {
			t.Fatal(err)
		}
		return result
	}
	base := TrafficSampleInput{
		NodeID: testNodeID, InstanceID: "instance-a",
		ActivationBundleID: bundle.ID, PID: 101, ProcessStartToken: "process-one",
		PeriodStart: periodOneStart, PeriodEnd: periodTwoStart,
		MemoryBytes: 1024, ActiveConnections: 2,
	}
	first := base
	first.SampledAt, first.UploadTotal, first.DownloadTotal = now, 1000, 2000
	if result := record(first); result.Sample.UploadDelta != nil || result.Period.OutboundBytes != 0 {
		t.Fatalf("unproven first sample = %+v", result)
	}
	second := base
	second.SampledAt, second.UploadTotal, second.DownloadTotal = now.Add(10*time.Second), 1100, 2200
	if result := record(second); result.Sample.UploadDelta == nil || *result.Sample.UploadDelta != 100 ||
		result.Sample.IntervalStart == nil || !result.Sample.IntervalStart.Equal(first.SampledAt) ||
		result.Sample.IntervalEnd == nil || !result.Sample.IntervalEnd.Equal(second.SampledAt) ||
		result.Sample.Coverage != CoverageComplete || result.Period.OutboundBytes != 100 || result.Period.InboundBytes != 200 {
		t.Fatalf("second sample = %+v", result)
	}

	crossed := second
	crossed.SampledAt, crossed.PeriodStart, crossed.PeriodEnd = periodTwoStart.Add(5*time.Second), periodTwoStart, periodTwoStart.AddDate(0, 1, 0)
	crossed.UploadTotal, crossed.DownloadTotal = 1200, 2400
	if result := record(crossed); result.Sample.UploadDelta == nil || *result.Sample.UploadDelta != 100 ||
		result.Sample.IntervalStart == nil || !result.Sample.IntervalStart.Equal(second.SampledAt) ||
		result.Sample.IntervalEnd == nil || !result.Sample.IntervalEnd.Equal(crossed.SampledAt) ||
		result.Sample.Coverage != CoveragePartial || result.Period.OutboundBytes != 100 || result.Period.InboundBytes != 200 {
		t.Fatalf("cross-period sample = %+v", result)
	}

	// A core restart keeps counting: the node's counters are cumulative.
	restartedCore := crossed
	restartedCore.PID, restartedCore.ProcessStartToken = 202, "process-two"
	restartedCore.SampledAt = crossed.SampledAt.Add(10 * time.Second)
	restartedCore.UploadTotal, restartedCore.DownloadTotal = 1250, 2470
	if result := record(restartedCore); result.Sample.UploadDelta == nil || *result.Sample.UploadDelta != 50 || result.Period.OutboundBytes != 150 {
		t.Fatalf("sample after a core restart = %+v", result)
	}
	// A recreated node database starts a segment whose first counters are not guessed.
	restarted := restartedCore
	restarted.InstanceID = "instance-b"
	restarted.SampledAt = restartedCore.SampledAt.Add(10 * time.Second)
	restarted.UploadTotal, restarted.DownloadTotal = 50, 70
	if result := record(restarted); result.Sample.UploadDelta != nil || result.Sample.IntervalStart != nil || result.Period.OutboundBytes != 150 {
		t.Fatalf("first sample of a new instance = %+v", result)
	}
	restarted.SampledAt = restarted.SampledAt.Add(10 * time.Second)
	restarted.UploadTotal, restarted.DownloadTotal = 80, 110
	if result := record(restarted); result.Sample.UploadDelta == nil || *result.Sample.UploadDelta != 30 ||
		result.Period.OutboundBytes != 180 || result.Period.InboundBytes != 310 {
		t.Fatalf("proven new-instance sample = %+v", result)
	}

	regressed := restarted
	regressed.SampledAt = regressed.SampledAt.Add(10 * time.Second)
	regressed.UploadTotal = 79
	if result := record(regressed); result.Sample.Accepted || result.Sample.DiagnosticCode != "counter_decreased" ||
		result.Sample.IntervalStart != nil || result.Period.OutboundBytes != 180 {
		t.Fatalf("regressed sample = %+v", result)
	}
}

func TestMetricsHistoryPreservesPartialAndMissingCoverage(t *testing.T) {
	ctx := context.Background()
	database := openTestStore(t, ctx)
	from := time.Date(2026, time.August, 10, 12, 0, 0, 0, time.UTC)
	bundle := seedTrafficActivationBundle(t, ctx, database, from.Add(-time.Hour))
	periodStart := time.Date(2026, time.August, 1, 0, 0, 0, 0, time.UTC)
	input := TrafficSampleInput{
		NodeID: testNodeID, InstanceID: "instance-a", ActivationBundleID: bundle.ID, PID: 303, ProcessStartToken: "history-process",
		PeriodStart: periodStart, PeriodEnd: periodStart.AddDate(0, 1, 0),
		MemoryBytes: 100, ActiveConnections: 2,
	}
	for index, totals := range []int64{100, 140, 190} {
		input.SampledAt = from.Add(time.Duration(index) * 10 * time.Second)
		input.UploadTotal, input.DownloadTotal = totals, totals*2
		input.MemoryBytes, input.ActiveConnections = 100+int64(index*20), 2+int64(index)
		if _, err := database.RecordTrafficSample(ctx, input); err != nil {
			t.Fatal(err)
		}
	}
	secondBundle := trafficBundle{ID: "bundle-history-second"}
	input.ActivationBundleID = secondBundle.ID
	input.SampledAt = from.Add(30 * time.Second)
	input.UploadTotal, input.DownloadTotal = 220, 440
	// Cumulative node counters prove the delta across a bundle switch too.
	if result, err := database.RecordTrafficSample(ctx, input); err != nil || result.Sample.UploadDelta == nil || *result.Sample.UploadDelta != 30 {
		t.Fatalf("bundle-switch sample = %+v, err=%v", result, err)
	}
	history, err := database.MetricsHistory(ctx, MetricsHistoryFilter{
		NodeID: testNodeID, From: from, To: from.Add(time.Minute), BucketSeconds: 20, ActivationBundleID: bundle.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(history.Buckets) != 3 || history.Buckets[0].Coverage != CoverageComplete ||
		history.Buckets[0].UploadBytes == nil || *history.Buckets[0].UploadBytes != 90 ||
		history.Buckets[1].Coverage != CoveragePartial || history.Buckets[1].UploadBytes != nil ||
		history.Buckets[2].Coverage != CoverageMissing ||
		history.Buckets[2].UploadBytes != nil {
		t.Fatalf("history = %+v", history)
	}
	secondHistory, err := database.MetricsHistory(ctx, MetricsHistoryFilter{
		NodeID: testNodeID, From: from, To: from.Add(time.Minute), BucketSeconds: 20, ActivationBundleID: secondBundle.ID,
	})
	if err != nil || secondHistory.Buckets[1].UploadBytes == nil || *secondHistory.Buckets[1].UploadBytes != 30 ||
		secondHistory.Buckets[1].Coverage != CoveragePartial || secondHistory.Buckets[0].Coverage != CoverageMissing {
		t.Fatalf("bundle-filtered history = %+v, err=%v", secondHistory, err)
	}
	fractionalHistory, err := database.MetricsHistory(ctx, MetricsHistoryFilter{
		NodeID: testNodeID, From: from.Add(500 * time.Millisecond), To: from.Add(40*time.Second + 500*time.Millisecond),
		BucketSeconds: 20, ActivationBundleID: bundle.ID,
	})
	if err != nil || fractionalHistory.Buckets[0].UploadBytes == nil || *fractionalHistory.Buckets[0].UploadBytes != 50 ||
		fractionalHistory.Buckets[0].Coverage != CoveragePartial || fractionalHistory.Buckets[1].Coverage != CoverageMissing {
		t.Fatalf("fractional-boundary history = %+v, err=%v", fractionalHistory, err)
	}
	if _, err := database.MetricsHistory(ctx, MetricsHistoryFilter{
		NodeID: testNodeID, From: from, To: from.Add(90*24*time.Hour + time.Second), BucketSeconds: 3600,
	}); err == nil {
		t.Fatal("history range over 90 days succeeded")
	}
	if _, err := database.MetricsHistory(ctx, MetricsHistoryFilter{
		NodeID: testNodeID, From: from, To: from.Add(24 * time.Hour), BucketSeconds: 1,
	}); err == nil {
		t.Fatal("history with more than 512 buckets succeeded")
	}
}

func TestMetricsHistoryCoverageUsesIntervalBoundariesAndGapEvidence(t *testing.T) {
	from := time.Date(2026, time.August, 10, 12, 0, 0, 0, time.UTC)
	periodStart := time.Date(2026, time.August, 1, 0, 0, 0, 0, time.UTC)

	t.Run("boundary spanning intervals are complete", func(t *testing.T) {
		ctx := context.Background()
		database := openTestStore(t, ctx)
		bundle := seedTrafficActivationBundle(t, ctx, database, from.Add(-time.Hour))
		input := TrafficSampleInput{
			NodeID: testNodeID, InstanceID: "instance-a", ActivationBundleID: bundle.ID, PID: 707, ProcessStartToken: "boundary-process",
			PeriodStart: periodStart, PeriodEnd: periodStart.AddDate(0, 1, 0),
			MemoryBytes: 100, ActiveConnections: 2,
		}
		for index, offset := range []time.Duration{-5 * time.Second, 5 * time.Second, 15 * time.Second, 25 * time.Second} {
			input.SampledAt = from.Add(offset)
			input.UploadTotal = int64(100 + index*10)
			input.DownloadTotal = int64(200 + index*20)
			if _, err := database.RecordTrafficSample(ctx, input); err != nil {
				t.Fatal(err)
			}
		}

		history, err := database.MetricsHistory(ctx, MetricsHistoryFilter{
			NodeID: testNodeID, From: from, To: from.Add(20 * time.Second), BucketSeconds: 20,
			ActivationBundleID: bundle.ID,
		})
		if err != nil {
			t.Fatal(err)
		}
		if len(history.Buckets) != 1 || history.Buckets[0].Coverage != CoverageComplete {
			t.Fatalf("boundary-spanning history = %+v", history)
		}
	})

	t.Run("over-limit interval remains partial", func(t *testing.T) {
		ctx := context.Background()
		database := openTestStore(t, ctx)
		bundle := seedTrafficActivationBundle(t, ctx, database, from.Add(-time.Hour))
		input := TrafficSampleInput{
			NodeID: testNodeID, InstanceID: "instance-a", ActivationBundleID: bundle.ID, PID: 808, ProcessStartToken: "gap-process",
			PeriodStart: periodStart, PeriodEnd: periodStart.AddDate(0, 1, 0),
			MemoryBytes: 100, ActiveConnections: 2,
		}
		for index, offset := range []time.Duration{-5 * time.Second, 5 * time.Second, 45 * time.Second} {
			input.SampledAt = from.Add(offset)
			input.UploadTotal = int64(100 + index*10)
			input.DownloadTotal = int64(200 + index*20)
			if _, err := database.RecordTrafficSample(ctx, input); err != nil {
				t.Fatal(err)
			}
		}

		history, err := database.MetricsHistory(ctx, MetricsHistoryFilter{
			NodeID: testNodeID, From: from, To: from.Add(40 * time.Second), BucketSeconds: 40,
			ActivationBundleID: bundle.ID,
		})
		if err != nil {
			t.Fatal(err)
		}
		if len(history.Buckets) != 1 || history.Buckets[0].Coverage != CoveragePartial {
			t.Fatalf("gap history = %+v", history)
		}
	})
}

func TestDeleteTrafficSamplesBeforeKeepsPeriods(t *testing.T) {
	ctx := context.Background()
	database := openTestStore(t, ctx)
	from := time.Date(2026, time.August, 1, 12, 0, 0, 0, time.UTC)
	bundle := seedTrafficActivationBundle(t, ctx, database, from.Add(-time.Hour))
	input := TrafficSampleInput{
		NodeID: testNodeID, InstanceID: "instance-a", ActivationBundleID: bundle.ID, PID: 404, ProcessStartToken: "retention-process",
		PeriodStart: time.Date(2026, time.August, 1, 0, 0, 0, 0, time.UTC), PeriodEnd: time.Date(2026, time.September, 1, 0, 0, 0, 0, time.UTC),
		MemoryBytes: 100, ActiveConnections: 1, UploadTotal: 10, DownloadTotal: 20, SampledAt: from,
	}
	if _, err := database.RecordTrafficSample(ctx, input); err != nil {
		t.Fatal(err)
	}
	deleted, err := database.DeleteTrafficSamplesBefore(ctx, from.Add(time.Second))
	if err != nil || deleted != 1 {
		t.Fatalf("deleted = %d, err=%v", deleted, err)
	}
	if _, err := database.GetTrafficPeriod(ctx, testNodeID, trafficPeriodID(input.PeriodStart, input.PeriodEnd)); err != nil {
		t.Fatalf("traffic period was not retained: %v", err)
	}
}

type trafficTestHelper interface {
	Helper()
	Fatal(args ...any)
}

// trafficBundle names the activation bundle that traffic samples record.
type trafficBundle struct{ ID string }

// seedTrafficActivationBundle returns the bundle that traffic fixtures
// record; the panel keeps no bundles of its own.
func seedTrafficActivationBundle(t trafficTestHelper, _ context.Context, _ *Store, _ time.Time) trafficBundle {
	t.Helper()
	return trafficBundle{ID: "bundle-traffic"}
}
