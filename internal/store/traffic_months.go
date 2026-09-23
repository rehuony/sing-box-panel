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

// seedTrafficMonths runs only during the version-11 upgrade. Existing single
// month totals take precedence over retained samples, which may already have
// expired. Multi-month records remain untouched and are never proportioned.
func seedTrafficMonths(ctx context.Context, tx *sql.Tx) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO traffic_months
        SELECT period_start, inbound_bytes, outbound_bytes,
               coalesce(json_extract(counters_json,'$.traffic_evidence_available'),0), 0,
               period_start, max(period_start, coalesce(json_extract(counters_json,'$.latest_sample_at'),created_at))
          FROM traffic_periods
         WHERE activation_bundle_id IS NULL
           AND period_start = strftime('%Y-%m-01T00:00:00.000000000Z',period_start)
           AND period_end = strftime('%Y-%m-01T00:00:00.000000000Z',period_start,'+1 month')
           AND id = 'traffic_'||strftime('%Y%m',period_start)||'_'||strftime('%Y%m',period_end)
        ON CONFLICT(month_start) DO NOTHING`)
	if err != nil {
		return fmt.Errorf("seed monthly period totals: %w", err)
	}
	// Normalize legacy JSON timestamps before comparing them to fixed-width
	// sample timestamps. Retained deltas strictly after a legacy total's last
	// sample can extend it without re-counting its original sample window.
	rows, err := tx.QueryContext(ctx, "SELECT month_start,last_sample_at FROM traffic_months")
	if err != nil {
		return err
	}
	type boundary struct{ start, last string }
	var boundaries []boundary
	for rows.Next() {
		var b boundary
		if err := rows.Scan(&b.start, &b.last); err != nil {
			rows.Close()
			return err
		}
		last, err := parseTime(b.last)
		if err != nil {
			rows.Close()
			return err
		}
		b.last = formatTime(last)
		boundaries = append(boundaries, b)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}
	for _, b := range boundaries {
		start, err := parseTime(b.start)
		if err != nil {
			return err
		}
		var inbound, outbound int64
		var evidence int
		var latest sql.NullString
		if err := tx.QueryRowContext(ctx, `SELECT coalesce(sum(download_delta),0),
      coalesce(sum(upload_delta),0),coalesce(max(upload_delta IS NOT NULL),0),max(sampled_at)
      FROM traffic_samples WHERE accepted=1 AND interval_start>=? AND sampled_at<?`,
			b.last, formatTime(start.AddDate(0, 1, 0))).Scan(&inbound, &outbound, &evidence, &latest); err != nil {
			return err
		}
		last := b.last
		if latest.Valid && latest.String > last {
			last = latest.String
		}
		if _, err := tx.ExecContext(ctx, `UPDATE traffic_months SET inbound_bytes=inbound_bytes+?,
      outbound_bytes=outbound_bytes+?,has_delta=max(has_delta,?),last_sample_at=? WHERE month_start=?`,
			inbound, outbound, evidence, last, b.start); err != nil {
			return err
		}
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO traffic_months
        SELECT strftime('%Y-%m-01T00:00:00.000000000Z',sampled_at),
               sum(CASE WHEN interval_start >= strftime('%Y-%m-01T00:00:00.000000000Z',sampled_at) THEN coalesce(download_delta,0) ELSE 0 END),
               sum(CASE WHEN interval_start >= strftime('%Y-%m-01T00:00:00.000000000Z',sampled_at) THEN coalesce(upload_delta,0) ELSE 0 END),
               max(CASE WHEN upload_delta IS NOT NULL AND interval_start >= strftime('%Y-%m-01T00:00:00.000000000Z',sampled_at) THEN 1 ELSE 0 END),
               0, min(sampled_at), max(sampled_at)
          FROM traffic_samples WHERE accepted=1 GROUP BY strftime('%Y-%m-01T00:00:00.000000000Z',sampled_at)
        ON CONFLICT(month_start) DO NOTHING`)
	return err
}

