// SPDX-License-Identifier: GPL-3.0-or-later
package store

import (
	"context"
	"database/sql"
	_ "embed"
	"encoding/json"
	"fmt"
	"time"
)

//go:embed traffic_months.sql
var trafficMonthsSchema string

func monthStart(at time.Time) time.Time {
	at = at.UTC()
	return time.Date(at.Year(), at.Month(), 1, 0, 0, 0, 0, time.UTC)
}

// recordTrafficMonth books each accepted delta in its observation month. This
// ledger does not use the sampling quality metadata needed by historical charts.
func recordTrafficMonth(ctx context.Context, tx *sql.Tx, sample TrafficSample) error {
	upload, download := int64(0), int64(0)
	if sample.UploadDelta != nil && sample.DownloadDelta != nil {
		upload, download = *sample.UploadDelta, *sample.DownloadDelta
	}
	_, err := tx.ExecContext(ctx, `INSERT INTO traffic_months
        (month_start,inbound_bytes,outbound_bytes,first_observed_at,last_sample_at)
        VALUES(?,?,?,?,?)
        ON CONFLICT(month_start) DO UPDATE SET
          inbound_bytes=traffic_months.inbound_bytes+excluded.inbound_bytes,
          outbound_bytes=traffic_months.outbound_bytes+excluded.outbound_bytes,
          last_sample_at=excluded.last_sample_at`,
		formatTime(monthStart(sample.SampledAt)), download, upload, formatTime(sample.SampledAt), formatTime(sample.SampledAt))
	return err
}

func (s *Store) AggregateTrafficPeriod(ctx context.Context, start, end, at time.Time) (TrafficPeriod, error) {
	return aggregateTrafficPeriod(ctx, s.db, start, end, at)
}

// aggregateTrafficPeriod returns recorded usage, including zero for an empty
// range. It never depends on the availability of a current core or raw samples.
func aggregateTrafficPeriod(ctx context.Context, q queryRower, start, end, at time.Time) (TrafficPeriod, error) {
	if start.IsZero() || !start.Equal(monthStart(start)) || !end.Equal(monthStart(end)) || !end.After(start) || at.Before(start) {
		return TrafficPeriod{}, fmt.Errorf("invalid monthly traffic range")
	}
	var inbound, outbound int64
	var first, last sql.NullString
	err := q.QueryRowContext(ctx, `SELECT coalesce(sum(inbound_bytes),0),coalesce(sum(outbound_bytes),0),
        min(first_observed_at),max(last_sample_at)
        FROM traffic_months WHERE month_start>=? AND month_start<? AND month_start<=?`,
		formatTime(start), formatTime(end), formatTime(monthStart(at))).Scan(&inbound, &outbound, &first, &last)
	if err != nil {
		return TrafficPeriod{}, err
	}
	firstAt := start
	metadata := map[string]any{"aggregation": "monthly"}
	if first.Valid {
		firstAt, err = parseTime(first.String)
		if err != nil {
			return TrafficPeriod{}, err
		}
	}
	if last.Valid {
		lastAt, err := parseTime(last.String)
		if err != nil {
			return TrafficPeriod{}, err
		}
		metadata["latest_sample_at"] = lastAt
	}
	counters, err := json.Marshal(metadata)
	if err != nil {
		return TrafficPeriod{}, err
	}
	return TrafficPeriod{ID: trafficPeriodID(start, end), PeriodStart: start, PeriodEnd: end,
		InboundBytes: inbound, OutboundBytes: outbound, Counters: counters, CreatedAt: firstAt}, nil
}
