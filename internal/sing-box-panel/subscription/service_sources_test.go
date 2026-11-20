// SPDX-License-Identifier: GPL-3.0-or-later

package subscription

import (
	"encoding/base64"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/settings"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/store"
)

func TestURLSourceRefreshKeepsLastContentThatParsed(t *testing.T) {
	ctx := t.Context()
	var mu sync.Mutex
	status, body := http.StatusOK, base64.StdEncoding.EncodeToString([]byte("socks5://a.example:1080#A\nsocks5://b.example:1080#B\n"))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(server.Close)
	serve := func(nextStatus int, nextBody string) {
		mu.Lock()
		defer mu.Unlock()
		status, body = nextStatus, nextBody
	}
	db, app := newPoolTestApplication(t)
	clock := time.Date(2026, time.August, 26, 12, 0, 0, 0, time.UTC)
	app.now = func() time.Time { return clock }
	value := settings.Defaults()
	value.Subscription.PrivateSourceCIDRs = []string{"127.0.0.1/32"}
	app.useSettings(testSettings(db, "", value))

	// Content that does not parse creates no source.
	serve(http.StatusOK, "<html>maintenance</html>")
	_, err := app.CreateSubscriptionSource(ctx, CreateSubscriptionSourceRequest{
		Name: "remote", Kind: store.SubscriptionSourceURL, URL: &server.URL, RefreshMinutes: new(15), Filter: &SourceFilter{},
	})
	var parseErr *SourceParseError
	if !errors.As(err, &parseErr) || parseErr.Code != SourceErrorUnrecognizedFormat {
		t.Fatalf("create from unparsable content = %v", err)
	}
	if sources, err := app.ListSubscriptionSources(ctx, SubscriptionListRequest{}); err != nil || len(sources.Items) != 0 {
		t.Fatalf("a failed create saved a source: %+v %v", sources, err)
	}

	serve(http.StatusOK, base64.StdEncoding.EncodeToString([]byte("socks5://a.example:1080#A\nsocks5://b.example:1080#B\n")))
	source, err := app.CreateSubscriptionSource(ctx, CreateSubscriptionSourceRequest{
		Name: "remote", Kind: store.SubscriptionSourceURL, URL: &server.URL, RefreshMinutes: new(15), Filter: &SourceFilter{},
	})
	if err != nil {
		t.Fatal(err)
	}
	if source.NodeCount != 2 || source.ContentFormat != SourceFormatShareLinks || *source.RefreshMinutes != 15 ||
		!source.NextRefreshAt.Equal(clock.Add(15*time.Minute)) || source.LastError != "" {
		t.Fatalf("created source = %+v", source)
	}

	// Failed fetches and parses keep the content and nodes and record why.
	clock = clock.Add(time.Minute)
	for _, failure := range []struct {
		status int
		body   string
		code   string
	}{
		{http.StatusBadGateway, "", "http_status_502"},
		{http.StatusOK, "socks5://a.example:1080#A\ntrojan://t.example:443#Broken\n", SourceErrorNodeNotConvertible},
	} {
		serve(failure.status, failure.body)
		if _, err := app.RefreshSubscriptionSource(ctx, source.ID); err == nil {
			t.Fatalf("refresh with %d %q succeeded", failure.status, failure.body)
		}
		current, err := app.SubscriptionSource(ctx, source.ID)
		names, _ := poolSummary(t, app)
		if err != nil || current.LastError != failure.code || !current.LastAttemptAt.Equal(clock) || !current.NextRefreshAt.Equal(clock.Add(15*time.Minute)) ||
			!current.ContentAt.Equal(source.ContentAt) || !current.UpdatedAt.Equal(source.UpdatedAt) || !slices.Equal(names, []string{"A", "B"}) {
			t.Fatalf("after failure %s: source = %+v, pool = %v, %v", failure.code, current, names, err)
		}
	}

	// A due source refreshes on schedule, and success clears the error.
	serve(http.StatusOK, "socks5://b.example:1080#B\nsocks5://c.example:1080#C\n")
	if err := app.RefreshDueSubscriptionSources(ctx); err != nil {
		t.Fatal(err)
	}
	if names, _ := poolSummary(t, app); !slices.Equal(names, []string{"A", "B"}) {
		t.Fatalf("a source refreshed before it was due: %v", names)
	}
	clock = clock.Add(15 * time.Minute)
	if err := app.RefreshDueSubscriptionSources(ctx); err != nil {
		t.Fatal(err)
	}
	current, err := app.SubscriptionSource(ctx, source.ID)
	names, _ := poolSummary(t, app)
	if err != nil || current.LastError != "" || !current.ContentAt.Equal(clock) || !current.NextRefreshAt.Equal(clock.Add(15*time.Minute)) ||
		!slices.Equal(names, []string{"B", "C"}) {
		t.Fatalf("after the scheduled refresh: source = %+v, pool = %v, %v", current, names, err)
	}

	// Only fetching on request leaves nothing scheduled.
	updated, err := app.UpdateSubscriptionSource(ctx, source.ID, UpdateSubscriptionSourceRequest{
		Name: "remote", URL: &server.URL, RefreshMinutes: new(0), Filter: &SourceFilter{},
	}, current.UpdatedAt)
	if err != nil || updated.NextRefreshAt != nil || *updated.RefreshMinutes != 0 {
		t.Fatalf("manual-only source = %+v, %v", updated, err)
	}
	if _, err := app.UpdateSubscriptionSource(ctx, source.ID, UpdateSubscriptionSourceRequest{
		Name: "remote", URL: &server.URL, RefreshMinutes: new(0), Filter: &SourceFilter{}, Content: new("socks5://d.example:1080#D"),
	}, updated.UpdatedAt); !errors.Is(err, ErrSourceKindMismatch) {
		t.Fatalf("a URL source took uploaded content: %v", err)
	}
}

func TestSourceFormatNamesALinkProtocol(t *testing.T) {
	ctx := t.Context()
	_, app := newPoolTestApplication(t)
	if ValidSourceFormat(SourceFormatShareLinks, true) || !ValidSourceFormat("vless", true) {
		t.Fatal("a source names one link protocol, and links of several are read with auto")
	}
	content := "socks5://a.example:1080#A\nsocks5://b.example:1080#B"
	source, err := app.CreateSubscriptionSource(ctx, CreateSubscriptionSourceRequest{
		Name: "socks", Kind: store.SubscriptionSourceFile, Format: "socks", Filter: &SourceFilter{}, Content: &content,
	})
	if err != nil || source.NodeCount != 2 || source.Format != "socks" || source.ContentFormat != SourceFormatShareLinks {
		t.Fatalf("socks source = %+v, %v", source, err)
	}
	// Every protocol links convert to can be stored as a source's format.
	for _, protocol := range shareLinkProtocols {
		source, err = app.UpdateSubscriptionSource(ctx, source.ID, UpdateSubscriptionSourceRequest{
			Name: "socks", Format: SourceFormat(protocol), Filter: &SourceFilter{},
		}, source.UpdatedAt)
		if err != nil || source.Format != SourceFormat(protocol) {
			t.Fatalf("format %s = %+v, %v", protocol, source, err)
		}
	}
}
