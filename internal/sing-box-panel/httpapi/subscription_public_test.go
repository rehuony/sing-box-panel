// SPDX-License-Identifier: GPL-3.0-or-later

package httpapi

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/store"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/subscription"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/telemetry"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/testutil"
)

func TestPublicSubscriptionTrafficRefreshesOn304AndIsSharedAcrossChannels(t *testing.T) {
	ctx := t.Context()
	db, app, handler, channel, publish := newSubscriptionPublicationHTTPFixture(t, "/panel")
	// A node's own quota is not a subscription total: the header reports
	// usage only.
	nodeID := testutil.PublishNode(t, db, "edge", testutil.InboundDocument, "edge.example", "")
	quota := int64(100)
	if _, err := db.SaveNodeSettings(ctx, nodeID, 2, store.NodeSettings{PublicHost: "edge.example", Traffic: store.NodeTrafficSettings{QuotaGiB: &quota, PeriodMonths: 1, ResetDay: 1}}); err != nil {
		t.Fatal(err)
	}
	publish()
	inbound := syncInboundNode(t, app.Subscriptions, nodeID)
	current, err := app.Subscriptions.SubscriptionChannel(ctx, channel.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := app.Subscriptions.UpdateSubscriptionChannel(ctx, channel.ID, subscription.UpdateSubscriptionChannelRequest{
		Name: current.Name, Format: current.Format, Enabled: true, ExpectedUpdatedAt: current.UpdatedAt, Config: channelConfigWith(inbound),
	}); err != nil {
		t.Fatal(err)
	}
	key, err := app.Subscriptions.CreateSubscriptionToken(ctx, subscription.CreateSubscriptionTokenRequest{Label: "traffic"})
	if err != nil {
		t.Fatal(err)
	}
	path := "/panel/sub/" + key.Token + "/" + channel.ID
	first := publicSubscriptionRequest(handler, path)
	if first.Code != http.StatusOK || first.Header().Get("Subscription-Userinfo") != "upload=0; download=0; total=0" {
		t.Fatalf("first response status=%d traffic=%q", first.Code, first.Header().Get("Subscription-Userinfo"))
	}

	// Record the node's samples as its session does. Only proven deltas, not
	// the first lifetime counters, belong to the subscription's period usage.
	now := time.Now().UTC()
	start := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	for index, totals := range [][2]int64{{1000, 2000}, {1123, 2456}} {
		if err := app.Telemetry.RecordNodeSample(ctx, nodeID, "instance", telemetry.NodeSample{
			SampledAt: start.Add(time.Duration(index) * 10 * time.Second),
			Traffic: &telemetry.NodeTraffic{ActivationBundleID: "bundle-traffic", PID: 101, ProcessStartToken: "traffic-process",
				UploadTotal: totals[0], DownloadTotal: totals[1]},
		}); err != nil {
			t.Fatal(err)
		}
	}
	conditional := publicSubscriptionConditionalRequest(handler, path, first.Header().Get("ETag"))
	if conditional.Code != http.StatusNotModified || conditional.Body.Len() != 0 || conditional.Header().Get("ETag") != first.Header().Get("ETag") ||
		conditional.Header().Get("Subscription-Userinfo") != "upload=123; download=456; total=0" {
		t.Fatalf("conditional response status=%d traffic=%q", conditional.Code, conditional.Header().Get("Subscription-Userinfo"))
	}
	second, err := app.Subscriptions.CreateSubscriptionChannel(ctx, subscription.CreateSubscriptionChannelRequest{
		Name: "Mihomo", Format: store.SubscriptionFormatMihomo, Enabled: true, Config: channelConfigWith(inbound),
	})
	if err != nil {
		t.Fatal(err)
	}
	other := publicSubscriptionRequest(handler, "/panel/sub/"+key.Token+"/"+second.ID)
	if other.Code != http.StatusOK || other.Header().Get("Subscription-Userinfo") != conditional.Header().Get("Subscription-Userinfo") {
		t.Fatalf("other channel status=%d traffic=%q", other.Code, other.Header().Get("Subscription-Userinfo"))
	}
	// A channel that leaves the node out does not report the node's traffic.
	without, err := app.Subscriptions.CreateSubscriptionChannel(ctx, subscription.CreateSubscriptionChannelRequest{
		Name: "Without VLESS", Format: store.SubscriptionFormatSingBox, Enabled: true, Config: json.RawMessage(emptyChannelConfig),
	})
	if err != nil {
		t.Fatal(err)
	}
	excluded := publicSubscriptionRequest(handler, "/panel/sub/"+key.Token+"/"+without.ID)
	if excluded.Code != http.StatusOK || excluded.Header().Get("Subscription-Userinfo") != "upload=0; download=0; total=0" {
		t.Fatalf("channel without the node status=%d traffic=%q", excluded.Code, excluded.Header().Get("Subscription-Userinfo"))
	}
	usage, err := app.Subscriptions.SubscriptionToken(ctx, key.Metadata.ID)
	if err != nil || usage.SuccessfulRequestCount != 4 || usage.BodyResponseCount != 3 || usage.BytesServed != int64(first.Body.Len()+other.Body.Len()+excluded.Body.Len()) {
		t.Fatalf("unexpected response accounting: %+v, %v", usage, err)
	}
}

func TestPublicSubscriptionFailuresDoNotExposeTrafficOrConsumeDownloads(t *testing.T) {
	for _, failure := range []string{"authorization recheck", "traffic storage"} {
		for _, conditional := range []bool{false, true} {
			name := failure + "/body"
			if conditional {
				name = failure + "/conditional"
			}
			t.Run(name, func(t *testing.T) {
				ctx := t.Context()
				db, app, handler := newSubscriptionHTTPServices(t, "")
				// The header reads the traffic of the node the channel carries.
				edge := testutil.PublishNode(t, db, "edge", testutil.InboundDocument, "edge.example", "")
				manual := createSourceNodes(t, app, "manual", "socks5://manual.example:1080#manual\n")[0]
				inbound := syncInboundNode(t, app.Subscriptions, edge)
				channel, err := app.Subscriptions.CreateSubscriptionChannel(ctx, subscription.CreateSubscriptionChannelRequest{
					Name: "public", Format: store.SubscriptionFormatMihomo, Enabled: true, Config: channelConfigWith(inbound, manual),
				})
				if err != nil {
					t.Fatal(err)
				}
				key, err := app.Subscriptions.CreateSubscriptionToken(ctx, subscription.CreateSubscriptionTokenRequest{Label: "traffic"})
				if err != nil {
					t.Fatal(err)
				}
				path := "/sub/" + key.Token + "/" + channel.ID
				first := publicSubscriptionRequest(handler, path)
				if first.Code != http.StatusOK {
					t.Fatalf("initial request status=%d", first.Code)
				}
				unknown := publicSubscriptionRequest(handler, "/sub/unknown/"+channel.ID)
				if unknown.Code != http.StatusNotFound || unknown.Header().Get("Subscription-Userinfo") != "" {
					t.Fatalf("unknown key status=%d traffic=%q", unknown.Code, unknown.Header().Get("Subscription-Userinfo"))
				}
				// Delivery reads the clock once before its authorization snapshot and
				// again in the final recheck. Change state there to exercise the
				// delivery-time checks.
				calls := 0
				clock := func() time.Time {
					calls++
					switch {
					case failure == "traffic storage" && calls == 1:
						err = db.WithTx(ctx, func(tx *sql.Tx) error {
							_, err := tx.ExecContext(ctx, "DROP TABLE traffic_days")
							return err
						})
					case failure == "authorization recheck" && calls == 2:
						_, err = app.Subscriptions.RevokeSubscriptionToken(ctx, key.Metadata.ID)
					}
					if err != nil {
						t.Fatal(err)
					}
					return time.Now()
				}
				// A new telemetry service has read no traffic yet, so the
				// header reads the dropped ledger.
				records := app.Telemetry
				if failure == "traffic storage" {
					records = telemetry.New(db, app.Settings, telemetry.Options{})
				}
				handler.services.Subscriptions = subscription.New(db, app.Settings, records, subscription.Options{Now: clock})
				etag := ""
				if conditional {
					etag = first.Header().Get("ETag")
				}
				failed := publicSubscriptionConditionalRequest(handler, path, etag)
				wantStatus := http.StatusServiceUnavailable
				if failure == "authorization recheck" {
					wantStatus = http.StatusNotFound
				}
				if failed.Code != wantStatus || failed.Header().Get("Subscription-Userinfo") != "" {
					t.Fatalf("failed request status=%d traffic=%q, want status=%d without traffic", failed.Code, failed.Header().Get("Subscription-Userinfo"), wantStatus)
				}
				usage, err := app.Subscriptions.SubscriptionToken(ctx, key.Metadata.ID)
				if err != nil || usage.SuccessfulRequestCount != 1 || usage.BodyResponseCount != 1 || usage.BytesServed != int64(first.Body.Len()) {
					t.Fatalf("failed request changed accounting: %+v, %v", usage, err)
				}
			})
		}
	}
}

// syncInboundNode synchronizes the pool after a fleet node was published
// directly in the store and returns the pool ID of the node's inbound.
func syncInboundNode(t *testing.T, subscriptions *subscription.Service, nodeID string) string {
	t.Helper()
	if err := subscriptions.SyncPool(t.Context()); err != nil {
		t.Fatal(err)
	}
	pool, err := subscriptions.SubscriptionNodes(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	for _, node := range pool.Items {
		if node.Origin.NodeID == nodeID {
			return node.ID
		}
	}
	t.Fatalf("node %s has no inbound in the pool: %+v %+v", nodeID, pool.Items, pool.Diagnostics)
	return ""
}

// channelConfigWith is a channel whose one group holds nodes.
func channelConfigWith(nodes ...string) json.RawMessage {
	members := make([]map[string]string, len(nodes))
	for index, id := range nodes {
		members[index] = map[string]string{"kind": "node", "id": id}
	}
	encoded, _ := json.Marshal(map[string]any{
		"groups":       []any{map[string]any{"id": "main", "name": "Main", "enabled": true, "type": "select", "members": members}},
		"routes":       []any{},
		"default_exit": map[string]any{"kind": "group", "id": "main"},
	})
	return encoded
}
