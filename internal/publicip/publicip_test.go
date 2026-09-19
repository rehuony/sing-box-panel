// SPDX-License-Identifier: GPL-3.0-or-later

package publicip

import (
	"context"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type transportFunc func(*http.Request) (*http.Response, error)

func (f transportFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestDetectionIsBoundedAndCached(t *testing.T) {
	var requests atomic.Int32
	now := time.Now()
	value := "1.1.1.1\n"
	d := New()
	d.now = func() time.Time { return now }
	d.client.Transport = transportFunc(func(r *http.Request) (*http.Response, error) {
		requests.Add(1)
		if r.URL.String() != "https://api.ipify.org" || r.Header.Get("Authorization") != "" {
			t.Errorf("unexpected request %s", r.URL)
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(value)), Header: make(http.Header)}, nil
	})
	var wg sync.WaitGroup
	for range 20 {
		wg.Go(func() {
			if got := d.Resolve(context.Background()); got != "1.1.1.1" {
				t.Errorf("got %q", got)
			}
		})
	}
	wg.Wait()
	if requests.Load() != 1 {
		t.Fatalf("cache misses: %d", requests.Load())
	}
	now = now.Add(6 * time.Minute)
	for _, invalid := range []string{"127.0.0.1", "0.0.0.0", "10.0.0.1", "::1", "100.64.0.1", "203.0.113.1", "https://example.com", strings.Repeat("1", 129)} {
		value = invalid
		if got := d.Resolve(context.Background()); got != "" {
			t.Fatalf("invalid address accepted %q", got)
		}
		before := requests.Load()
		_ = d.Resolve(context.Background())
		if requests.Load() != before {
			t.Fatal("failure not cached")
		}
		now = now.Add(time.Minute)
	}
}
