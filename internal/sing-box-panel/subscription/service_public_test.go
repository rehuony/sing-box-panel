// SPDX-License-Identifier: GPL-3.0-or-later

package subscription

import (
	"database/sql"
	"encoding/json"
	"math"
	"path/filepath"
	"testing"
	"time"

	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/store"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/telemetry"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/testutil"
)

func TestPublicSubscriptionTrafficSumsEveryNodesOwnPeriod(t *testing.T) {
	ctx := t.Context()
	db, err := store.Open(ctx, filepath.Join(t.TempDir(), "panel.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	app := newSubscriptionTestApplication(db)
	if _, err := app.CreateSubscriptionSource(ctx, CreateSubscriptionSourceRequest{
		Name: "manual", Kind: store.SubscriptionSourceFile, Filter: &SourceFilter{},
		Content: new("socks5://manual.example:1080#manual"),
	}); err != nil {
		t.Fatal(err)
	}
	// Seed durable daily totals of two nodes without live samples. Node b
	// resets on the 15th and has a quota, but node a has none, so the
	// subscription's total stays unlimited. A third node's inbound is not in
	// the channel, so its traffic is not counted.
	nodes := []string{
		testutil.PublishNode(t, db, "edge-a", testutil.InboundDocument, "", "203.0.113.1"),
		testutil.PublishNode(t, db, "edge-b", testutil.InboundDocument, "", "203.0.113.2"),
	}
	unpublished := testutil.PublishNode(t, db, "edge-c", testutil.InboundDocument, "", "203.0.113.3")
	if err := app.SyncPool(ctx); err != nil {
		t.Fatal(err)
	}
	pool, err := db.SubscriptionPool(ctx)
	if err != nil || len(pool) != 4 {
		t.Fatalf("pool = %+v, %v", pool, err)
	}
	members := []any{}
	for _, node := range pool {
		if node.NodeID != unpublished {
			members = append(members, map[string]any{"kind": "node", "id": node.ID})
		}
	}
	config, _ := json.Marshal(map[string]any{"groups": []any{map[string]any{
		"id": "main", "name": "Main", "enabled": true, "type": "select", "members": members,
	}}, "routes": []any{}, "default_exit": map[string]any{"kind": "group", "id": "main"}})
	channel, err := app.CreateSubscriptionChannel(ctx, CreateSubscriptionChannelRequest{
		Name: "public", Format: store.SubscriptionFormatMihomo, Config: config, Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	key, err := app.CreateSubscriptionToken(ctx, CreateSubscriptionTokenRequest{Label: "traffic"})
	if err != nil {
		t.Fatal(err)
	}
	quota := int64(100)
	if _, err := db.SaveNodeSettings(ctx, nodes[1], 2, store.NodeSettings{Traffic: store.NodeTrafficSettings{QuotaGiB: &quota, PeriodMonths: 1, ResetDay: 15}}); err != nil {
		t.Fatal(err)
	}
	recordDay := func(node, day string, upload, download int64) {
		t.Helper()
		err := db.WithTx(ctx, func(tx *sql.Tx) error {
			_, err := tx.ExecContext(ctx, `INSERT INTO traffic_days
				(node_id,day_start,inbound_bytes,outbound_bytes,first_observed_at,last_sample_at)
				VALUES(?,?,?,?,?,?)`, node, day, download, upload, day, day)
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	recordDay(nodes[0], "2026-07-31T00:00:00.000000000Z", 20, 40)
	recordDay(nodes[0], "2026-08-10T00:00:00.000000000Z", 100, 400)
	recordDay(nodes[1], "2026-08-14T00:00:00.000000000Z", 5, 6)
	recordDay(nodes[1], "2026-08-20T00:00:00.000000000Z", 23, 56)
	recordDay(unpublished, "2026-08-20T00:00:00.000000000Z", 1000, 1000)
	original, err := app.PublicSubscription(ctx, key.Token, channel.ID)
	if err != nil {
		t.Fatal(err)
	}

	for _, test := range []struct {
		name     string
		at       string
		resetDay int
		want     PublicSubscriptionTraffic
	}{
		{"each node's own period", "2026-08-31T23:59:59Z", 15, PublicSubscriptionTraffic{UploadBytes: 123, DownloadBytes: 456}},
		{"one node starts a new UTC period", "2026-09-01T08:00:00+08:00", 15, PublicSubscriptionTraffic{UploadBytes: 23, DownloadBytes: 56}},
		{"before the reset day", "2026-08-14T12:00:00Z", 15, PublicSubscriptionTraffic{UploadBytes: 105, DownloadBytes: 406}},
		{"a changed reset day applies at once", "2026-08-31T23:59:59Z", 1, PublicSubscriptionTraffic{UploadBytes: 128, DownloadBytes: 462}},
	} {
		t.Run(test.name, func(t *testing.T) {
			settings, err := db.Node(ctx, nodes[1])
			if err != nil {
				t.Fatal(err)
			}
			if _, err := db.SaveNodeSettings(ctx, nodes[1], settings.Settings.Revision, store.NodeSettings{
				Traffic: store.NodeTrafficSettings{QuotaGiB: &quota, PeriodMonths: 1, ResetDay: test.resetDay},
			}); err != nil {
				t.Fatal(err)
			}
			at, err := time.Parse(time.RFC3339, test.at)
			if err != nil {
				t.Fatal(err)
			}
			app.now = func() time.Time { return at }
			got, err := app.PublicSubscription(ctx, key.Token, channel.ID)
			if err != nil {
				t.Fatal(err)
			}
			if got.Traffic != test.want {
				t.Fatalf("traffic = %+v, want %+v", got.Traffic, test.want)
			}
			if string(got.Body) != string(original.Body) || got.ETag != original.ETag {
				t.Fatal("traffic changes altered subscription content or ETag")
			}
		})
	}

	// Preview renders configuration only and must not depend on traffic storage.
	if err := db.WithTx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, "DROP TABLE traffic_days")
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := app.RenderSubscriptionDraft(ctx, "", channel.ID, nil); err != nil {
		t.Fatalf("preview unexpectedly depends on traffic storage: %v", err)
	}
	// A new telemetry service has read no traffic yet, so the header reads
	// the dropped ledger.
	app.telemetry = telemetry.New(db, app.settings, telemetry.Options{})
	if _, err := app.PublicSubscription(ctx, key.Token, channel.ID); err == nil {
		t.Fatal("traffic storage failure returned a successful subscription")
	}
}

// TestQuotaSumIsUnlimitedUnlessEveryNodeHasAQuota: the header's total adds
// the nodes' quotas, reads as unlimited when any node has none and never
// wraps around.
func TestQuotaSumIsUnlimitedUnlessEveryNodeHasAQuota(t *testing.T) {
	limited := func(bytes int64) telemetry.NodeTrafficUsage { return telemetry.NodeTrafficUsage{QuotaBytes: &bytes} }
	for _, test := range []struct {
		name  string
		nodes []telemetry.NodeTrafficUsage
		want  int64
	}{
		{"every node limited", []telemetry.NodeTrafficUsage{limited(100 << 30), limited(50 << 30)}, 150 << 30},
		{"one node unlimited", []telemetry.NodeTrafficUsage{limited(100 << 30), {}}, 0},
		{"no node", nil, 0},
		{"too large to add", []telemetry.NodeTrafficUsage{limited(math.MaxInt64 - 1), limited(2), limited(3)}, math.MaxInt64},
	} {
		if got := quotaSum(test.nodes); got != test.want {
			t.Errorf("%s: quotaSum = %d, want %d", test.name, got, test.want)
		}
	}
}
