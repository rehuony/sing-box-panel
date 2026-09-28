package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"
)

const legacyTrafficCheckpointSchema = `CREATE TABLE traffic_checkpoint (
    singleton INTEGER PRIMARY KEY CHECK (singleton = 1),
    period_start TEXT NOT NULL CHECK (period_start <> ''),
    period_end TEXT NOT NULL CHECK (period_end <> ''),
    pid INTEGER NOT NULL CHECK (pid > 0),
    process_start_token TEXT NOT NULL CHECK (process_start_token <> ''),
    activation_bundle_id TEXT NOT NULL
        REFERENCES activation_bundles(id) ON DELETE RESTRICT,
    last_upload_total INTEGER NOT NULL CHECK (last_upload_total >= 0),
    last_download_total INTEGER NOT NULL CHECK (last_download_total >= 0),
    accumulated_upload INTEGER NOT NULL CHECK (accumulated_upload >= 0),
    accumulated_download INTEGER NOT NULL CHECK (accumulated_download >= 0),
    has_delta INTEGER NOT NULL DEFAULT 0 CHECK (has_delta IN (0, 1)),
    sampled_at TEXT NOT NULL CHECK (sampled_at <> ''),
    CHECK (period_end > period_start)
) STRICT;`

// restoreLegacyTrafficAccounting constructs the actual v12–14 tables before
// testing upgrades, rather than only changing the version on the current schema.
func restoreLegacyTrafficAccounting(t *testing.T, db *Store) {
	t.Helper()
	_, err := db.db.ExecContext(t.Context(), `
        ALTER TABLE traffic_months RENAME TO traffic_months_current;
        `+legacyTrafficMonthsSchema+`
        INSERT INTO traffic_months
        SELECT month_start,inbound_bytes,outbound_bytes,1,0,first_observed_at,last_sample_at FROM traffic_months_current;
        DROP TABLE traffic_months_current;
        ALTER TABLE traffic_checkpoint RENAME TO traffic_checkpoint_current;
        `+legacyTrafficCheckpointSchema+`
        INSERT INTO traffic_checkpoint
        SELECT singleton, strftime('%Y-%m-01T00:00:00.000000000Z',sampled_at),
            strftime('%Y-%m-01T00:00:00.000000000Z',sampled_at,'+1 month'),
            pid,process_start_token,activation_bundle_id,last_upload_total,last_download_total,0,0,1,sampled_at
        FROM traffic_checkpoint_current;
        DROP TABLE traffic_checkpoint_current;
    `)
	if err != nil {
		t.Fatal(err)
	}
}

