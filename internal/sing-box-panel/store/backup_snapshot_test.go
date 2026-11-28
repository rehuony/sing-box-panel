// SPDX-License-Identifier: GPL-3.0-or-later

package store

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/rehuony/sing-box-panel/internal/database"
)

// TestEveryPanelTableHasOneBackupCategory keeps backups complete as the
// schema grows: each table of the panel database is in exactly one category
// or belongs to the installation.
func TestEveryPanelTableHasOneBackupCategory(t *testing.T) {
	ctx := testContext(t)
	database := openTestStore(t, ctx)
	rows, err := database.db.QueryContext(ctx, `SELECT name FROM sqlite_schema
        WHERE type = 'table' AND name NOT LIKE 'sqlite_%' AND name <> 'migration_history'`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	homes := make(map[string]int)
	for _, table := range installationTables {
		homes[table]++
	}
	for _, category := range Categories {
		for _, table := range categoryTables[category] {
			homes[table]++
		}
	}
	var tables []string
	for rows.Next() {
		var table string
		if err := rows.Scan(&table); err != nil {
			t.Fatal(err)
		}
		tables = append(tables, table)
		if homes[table] != 1 {
			t.Errorf("table %s is in %d backup categories, want 1", table, homes[table])
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	for table := range homes {
		if !slices.Contains(tables, table) {
			t.Errorf("backup category names %s, which the schema does not have", table)
		}
	}
}

// TestSnapshotLeavesNothingItDeletesReadable keeps a backup's file free of
// what it leaves out and of what the panel deleted before, whether the panel
// runs or is stopped and whether the raw traffic samples stay, as they do in
// a manual backup and not in a scheduled one.
func TestSnapshotLeavesNothingItDeletesReadable(t *testing.T) {
	marker := func(kind string) string { return "MARK-" + kind + "-" + strings.Repeat("x", 48) }
	for _, tc := range []struct {
		name                    string
		running, withoutSamples bool
	}{
		{"running manual", true, false},
		{"running scheduled", true, true},
		{"stopped manual", false, false},
		{"stopped scheduled", false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := testContext(t)
			db := openTestStore(t, ctx)
			dataDir := filepath.Dir(db.handle.Path())
			now := time.Now().UTC()
			// Enough rows for indexes of more than one page. Sessions belong to
			// the installation; the pool node table has a delete trigger.
			for index := range 200 {
				token, credential := sha256.Sum256(fmt.Appendf(nil, "token-%d", index)), sha256.Sum256([]byte("credential"))
				pool := fmt.Sprintf("%s-%d", marker("pool"), index)
				for _, statement := range []struct {
					query string
					args  []any
				}{
					{`INSERT INTO auth_sessions (token_hash, csrf_token, expires_at, credential_hash) VALUES (?, ?, ?, ?)`,
						[]any{token[:], fmt.Sprintf("%s-%d", marker("session"), index), now.Add(time.Hour).Unix(), credential[:]}},
					{`INSERT INTO subscription_nodes (id, node_id, origin_key, origin_name, display_name, protocol, outbound_json, position, created_at, updated_at)
                      VALUES (?, ?, ?, 'in', ?, 'vless', ?, ?, 'now', 'now')`,
						[]any{fmt.Sprintf("pool-%d", index), testNodeID, pool, pool, `{"uuid":"` + pool + `"}`, index}},
				} {
					if _, err := db.db.ExecContext(ctx, statement.query, statement.args...); err != nil {
						t.Fatal(err)
					}
				}
			}
			// A kept table's record the panel deleted before the backup.
			if _, err := db.db.ExecContext(ctx, `INSERT INTO subscription_users (id, name, created_at, updated_at) VALUES ('gone', ?, 'now', 'now')`, marker("deleted")); err != nil {
				t.Fatal(err)
			}
			if _, err := db.db.ExecContext(ctx, `DELETE FROM subscription_users`); err != nil {
				t.Fatal(err)
			}
			if _, err := db.RecordTrafficSample(ctx, TrafficSampleInput{
				NodeID: testNodeID, InstanceID: "instance-a", ActivationBundleID: marker("sample"), PID: 101, ProcessStartToken: "process",
				PeriodStart: dayStart(now), PeriodEnd: dayStart(now).AddDate(0, 1, 0), SampledAt: now,
			}); err != nil {
				t.Fatal(err)
			}
			if !tc.running {
				if err := db.Close(); err != nil {
					t.Fatal(err)
				}
				// The stopped panel's file still holds the deleted record.
				if live, err := os.ReadFile(filepath.Join(dataDir, PanelFile)); err != nil || !bytes.Contains(live, []byte(marker("deleted"))) {
					t.Fatalf("live file holds the deleted record: %v, %v", err == nil && bytes.Contains(live, []byte(marker("deleted"))), err)
				}
			}
			dir := t.TempDir()
			categories := slices.DeleteFunc(slices.Clone(Categories), func(category Category) bool { return category == CategorySubscriptions })
			if _, err := Snapshot(ctx, dataDir, dir, categories, false, tc.withoutSamples); err != nil {
				t.Fatal(err)
			}
			raw, err := os.ReadFile(filepath.Join(dir, PanelFile))
			if err != nil {
				t.Fatal(err)
			}
			for _, kind := range []string{"session", "pool", "deleted", "sample"} {
				readable := bytes.Contains(raw, []byte(marker(kind)))
				if want := kind == "sample" && !tc.withoutSamples; readable != want {
					t.Errorf("%s records readable = %v, want %v", kind, readable, want)
				}
			}
			snapshot, err := database.OpenFile(filepath.Join(dir, PanelFile), true)
			if err != nil {
				t.Fatal(err)
			}
			defer snapshot.Close()
			var got string
			if err := snapshot.QueryRowContext(ctx, `SELECT
                (SELECT count(*) FROM nodes) || '|' || (SELECT count(*) FROM traffic_samples) || '|' ||
                (SELECT count(*) FROM traffic_days) || '|' || (SELECT count(*) FROM traffic_checkpoint) || '|' ||
                (SELECT count(*) FROM sqlite_schema WHERE type = 'trigger' AND name = 'subscription_nodes_release_exits')`).Scan(&got); err != nil {
				t.Fatal(err)
			}
			want := "1|1|1|1|1"
			if tc.withoutSamples {
				want = "1|0|1|1|1"
			}
			if got != want {
				t.Fatalf("nodes|samples|days|checkpoints|trigger = %s, want %s", got, want)
			}
		})
	}
}
