// SPDX-License-Identifier: GPL-3.0-or-later

package store

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

// BenchmarkMetricsHistoryNinetyDays exercises the production query shape over
// one 10-second sample per interval for the full supported 90-day window.
func BenchmarkMetricsHistoryNinetyDays(b *testing.B) {
	ctx := context.Background()
	database, err := Open(ctx, filepath.Join(b.TempDir(), "panel.db"))
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { _ = database.Close() })
	from := time.Date(2026, time.June, 1, 0, 0, 0, 0, time.UTC)
	bundle := seedTrafficActivationBundle(b, ctx, database, from.Add(-time.Hour))
	if _, err := database.db.ExecContext(ctx, `
		WITH digits(d) AS (
			VALUES (0),(1),(2),(3),(4),(5),(6),(7),(8),(9)
		), samples(n) AS (
			SELECT a.d + b.d*10 + c.d*100 + d.d*1000 + e.d*10000 + f.d*100000
			  FROM digits a, digits b, digits c, digits d, digits e, digits f
			 WHERE a.d + b.d*10 + c.d*100 + d.d*1000 + e.d*10000 + f.d*100000 < 777600
		)
		INSERT INTO traffic_samples(
			activation_bundle_id, pid, process_start_token, sampled_at,
			memory_bytes, active_connections, upload_total, download_total,
			upload_delta, download_delta, interval_start, interval_end,
			coverage, accepted, diagnostic_code
		)
		SELECT ?, 606, 'benchmark-process',
		       strftime('%Y-%m-%dT%H:%M:%S', ? + (n+1)*10, 'unixepoch') || '.000000000Z',
		       1048576 + n%4096, n%64, n+1, (n+1)*2, 1, 2,
		       strftime('%Y-%m-%dT%H:%M:%S', ? + n*10, 'unixepoch') || '.000000000Z',
		       strftime('%Y-%m-%dT%H:%M:%S', ? + (n+1)*10, 'unixepoch') || '.000000000Z',
		       'complete', 1, ''
		  FROM samples
		 ORDER BY n`, bundle.ID, from.Unix(), from.Unix(), from.Unix()); err != nil {
		b.Fatal(err)
	}
	filter := MetricsHistoryFilter{
		From: from, To: from.Add(90 * 24 * time.Hour), BucketSeconds: 15188,
		ActivationBundleID: bundle.ID,
	}
	b.ResetTimer()
	for range b.N {
		history, err := database.MetricsHistory(ctx, filter)
		if err != nil {
			b.Fatal(err)
		}
		if len(history.Buckets) != 512 {
			b.Fatalf("bucket count = %d, want 512", len(history.Buckets))
		}
	}
}
