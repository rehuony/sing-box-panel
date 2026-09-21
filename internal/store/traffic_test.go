// SPDX-License-Identifier: GPL-3.0-or-later

package store

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/rehuony/sing-box-panel/internal/configuration"
)

func TestTrafficPeriodsAreMonotonicAndQueryable(t *testing.T) {
	ctx := context.Background()
	database := openTestStore(t, ctx)
	start := time.Date(2026, time.August, 1, 0, 0, 0, 0, time.UTC)
	period, err := database.UpsertTrafficPeriod(ctx, TrafficPeriod{
		ID: "traffic-2026-08", PeriodStart: start, PeriodEnd: start.AddDate(0, 1, 0),
		InboundBytes: 10, OutboundBytes: 20, Counters: json.RawMessage(`{"proxy-a":30}`),
		CreatedAt: start,
	})
	if err != nil {
		t.Fatal(err)
	}
	period.InboundBytes = 30
	period.OutboundBytes = 50
	period.Counters = json.RawMessage(`{"proxy-a":80}`)
	updated, err := database.UpsertTrafficPeriod(ctx, period)
	if err != nil {
		t.Fatal(err)
	}
	if updated.InboundBytes != 30 || updated.OutboundBytes != 50 {
		t.Fatalf("updated period = %+v", updated)
	}

	decreased := updated
	decreased.InboundBytes--
	if _, err := database.UpsertTrafficPeriod(ctx, decreased); err == nil {
		t.Fatal("decreasing traffic sample succeeded")
	}
	current, err := database.CurrentTrafficPeriod(ctx, start.Add(12*time.Hour))
	if err != nil || current.ID != period.ID {
		t.Fatalf("current = %+v, err=%v", current, err)
	}
	from, until := start.Add(-time.Hour), start.Add(time.Hour)
	listed, err := database.ListTrafficPeriods(ctx, TrafficPeriodFilter{
		OverlapsStart: &from, OverlapsEnd: &until, Limit: 10,
	})
	if err != nil || len(listed) != 1 || listed[0].ID != period.ID {
		t.Fatalf("listed = %+v, err=%v", listed, err)
	}
	if _, err := database.CurrentTrafficPeriod(ctx, start.AddDate(1, 0, 0)); !errors.Is(err, ErrTrafficPeriodNotFound) {
		t.Fatalf("missing current error = %v", err)
	}
}

func TestTrafficPeriodRejectsInvalidBoundariesAndJSON(t *testing.T) {
	ctx := context.Background()
	database := openTestStore(t, ctx)
	now := time.Now().UTC()
	for _, period := range []TrafficPeriod{
		{ID: "bad-boundary", PeriodStart: now, PeriodEnd: now, Counters: json.RawMessage(`{}`)},
		{ID: "bad-counter", PeriodStart: now, PeriodEnd: now.Add(time.Hour), InboundBytes: -1, Counters: json.RawMessage(`{}`)},
		{ID: "bad-json", PeriodStart: now, PeriodEnd: now.Add(time.Hour), Counters: json.RawMessage(`[]`)},
	} {
		if _, err := database.UpsertTrafficPeriod(ctx, period); err == nil {
			t.Fatalf("invalid period succeeded: %+v", period)
		}
	}
}

