// SPDX-License-Identifier: GPL-3.0-or-later

package application

import (
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/rehuony/sing-box-panel/internal/settings"
	"github.com/rehuony/sing-box-panel/internal/store"
	"github.com/rehuony/sing-box-panel/internal/testutil"
)

func TestPublicSubscriptionTrafficUsesLiveSettingsAndCurrentUTCPeriod(t *testing.T) {
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
	configuration := settings.Defaults()
	configuration.DataDir = t.TempDir()
	configuration.Auth.Email = testutil.AdminEmail
	configuration.Auth.PasswordHash = testutil.PasswordHash

	// Seed durable month totals without live samples. The subscription
	// must still report these after collection stops or raw samples expire.
	recordMonth := func(month string, upload, download int64) {
		t.Helper()
		err := db.WithTx(ctx, func(tx *sql.Tx) error {
			_, err := tx.ExecContext(ctx, `INSERT INTO traffic_months
				(month_start,inbound_bytes,outbound_bytes,first_observed_at,last_sample_at)
				VALUES(?,?,?,?,?)`, month, download, upload, month, month)
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	recordMonth("2026-07-01T00:00:00.000000000Z", 20, 40)
	recordMonth("2026-08-01T00:00:00.000000000Z", 123, 456)
	original, err := app.PublicSubscription(ctx, key.Token, channel.ID)
	if err != nil {
		t.Fatal(err)
	}
	app.settingsPath = filepath.Join(t.TempDir(), "setting.json")

	quota, zero, maximumQuota := int64(100), int64(0), int64((1<<63-1)/(1<<30))
	for _, test := range []struct {
		name     string
		at       string
		months   int
		quota    *int64
		addMonth bool
		want     PublicSubscriptionTraffic
	}{
		{"monthly before UTC boundary", "2026-09-01T07:59:59+08:00", 1, &quota, false, PublicSubscriptionTraffic{123, 456, 107374182400}},
		{"unlimited absent quota", "2026-08-31T23:59:59Z", 1, nil, false, PublicSubscriptionTraffic{123, 456, 0}},
		{"unlimited zero quota", "2026-08-31T23:59:59Z", 1, &zero, false, PublicSubscriptionTraffic{123, 456, 0}},
		{"largest quota", "2026-08-31T23:59:59Z", 1, &maximumQuota, false, PublicSubscriptionTraffic{123, 456, 9223372035781033984}},
		{"new monthly period without samples", "2026-09-01T08:00:00+08:00", 1, &quota, false, PublicSubscriptionTraffic{0, 0, 107374182400}},
		{"new month recorded usage", "2026-09-01T00:00:10Z", 1, &quota, true, PublicSubscriptionTraffic{7, 11, 107374182400}},
		{"live change to three months", "2026-09-30T23:59:59Z", 3, &quota, true, PublicSubscriptionTraffic{150, 507, 107374182400}},
		{"new three month period without samples", "2026-10-01T00:00:00Z", 3, &quota, true, PublicSubscriptionTraffic{0, 0, 107374182400}},
	} {
		t.Run(test.name, func(t *testing.T) {
			if err := db.WithTx(ctx, func(tx *sql.Tx) error {
				_, err := tx.ExecContext(ctx, "DELETE FROM traffic_months WHERE month_start = '2026-09-01T00:00:00.000000000Z'")
				return err
			}); err != nil {
				t.Fatal(err)
			}
			if test.addMonth {
				recordMonth("2026-09-01T00:00:00.000000000Z", 7, 11)
			}
			configuration.Traffic.QuotaGiB, configuration.Traffic.PeriodMonths = test.quota, test.months
			raw, err := json.Marshal(configuration)
			if err != nil {
				t.Fatal(err)
			}
			if err := settings.Replace(app.settingsPath, raw); err != nil {
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
	start := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	end := start.AddDate(0, 1, 0)
	history, err := db.AggregateTrafficPeriod(ctx, start, end, end.Add(-time.Second))
	if err != nil || history.OutboundBytes != 123 || history.InboundBytes != 456 {
		t.Fatalf("reset changed historical totals: %+v, %v", history, err)
	}

	// Preview renders configuration only and must not depend on traffic storage.
	if err := db.WithTx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, "DROP TABLE traffic_months")
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

func TestPublicSubscriptionTrafficRejectsInvalidAccountingSettings(t *testing.T) {
	app := &Application{settingsPath: filepath.Join(t.TempDir(), "setting.json")}
	for _, raw := range []string{
		`{"traffic":{"quota_gib":-1}}`,
		`{"traffic":{"quota_gib":8589934592}}`,
		`{"traffic":{"period_months":0}}`,
		`{"traffic":{"period_months":121}}`,
		`{`,
	} {
		if err := os.WriteFile(app.settingsPath, []byte(raw), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := app.publicSubscriptionTraffic(t.Context(), time.Now()); err == nil {
			t.Fatalf("invalid accounting settings accepted: %s", raw)
		}
	}
}