func TestTrafficAccountingUpgradePreservesLedgerAndCheckpoint(t *testing.T) {
	for _, version := range []int{11, 12, 13, 14} {
		for _, fail := range []bool{false, true} {
			t.Run(fmt.Sprintf("version-%d/fail-%t", version, fail), func(t *testing.T) {
				ctx := t.Context()
				db := openTestStore(t, ctx)
				start := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
				bundle := seedTrafficActivationBundle(t, ctx, db, start)
				input := TrafficSampleInput{ActivationBundleID: bundle.ID, PID: 101, ProcessStartToken: "upgrade-process",
					PeriodStart: start, PeriodEnd: start.AddDate(0, 1, 0), SampledAt: start.Add(time.Hour), UploadTotal: 100, DownloadTotal: 200}
				if _, err := db.RecordTrafficSample(ctx, input); err != nil {
					t.Fatal(err)
				}
				input.SampledAt = input.SampledAt.Add(10 * time.Second)
				input.UploadTotal, input.DownloadTotal = 110, 220
				if _, err := db.RecordTrafficSample(ctx, input); err != nil {
					t.Fatal(err)
				}
				legacy := TrafficPeriod{ID: "traffic_202608_202609", PeriodStart: start, PeriodEnd: input.PeriodEnd,
					OutboundBytes: 1000, InboundBytes: 2000, CreatedAt: start,
					Counters: json.RawMessage(`{"coverage":"partial","traffic_evidence_available":true,"latest_sample_at":"2026-08-01T01:00:10Z","retained":"yes"}`)}
				if _, err := db.UpsertTrafficPeriod(ctx, legacy); err != nil {
					t.Fatal(err)
				}
				if _, err := db.db.ExecContext(ctx, `UPDATE traffic_months SET outbound_bytes=1000,inbound_bytes=2000`); err != nil {
					t.Fatal(err)
				}
				restoreLegacyTrafficAccounting(t, db)
				if version == 11 {
					if _, err := db.db.ExecContext(ctx, `DROP TABLE traffic_months`); err != nil {
						t.Fatal(err)
					}
				}
				if fail {
					if _, err := db.db.ExecContext(ctx, `CREATE TRIGGER reject_traffic_cleanup BEFORE UPDATE OF counters_json ON traffic_periods
                        BEGIN SELECT RAISE(ABORT,'reject traffic cleanup'); END`); err != nil {
						t.Fatal(err)
					}
				}
				if _, err := db.db.ExecContext(ctx, fmt.Sprintf("PRAGMA user_version=%d", version)); err != nil {
					t.Fatal(err)
				}
				path := db.Path()
				if err := db.Close(); err != nil {
					t.Fatal(err)
				}
				upgraded, err := Open(ctx, path)
				if fail {
					if err == nil {
						upgraded.Close()
						t.Fatal("failed migration succeeded")
					}
					raw, err := sql.Open("sqlite", path)
					if err != nil {
						t.Fatal(err)
					}
					defer raw.Close()
					assertPragmaInt(t, ctx, raw, 0, "user_version", version)
					var count int
					if err := raw.QueryRowContext(ctx, `SELECT count(*) FROM pragma_table_info('traffic_checkpoint') WHERE name='accumulated_upload'`).Scan(&count); err != nil || count != 1 {
						t.Fatalf("checkpoint migration was not rolled back: %d %v", count, err)
					}
					if err := raw.QueryRowContext(ctx, `SELECT count(*) FROM traffic_periods WHERE json_extract(counters_json,'$.coverage')='partial'`).Scan(&count); err != nil || count != 1 {
						t.Fatalf("period cleanup was not rolled back: %d %v", count, err)
					}
					if version == 11 {
						if err := raw.QueryRowContext(ctx, `SELECT count(*) FROM sqlite_schema WHERE name='traffic_months'`).Scan(&count); err != nil || count != 0 {
							t.Fatalf("initial migration was not rolled back: %d %v", count, err)
						}
					}
					return
				}
				if err != nil {
					t.Fatal(err)
				}
				defer upgraded.Close()
				for range 2 {
					got, err := upgraded.AggregateTrafficPeriod(ctx, start, input.PeriodEnd, input.SampledAt)
					if err != nil || got.OutboundBytes != 1000 || got.InboundBytes != 2000 {
						t.Fatalf("upgrade lost or replayed usage: %+v %v", got, err)
					}
					if err := upgraded.initializeSchema(ctx); err != nil {
						t.Fatal(err)
					}
				}
				var obsolete int
				if err := upgraded.db.QueryRowContext(ctx, `SELECT
                    (SELECT count(*) FROM pragma_table_info('traffic_months') WHERE name IN ('complete','has_delta')) +
                    (SELECT count(*) FROM pragma_table_info('traffic_checkpoint') WHERE name IN ('period_start','period_end','accumulated_upload','accumulated_download','has_delta'))`).Scan(&obsolete); err != nil || obsolete != 0 {
					t.Fatalf("obsolete accounting state remains: %d %v", obsolete, err)
				}
				archived, err := upgraded.GetTrafficPeriod(ctx, legacy.ID)
				if err != nil || archived.OutboundBytes != 1000 || strings.Contains(string(archived.Counters), "coverage") || strings.Contains(string(archived.Counters), "traffic_evidence_available") || !strings.Contains(string(archived.Counters), `"retained":"yes"`) {
					t.Fatalf("archive cleanup: %+v %v", archived, err)
				}
				// Continue from the old checkpoint, even after raw-sample retention.
				if _, err := upgraded.DeleteTrafficSamplesBefore(ctx, input.SampledAt.Add(time.Second)); err != nil {
					t.Fatal(err)
				}
				input.SampledAt = input.SampledAt.Add(time.Minute)
				input.UploadTotal, input.DownloadTotal = 117, 231
				got, err := upgraded.RecordTrafficSample(ctx, input)
				if err != nil || got.Period.OutboundBytes != 1007 || got.Period.InboundBytes != 2011 {
					t.Fatalf("checkpoint did not continue: %+v %v", got, err)
				}
				input.SampledAt = input.PeriodEnd.Add(time.Second)
				input.PeriodStart, input.PeriodEnd = input.PeriodEnd, input.PeriodEnd.AddDate(0, 1, 0)
				input.UploadTotal, input.DownloadTotal = 122, 239
				got, err = upgraded.RecordTrafficSample(ctx, input)
				if err != nil || got.Period.OutboundBytes != 5 || got.Period.InboundBytes != 8 {
					t.Fatalf("new cross-month rule: %+v %v", got, err)
				}
			})
		}
	}
}
