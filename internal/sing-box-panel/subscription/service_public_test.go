// SPDX-License-Identifier: GPL-3.0-or-later

package subscription

import (
	"database/sql"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/store"
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
	if _, err := app.CreateSubscriptionNode(ctx, json.RawMessage(`{"type":"socks","tag":"manual","server":"manual.example","server_port":1080}`)); err != nil {
		t.Fatal(err)
	}
	channel, err := app.CreateSubscriptionChannel(ctx, CreateSubscriptionChannelRequest{
		Name: "public", Format: store.SubscriptionFormatMihomo, PublicHost: "public.example", Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	key, err := app.CreateSubscriptionToken(ctx, CreateSubscriptionTokenRequest{Label: "traffic"})
	if err != nil {
		t.Fatal(err)
	}

	// Seed durable daily totals of two nodes without live samples. Node b
	// resets on the 15th and has a quota of its own, which the header must
	// not present as a total.
	nodes := []string{testutil.CreateNode(t, db, "edge-a"), testutil.CreateNode(t, db, "edge-b")}
	quota := int64(100)
	if _, err := db.SaveNodeTrafficSettings(ctx, nodes[1], 1, store.NodeTrafficSettings{QuotaGiB: &quota, PeriodMonths: 1, ResetDay: 15}); err != nil {
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
		{"each node's own period", "2026-08-31T23:59:59Z", 15, PublicSubscriptionTraffic{123, 456, 0}},
		{"one node starts a new UTC period", "2026-09-01T08:00:00+08:00", 15, PublicSubscriptionTraffic{23, 56, 0}},
		{"before the reset day", "2026-08-14T12:00:00Z", 15, PublicSubscriptionTraffic{105, 406, 0}},
		{"a changed reset day applies at once", "2026-08-31T23:59:59Z", 1, PublicSubscriptionTraffic{128, 462, 0}},
	} {
		t.Run(test.name, func(t *testing.T) {
			settings, err := db.Node(ctx, nodes[1])
			if err != nil {
				t.Fatal(err)
			}
			if _, err := db.SaveNodeTrafficSettings(ctx, nodes[1], settings.Traffic.Revision, store.NodeTrafficSettings{
				QuotaGiB: &quota, PeriodMonths: 1, ResetDay: test.resetDay,
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
	if _, err := app.RenderSubscriptionPreview(ctx, "", channel.ID); err != nil {
		t.Fatalf("preview unexpectedly depends on traffic storage: %v", err)
	}
	if _, err := app.PublicSubscription(ctx, key.Token, channel.ID); err == nil {
		t.Fatal("traffic storage failure returned a successful subscription")
	}
}
