// SPDX-License-Identifier: GPL-3.0-or-later

package host

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
	d := NewPublicIPDetector()
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

func TestRefreshReplacesTheHintBeforeItExpires(t *testing.T) {
	now := time.Now()
	value, fail := "1.1.1.1", false
	d := NewPublicIPDetector()
	d.now = func() time.Time { return now }
	d.client.Transport = transportFunc(func(*http.Request) (*http.Response, error) {
		if fail {
			return &http.Response{StatusCode: 503, Body: io.NopCloser(strings.NewReader("")), Header: make(http.Header)}, nil
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(value)), Header: make(http.Header)}, nil
	})
	d.resolve(t.Context(), publicIPRefreshInterval)
	// The last refresh before expiry fails: the hint stays until it expires.
	now = now.Add(5*time.Minute - publicIPRefreshInterval/2)
	fail = true
	if got := d.resolve(t.Context(), publicIPRefreshInterval); got != "1.1.1.1" || d.Cached(t.Context()) != "1.1.1.1" {
		t.Fatalf("a failed early refresh dropped the hint: %q", got)
	}
	// The retry succeeds before the hint expires, so Cached never runs dry.
	fail, value = false, "8.8.8.8"
	now = now.Add(publicIPRefreshInterval / 4)
	if got := d.resolve(t.Context(), publicIPRefreshInterval); got != "8.8.8.8" {
		t.Fatal(got)
	}
	now = now.Add(publicIPRefreshInterval)
	if got := d.Cached(t.Context()); got != "8.8.8.8" {
		t.Fatalf("hint expired between refreshes: %q", got)
	}
}

func TestCachedDoesNotWaitForPublicNetwork(t *testing.T) {
	d := NewPublicIPDetector()
	entered, release := make(chan struct{}), make(chan struct{})
	d.client.Transport = transportFunc(func(r *http.Request) (*http.Response, error) {
		close(entered)
		select {
		case <-release:
		case <-r.Context().Done():
			return nil, r.Context().Err()
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("1.1.1.1")), Header: make(http.Header)}, nil
	})
	done := make(chan struct{})
	go func() { defer close(done); d.Resolve(t.Context()) }()
	<-entered
	cached := make(chan string, 1)
	go func() { cached <- d.Cached(t.Context()) }()
	select {
	case value := <-cached:
		if value != "" {
			t.Fatal(value)
		}
	case <-time.After(time.Second):
		close(release)
		<-done
		t.Fatal("management cache blocked behind detection")
	}
	close(release)
	<-done
	if got := d.Cached(t.Context()); got != "1.1.1.1" {
		t.Fatal(got)
	}
}
