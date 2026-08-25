// SPDX-License-Identifier: GPL-3.0-or-later

package store

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"
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
