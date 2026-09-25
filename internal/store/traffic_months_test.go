package store

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestMonthlyTrafficSurvivesPeriodChangesRetentionAndProcessRestart(t *testing.T) {
	db := openTestStore(t, t.Context())
	august := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	bundle := seedTrafficActivationBundle(t, t.Context(), db, august)
	input := TrafficSampleInput{ActivationBundleID: bundle.ID, PID: 101, ProcessStartToken: "first-process", PeriodStart: august, PeriodEnd: august.AddDate(0, 1, 0), SampledAt: august.Add(24 * time.Hour), UploadTotal: 1000, DownloadTotal: 2000}
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
	expanded, err := db.GetTrafficPeriod(t.Context(), trafficPeriodID(august.AddDate(0, -1, 0), august.AddDate(0, 2, 0)))
	if err != nil || expanded.OutboundBytes != 250 {
		t.Fatalf("previously materialized range became stale: %+v %v", expanded, err)
	}
	input.PID, input.ProcessStartToken = 102, "second-process"
	input.UploadTotal, input.DownloadTotal = 50000, 80000
	if got := record(); got.Period.OutboundBytes != 250 {
		t.Fatalf("restart lifetime counted: %+v", got.Period)
	}
	input.UploadTotal, input.DownloadTotal = 50010, 80020
	record()
	if _, err := db.DeleteTrafficSamplesBefore(t.Context(), input.SampledAt); err != nil {
		t.Fatal(err)
	}
	got, err := db.AggregateTrafficPeriod(t.Context(), august, august.AddDate(0, 1, 0), input.SampledAt)
	if err != nil || got.OutboundBytes != 260 || got.InboundBytes != 520 || !strings.Contains(string(got.Counters), `"coverage":"partial"`) {
		t.Fatalf("durable monthly aggregate: %+v %v", got, err)
	}
}

func TestMonthlyTrafficDoesNotGuessCrossMonthDelta(t *testing.T) {
	db := openTestStore(t, t.Context())
	start := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	bundle := seedTrafficActivationBundle(t, t.Context(), db, start)
	input := TrafficSampleInput{ActivationBundleID: bundle.ID, PID: 101, ProcessStartToken: "same-process", PeriodStart: start, PeriodEnd: start.AddDate(0, 3, 0), SampledAt: start.AddDate(0, 1, 0).Add(-time.Second), UploadTotal: 100, DownloadTotal: 200}
	if _, err := db.RecordTrafficSample(t.Context(), input); err != nil {
		t.Fatal(err)
	}
	input.SampledAt = input.SampledAt.Add(2 * time.Second)
	input.UploadTotal, input.DownloadTotal = 1000, 2000
	got, err := db.RecordTrafficSample(t.Context(), input)
	if err != nil || got.Period.OutboundBytes != 0 || got.Period.InboundBytes != 0 {
		t.Fatalf("cross-month delta was guessed: %+v %v", got, err)
	}
	input.SampledAt = input.SampledAt.Add(time.Second)
	input.UploadTotal, input.DownloadTotal = 1007, 2011
	got, err = db.RecordTrafficSample(t.Context(), input)
	if err != nil || got.Period.OutboundBytes != 7 || got.Period.InboundBytes != 11 {
		t.Fatalf("proven August delta: %+v %v", got, err)
	}
}

func TestVersion11UpgradePreservesSingleMonthTotalsAndBackfillsOnlyMissingMonths(t *testing.T) {
	ctx := context.Background()
	db := openTestStore(t, ctx)
	start := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	bundle := seedTrafficActivationBundle(t, ctx, db, start)
	input := TrafficSampleInput{ActivationBundleID: bundle.ID, PID: 101, ProcessStartToken: "migration-process", PeriodStart: start, PeriodEnd: start.AddDate(0, 1, 0), SampledAt: start.Add(time.Hour), UploadTotal: 100, DownloadTotal: 200}
	if _, err := db.RecordTrafficSample(ctx, input); err != nil {
		t.Fatal(err)
	}
	input.SampledAt = input.SampledAt.Add(time.Second)
	input.UploadTotal += 10
	input.DownloadTotal += 20
	if _, err := db.RecordTrafficSample(ctx, input); err != nil {
		t.Fatal(err)
	}
	// The full single-month total is larger than the retained sample window.
	legacy := TrafficPeriod{ID: "traffic_202608_202609", PeriodStart: start, PeriodEnd: start.AddDate(0, 1, 0), InboundBytes: 2000, OutboundBytes: 1000, CreatedAt: start, Counters: json.RawMessage(`{"traffic_evidence_available":true,"latest_sample_at":"2026-08-01T01:00:01Z"}`)}
	if _, err := db.UpsertTrafficPeriod(ctx, legacy); err != nil {
		t.Fatal(err)
	}
	// A later multi-month run may have more samples than the saved single-month record.
	input.SampledAt = input.SampledAt.Add(time.Second)
	input.UploadTotal += 7
	input.DownloadTotal += 9
	if _, err := db.RecordTrafficSample(ctx, input); err != nil {
		t.Fatal(err)
	}
	input.PeriodStart = start.AddDate(0, 1, 0)
	input.PeriodEnd = start.AddDate(0, 2, 0)
	input.SampledAt = input.PeriodStart.Add(time.Hour)
	if _, err := db.RecordTrafficSample(ctx, input); err != nil {
		t.Fatal(err)
	}
	input.SampledAt = input.SampledAt.Add(time.Second)
	input.UploadTotal += 3
	input.DownloadTotal += 5
	if _, err := db.RecordTrafficSample(ctx, input); err != nil {
		t.Fatal(err)
	}
	path := db.Path()
	if _, err := db.db.ExecContext(ctx, "DROP TABLE traffic_months; PRAGMA user_version=11"); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	upgraded, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer upgraded.Close()
	got, err := upgraded.AggregateTrafficPeriod(ctx, start, start.AddDate(0, 3, 0), input.SampledAt)
	if err != nil || got.OutboundBytes != 1010 || got.InboundBytes != 2014 {
		t.Fatalf("migration lost or duplicated bytes: %+v %v", got, err)
	}
	preserved, err := upgraded.GetTrafficPeriod(ctx, legacy.ID)
	if err != nil || preserved.OutboundBytes != 1000 {
		t.Fatalf("legacy history changed: %+v %v", preserved, err)
	}
	if info, err := upgraded.SchemaInfo(ctx); err != nil || info.Version != CurrentSchemaVersion {
		t.Fatal(info, err)
	}
}