func recordTrafficMonth(ctx context.Context, tx *sql.Tx, sample TrafficSample) error {
	start := monthStart(sample.SampledAt)
	proven := sample.UploadDelta != nil && sample.IntervalStart != nil && !sample.IntervalStart.Before(start)
	upload, download := int64(0), int64(0)
	first := sample.SampledAt
	complete := sample.SampledAt.Equal(start)
	if proven {
		upload, download = *sample.UploadDelta, *sample.DownloadDelta
		first = *sample.IntervalStart
		complete = sample.Coverage == CoverageComplete
	}
	_, err := tx.ExecContext(ctx, `INSERT INTO traffic_months
        (month_start,inbound_bytes,outbound_bytes,has_delta,complete,first_observed_at,last_sample_at)
        VALUES(?,?,?,?,?,?,?)
        ON CONFLICT(month_start) DO UPDATE SET
          inbound_bytes=traffic_months.inbound_bytes+excluded.inbound_bytes,
          outbound_bytes=traffic_months.outbound_bytes+excluded.outbound_bytes,
          has_delta=max(traffic_months.has_delta,excluded.has_delta),
          complete=traffic_months.complete AND ? AND traffic_months.last_sample_at=?,
          last_sample_at=excluded.last_sample_at`,
		formatTime(start), download, upload, proven, complete && first.Equal(start), formatTime(first), formatTime(sample.SampledAt), complete, formatTime(first))
	return err
}

func (s *Store) AggregateTrafficPeriod(ctx context.Context, start, end, at time.Time) (TrafficPeriod, error) {
	return aggregateTrafficPeriod(ctx, s.db, start, end, at)
}

func aggregateTrafficPeriod(ctx context.Context, q queryRower, start, end, at time.Time) (TrafficPeriod, error) {
	if !start.Equal(monthStart(start)) || !end.Equal(monthStart(end)) || !end.After(start) || at.Before(start) {
		return TrafficPeriod{}, fmt.Errorf("invalid monthly traffic range")
	}
	var inbound, outbound int64
	var count, complete, evidence int
	var first, last sql.NullString
	err := q.QueryRowContext(ctx, `SELECT coalesce(sum(inbound_bytes),0),coalesce(sum(outbound_bytes),0),
        count(*),coalesce(sum(complete AND (month_start=? OR last_sample_at>=strftime('%Y-%m-01T00:00:00.000000000Z',month_start,'+1 month'))),0),
        coalesce(max(has_delta),0),min(first_observed_at),max(last_sample_at)
        FROM traffic_months WHERE month_start>=? AND month_start<? AND month_start<=?`,
		formatTime(monthStart(at)), formatTime(start), formatTime(end), formatTime(monthStart(at))).Scan(&inbound, &outbound, &count, &complete, &evidence, &first, &last)
	if err != nil {
		return TrafficPeriod{}, err
	}
	if count == 0 {
		return TrafficPeriod{}, ErrTrafficPeriodNotFound
	}
	through := minTime(at, end.Add(-time.Nanosecond))
	expected := (through.Year()-start.Year())*12 + int(through.Month()-start.Month()) + 1
	coverage := CoveragePartial
	if evidence == 0 {
		coverage = CoverageMissing
	} else if count == expected && complete == count {
		coverage = CoverageComplete
	}
	lastAt, err := parseTime(last.String)
	if err != nil {
		return TrafficPeriod{}, err
	}
	if through.Sub(lastAt) > maximumCompleteSampleGap && coverage == CoverageComplete {
		coverage = CoveragePartial
	}
	firstAt, err := parseTime(first.String)
	if err != nil {
		return TrafficPeriod{}, err
	}
	counters, err := json.Marshal(map[string]any{"traffic_evidence_available": evidence != 0, "coverage": coverage, "latest_sample_at": lastAt, "aggregation": "monthly"})
	if err != nil {
		return TrafficPeriod{}, err
	}
	return TrafficPeriod{ID: trafficPeriodID(start, end), PeriodStart: start, PeriodEnd: end, InboundBytes: inbound, OutboundBytes: outbound, Counters: counters, CreatedAt: firstAt}, nil
}

func minTime(a, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}
	return b
}
