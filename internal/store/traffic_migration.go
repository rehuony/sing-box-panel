// SPDX-License-Identifier: GPL-3.0-or-later
package store

import (
	"context"
	"database/sql"
	"fmt"
)

// Version 11 first upgrades through the original monthly accounting schema.
const legacyTrafficMonthsSchema = `CREATE TABLE traffic_months (
    month_start TEXT PRIMARY KEY CHECK (month_start <> ''),
    inbound_bytes INTEGER NOT NULL CHECK (inbound_bytes >= 0),
    outbound_bytes INTEGER NOT NULL CHECK (outbound_bytes >= 0),
    has_delta INTEGER NOT NULL CHECK (has_delta IN (0, 1)),
    complete INTEGER NOT NULL CHECK (complete IN (0, 1)),
    first_observed_at TEXT NOT NULL CHECK (first_observed_at <> ''),
    last_sample_at TEXT NOT NULL CHECK (last_sample_at >= first_observed_at)
) STRICT;
`

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

// migrateTrafficAccounting preserves the ledger and checkpoint, without replaying
// historical samples under the new month attribution rule.
func migrateTrafficAccounting(ctx context.Context, tx *sql.Tx) error {
	if _, err := tx.ExecContext(ctx, `
        ALTER TABLE traffic_months DROP COLUMN complete;
        ALTER TABLE traffic_months DROP COLUMN has_delta;
        ALTER TABLE traffic_checkpoint RENAME TO traffic_checkpoint_legacy;
    `); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, trafficCheckpointSchema); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, `
        INSERT INTO traffic_checkpoint
            (singleton,pid,process_start_token,activation_bundle_id,last_upload_total,last_download_total,sampled_at)
        SELECT singleton,pid,process_start_token,activation_bundle_id,last_upload_total,last_download_total,sampled_at
        FROM traffic_checkpoint_legacy;
        DROP TABLE traffic_checkpoint_legacy;
        UPDATE traffic_periods SET counters_json=json_remove(counters_json,'$.coverage','$.traffic_evidence_available')
        WHERE json_type(counters_json,'$.coverage') IS NOT NULL
           OR json_type(counters_json,'$.traffic_evidence_available') IS NOT NULL;
    `)
	return err
}
