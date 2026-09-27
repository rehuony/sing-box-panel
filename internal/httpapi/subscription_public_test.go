// SPDX-License-Identifier: GPL-3.0-or-later

package httpapi

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/rehuony/sing-box-panel/internal/application"
	"github.com/rehuony/sing-box-panel/internal/settings"
	"github.com/rehuony/sing-box-panel/internal/store"
)

func TestPublicSubscriptionTrafficRefreshesOn304AndIsSharedAcrossChannels(t *testing.T) {
	ctx := t.Context()
	db, app, handler, channel, startup := newSubscriptionPublicationHTTPFixture(t, "/panel")
	configuration := handler.settings
	quota := int64(100)
	configuration.Traffic.QuotaGiB = &quota
	raw, err := json.Marshal(configuration)
	if err != nil {
		t.Fatal(err)
	}
	if err := settings.Replace(configuration.Path(), raw); err != nil {
		t.Fatal(err)
	}
	prepared, err := app.PrepareActivationBundle(ctx, startup.ID, store.MonitoringProcessOnly)
	if err != nil {
		t.Fatal(err)
	}
	applySubscriptionHTTPBundle(t, db, app, prepared.Bundle.ID)
	key, err := app.CreateSubscriptionToken(ctx, application.CreateSubscriptionTokenRequest{Label: "traffic"})
	if err != nil {
		t.Fatal(err)
	}
	path := "/panel/sub/" + key.Token + "/" + channel.ID
	first := publicSubscriptionRequest(handler, path)
	if first.Code != http.StatusOK || first.Header().Get("Subscription-Userinfo") != "upload=0; download=0; total=107374182400" {
		t.Fatalf("first response status=%d traffic=%q", first.Code, first.Header().Get("Subscription-Userinfo"))
	}

	// Use real collector writes while the core is stopped. Only proven deltas,
	// not the first lifetime counters, belong to the subscription's period usage.
	now := time.Now().UTC()
	start := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	input := store.TrafficSampleInput{
		ActivationBundleID: prepared.Bundle.ID, PID: 101, ProcessStartToken: "traffic-process",
		PeriodStart: start, PeriodEnd: start.AddDate(0, 1, 0), SampledAt: start,
		UploadTotal: 1000, DownloadTotal: 2000,
	}
	if _, err := db.RecordTrafficSample(ctx, input); err != nil {
		t.Fatal(err)
	}
	input.SampledAt = start.Add(time.Nanosecond)
	input.UploadTotal, input.DownloadTotal = 1123, 2456
	if _, err := db.RecordTrafficSample(ctx, input); err != nil {
		t.Fatal(err)
	}
	conditional := publicSubscriptionConditionalRequest(handler, path, first.Header().Get("ETag"))
	if conditional.Code != http.StatusNotModified || conditional.Body.Len() != 0 || conditional.Header().Get("ETag") != first.Header().Get("ETag") ||
		conditional.Header().Get("Subscription-Userinfo") != "upload=123; download=456; total=107374182400" {
		t.Fatalf("conditional response status=%d traffic=%q", conditional.Code, conditional.Header().Get("Subscription-Userinfo"))
	}
	second, err := app.CreateSubscriptionChannel(ctx, application.CreateSubscriptionChannelRequest{
		Name: "Mihomo", Format: store.SubscriptionFormatMihomo, PublicHost: "publish.example", Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	other := publicSubscriptionRequest(handler, "/panel/sub/"+key.Token+"/"+second.ID)
	if other.Code != http.StatusOK || other.Header().Get("Subscription-Userinfo") != conditional.Header().Get("Subscription-Userinfo") {
		t.Fatalf("other channel status=%d traffic=%q", other.Code, other.Header().Get("Subscription-Userinfo"))
	}
	usage, err := app.SubscriptionToken(ctx, key.Metadata.ID)
	if err != nil || usage.SuccessfulRequestCount != 3 || usage.BodyResponseCount != 2 || usage.BytesServed != int64(first.Body.Len()+other.Body.Len()) {
		t.Fatalf("unexpected response accounting: %+v, %v", usage, err)
	}
}

func TestPublicSubscriptionFailuresDoNotExposeTrafficOrConsumeDownloads(t *testing.T) {
	for _, failure := range []string{"authorization recheck", "traffic storage", "traffic settings"} {
		for _, conditional := range []bool{false, true} {
			name := failure + "/body"
			if conditional {
				name = failure + "/conditional"
			}
			t.Run(name, func(t *testing.T) {
				ctx := t.Context()
				db, app, handler := newSubscriptionHTTPServices(t, "")
				if _, err := app.CreateSubscriptionNode(ctx, json.RawMessage(`{"type":"socks","tag":"manual","server":"manual.example","server_port":1080}`)); err != nil {
					t.Fatal(err)
				}
				channel, err := app.CreateSubscriptionChannel(ctx, application.CreateSubscriptionChannelRequest{
					Name: "public", Format: store.SubscriptionFormatMihomo, Enabled: true,
				})
				if err != nil {
					t.Fatal(err)
				}
				key, err := app.CreateSubscriptionToken(ctx, application.CreateSubscriptionTokenRequest{Label: "traffic"})
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
				// The resolver runs during rendering, after the initial authorization
				// snapshot. Change state here to exercise the delivery-time checks.
				app.SetPublicIPResolver(func(context.Context) string {
					switch failure {
					case "authorization recheck":
						_, err = app.RevokeSubscriptionToken(ctx, key.Metadata.ID)
					case "traffic storage":
						err = db.WithTx(ctx, func(tx *sql.Tx) error {
							_, err := tx.ExecContext(ctx, "DROP TABLE traffic_months")
							return err
						})
					case "traffic settings":
						err = os.WriteFile(handler.settings.Path(), []byte(`{"traffic":{"quota_gib":-1}}`), 0o600)
					}
					if err != nil {
						t.Fatal(err)
					}
					return "public.example"
				})
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
				usage, err := app.SubscriptionToken(ctx, key.Metadata.ID)
				if err != nil || usage.SuccessfulRequestCount != 1 || usage.BodyResponseCount != 1 || usage.BytesServed != int64(first.Body.Len()) {
					t.Fatalf("failed request changed accounting: %+v, %v", usage, err)
				}
			})
		}
	}
}