func TestTrafficPeriodPaginationUsesPairedStableCursor(t *testing.T) {
	ctx := context.Background()
	database := openTestStore(t, ctx)
	base := time.Date(2026, time.June, 1, 0, 0, 0, 0, time.UTC)
	for index, id := range []string{"period-oldest", "period-middle", "period-newest"} {
		start := base.AddDate(0, index, 0)
		if _, err := database.UpsertTrafficPeriod(ctx, TrafficPeriod{
			ID: id, PeriodStart: start, PeriodEnd: start.AddDate(0, 1, 0),
			Counters: json.RawMessage(`{}`), CreatedAt: start,
		}); err != nil {
			t.Fatal(err)
		}
	}
	first, err := database.ListTrafficPeriodPage(ctx, TrafficPeriodFilter{Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Items) != 2 || first.Items[0].ID != "period-newest" ||
		first.Items[1].ID != "period-middle" || first.Next == nil {
		t.Fatalf("first page = %+v", first)
	}
	second, err := database.ListTrafficPeriodPage(ctx, TrafficPeriodFilter{Cursor: first.Next, Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(second.Items) != 1 || second.Items[0].ID != "period-oldest" || second.Next != nil {
		t.Fatalf("second page = %+v", second)
	}
	if _, err := database.ListTrafficPeriodPage(ctx, TrafficPeriodFilter{
		Cursor: &TrafficPeriodCursor{PeriodStart: first.Next.PeriodStart}, Limit: 2,
	}); err == nil {
		t.Fatal("incomplete traffic cursor succeeded")
	}
}

func TestTrafficSamplesUseProvenDeltasAcrossPeriodsAndProcessIncarnations(t *testing.T) {
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
		ActivationBundleID: bundle.ID, PID: 101, ProcessStartToken: "process-one",
		PeriodStart: periodOneStart, PeriodEnd: periodTwoStart,
		MemoryBytes: 1024, ActiveConnections: 2,
	}
	first := base
	first.SampledAt, first.UploadTotal, first.DownloadTotal = now, 1000, 2000
	if result := record(first); result.Sample.UploadDelta != nil || result.Period.OutboundBytes != 0 ||
		!strings.Contains(string(result.Period.Counters), `"traffic_evidence_available":false`) {
		t.Fatalf("unproven first sample = %+v", result)
	}
	second := base
	second.SampledAt, second.UploadTotal, second.DownloadTotal = now.Add(10*time.Second), 1100, 2200
	if result := record(second); result.Sample.UploadDelta == nil || *result.Sample.UploadDelta != 100 ||
		result.Sample.IntervalStart == nil || !result.Sample.IntervalStart.Equal(first.SampledAt) ||
		result.Sample.IntervalEnd == nil || !result.Sample.IntervalEnd.Equal(second.SampledAt) ||
		result.Sample.Coverage != CoverageComplete || result.Period.OutboundBytes != 100 || result.Period.InboundBytes != 200 ||
		!strings.Contains(string(result.Period.Counters), `"traffic_evidence_available":true`) {
		t.Fatalf("second sample = %+v", result)
	}

	crossed := second
	crossed.SampledAt, crossed.PeriodStart, crossed.PeriodEnd = periodTwoStart.Add(5*time.Second), periodTwoStart, periodTwoStart.AddDate(0, 1, 0)
	crossed.UploadTotal, crossed.DownloadTotal = 1200, 2400
	if result := record(crossed); result.Sample.UploadDelta == nil || *result.Sample.UploadDelta != 100 ||
		result.Sample.IntervalStart == nil || !result.Sample.IntervalStart.Equal(second.SampledAt) ||
		result.Sample.IntervalEnd == nil || !result.Sample.IntervalEnd.Equal(crossed.SampledAt) ||
		result.Sample.Coverage != CoveragePartial || result.Period.OutboundBytes != 0 || result.Period.InboundBytes != 0 {
		t.Fatalf("cross-period sample = %+v", result)
	}

	restarted := crossed
	restarted.PID, restarted.ProcessStartToken = 202, "process-two"
	restarted.SampledAt = crossed.SampledAt.Add(10 * time.Second)
	restarted.UploadTotal, restarted.DownloadTotal = 50, 70
	if result := record(restarted); result.Sample.UploadDelta != nil || result.Sample.IntervalStart != nil || result.Period.OutboundBytes != 0 {
		t.Fatalf("first restarted-process sample = %+v", result)
	}
	restarted.SampledAt = restarted.SampledAt.Add(10 * time.Second)
	restarted.UploadTotal, restarted.DownloadTotal = 80, 110
	if result := record(restarted); result.Sample.UploadDelta == nil || *result.Sample.UploadDelta != 30 ||
		result.Period.OutboundBytes != 30 || result.Period.InboundBytes != 40 {
		t.Fatalf("proven restarted-process sample = %+v", result)
	}

	regressed := restarted
	regressed.SampledAt = regressed.SampledAt.Add(10 * time.Second)
	regressed.UploadTotal = 79
	if result := record(regressed); result.Sample.Accepted || result.Sample.DiagnosticCode != "counter_decreased" ||
		result.Sample.IntervalStart != nil || result.Period.OutboundBytes != 30 {
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
		ActivationBundleID: bundle.ID, PID: 303, ProcessStartToken: "history-process",
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
	secondBundle, err := database.SaveActivationBundle(ctx, ActivationBundle{
		ID: "bundle-history-second", StartupArtifactID: bundle.StartupArtifactID,
		MonitoringTier: MonitoringLimited, CreatedAt: from.Add(30 * time.Second),
	})
	if err != nil {
		t.Fatal(err)
	}
	input.ActivationBundleID = secondBundle.ID
	input.SampledAt = from.Add(30 * time.Second)
	input.UploadTotal, input.DownloadTotal = 220, 440
	if result, err := database.RecordTrafficSample(ctx, input); err != nil || result.Sample.UploadDelta != nil || result.Sample.IntervalStart != nil {
		t.Fatalf("bundle-switch sample = %+v, err=%v", result, err)
	}
	history, err := database.MetricsHistory(ctx, MetricsHistoryFilter{
		From: from, To: from.Add(time.Minute), BucketSeconds: 20, ActivationBundleID: bundle.ID,
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
		From: from, To: from.Add(time.Minute), BucketSeconds: 20, ActivationBundleID: secondBundle.ID,
	})
	if err != nil || secondHistory.Buckets[1].UploadBytes != nil ||
		secondHistory.Buckets[1].Coverage != CoveragePartial || secondHistory.Buckets[0].Coverage != CoverageMissing {
		t.Fatalf("bundle-filtered history = %+v, err=%v", secondHistory, err)
	}
	fractionalHistory, err := database.MetricsHistory(ctx, MetricsHistoryFilter{
		From: from.Add(500 * time.Millisecond), To: from.Add(40*time.Second + 500*time.Millisecond),
		BucketSeconds: 20, ActivationBundleID: bundle.ID,
	})
	if err != nil || fractionalHistory.Buckets[0].UploadBytes == nil || *fractionalHistory.Buckets[0].UploadBytes != 50 ||
		fractionalHistory.Buckets[0].Coverage != CoveragePartial || fractionalHistory.Buckets[1].Coverage != CoverageMissing {
		t.Fatalf("fractional-boundary history = %+v, err=%v", fractionalHistory, err)
	}
	if _, err := database.MetricsHistory(ctx, MetricsHistoryFilter{
		From: from, To: from.Add(90*24*time.Hour + time.Second), BucketSeconds: 3600,
	}); err == nil {
		t.Fatal("history range over 90 days succeeded")
	}
	if _, err := database.MetricsHistory(ctx, MetricsHistoryFilter{
		From: from, To: from.Add(24 * time.Hour), BucketSeconds: 1,
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
			ActivationBundleID: bundle.ID, PID: 707, ProcessStartToken: "boundary-process",
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
			From: from, To: from.Add(20 * time.Second), BucketSeconds: 20,
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
			ActivationBundleID: bundle.ID, PID: 808, ProcessStartToken: "gap-process",
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
			From: from, To: from.Add(40 * time.Second), BucketSeconds: 40,
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
		ActivationBundleID: bundle.ID, PID: 404, ProcessStartToken: "retention-process",
		PeriodStart: from.Truncate(24 * time.Hour), PeriodEnd: from.Truncate(24*time.Hour).AddDate(0, 1, 0),
		MemoryBytes: 100, ActiveConnections: 1, UploadTotal: 10, DownloadTotal: 20, SampledAt: from,
	}
	if _, err := database.RecordTrafficSample(ctx, input); err != nil {
		t.Fatal(err)
	}
	deleted, err := database.DeleteTrafficSamplesBefore(ctx, from.Add(time.Second))
	if err != nil || deleted != 1 {
		t.Fatalf("deleted = %d, err=%v", deleted, err)
	}
	if _, err := database.CurrentTrafficPeriod(ctx, from); err != nil {
		t.Fatalf("traffic period was not retained: %v", err)
	}
}

type trafficTestHelper interface {
	Helper()
	Fatal(args ...any)
}

func seedTrafficActivationBundle(t trafficTestHelper, ctx context.Context, database *Store, now time.Time) ActivationBundle {
	t.Helper()
	revision, err := saveTestConfiguration(ctx, database, 0, NewCanonicalRevision{
		ID: "revision-traffic", SchemaVersion: configuration.SchemaVersion,
		Document: configuration.Empty().CanonicalJSON(), CommandID: "command-traffic", CreatedAt: now,
	},
	)
	if err != nil {
		t.Fatal(err)
	}
	core := testCoreArtifact("core-traffic", 808, 'b', "amd64", now.Add(time.Second))
	if _, err := database.UpsertCoreArtifact(ctx, core); err != nil {
		t.Fatal(err)
	}
	startup, err := database.CreateStartupArtifact(ctx, StartupArtifact{
		ID: "startup-traffic", CanonicalRevisionID: revision.ID, ExactCoreVersion: core.ExactVersion,
		CoreArtifactID: core.ID, ConfigBytes: []byte(`{}`), CreatedAt: now.Add(2 * time.Second),
	})
	if err != nil {
		t.Fatal(err)
	}
	startup, err = database.CompleteStartupArtifactCheck(ctx, startup.ID, true, now.Add(3*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	bundle, err := database.SaveActivationBundle(ctx, ActivationBundle{
		ID: "bundle-traffic", StartupArtifactID: startup.ID,
		MonitoringTier: MonitoringProcessOnly, CreatedAt: now.Add(4 * time.Second),
	})
	if err != nil {
		t.Fatal(err)
	}
	return bundle
}
