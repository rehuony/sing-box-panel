package server

import (
	"context"
	"github.com/rehuony/sing-box-panel/internal/store"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func openRunnerStore(t *testing.T, ctx context.Context) *store.Store {
	t.Helper()
	taskStore, err := store.Open(ctx, filepath.Join(t.TempDir(), "panel.db"))
	if err != nil {
		t.Fatalf("store.Open() error = %v", err)
	}
	t.Cleanup(func() { _ = taskStore.Close() })
	return taskStore
}

func runnerTestContext(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	return ctx
}

func receiveString(t *testing.T, ctx context.Context, values <-chan string) string {
	t.Helper()
	select {
	case value := <-values:
		return value
	case <-ctx.Done():
		t.Fatalf("waiting for value: %v", ctx.Err())
		return ""
	}
}

func waitSignal(t *testing.T, ctx context.Context, signal <-chan struct{}) {
	t.Helper()
	select {
	case <-signal:
	case <-ctx.Done():
		t.Fatalf("waiting for signal: %v", ctx.Err())
	}
}

type fakeClock struct {
	mu      sync.Mutex
	now     time.Time
	tickers map[*fakeTicker]struct{}
}

func newFakeClock(now time.Time) *fakeClock {
	return &fakeClock{now: now.UTC(), tickers: make(map[*fakeTicker]struct{})}
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) NewTicker(interval time.Duration) runtimeTicker {
	c.mu.Lock()
	defer c.mu.Unlock()
	ticker := &fakeTicker{
		clock:    c,
		interval: interval,
		next:     c.now.Add(interval),
		values:   make(chan time.Time, 1),
	}
	c.tickers[ticker] = struct{}{}
	return ticker
}

func (c *fakeClock) Advance(delta time.Duration) {
	c.mu.Lock()
	c.now = c.now.Add(delta)
	for ticker := range c.tickers {
		if ticker.stopped || c.now.Before(ticker.next) {
			continue
		}
		select {
		case ticker.values <- c.now:
		default:
		}
		for !ticker.next.After(c.now) {
			ticker.next = ticker.next.Add(ticker.interval)
		}
	}
	c.mu.Unlock()
}

type fakeTicker struct {
	clock    *fakeClock
	interval time.Duration
	next     time.Time
	values   chan time.Time
	stopped  bool
}

func (t *fakeTicker) C() <-chan time.Time {
	return t.values
}

func (t *fakeTicker) Stop() {
	t.clock.mu.Lock()
	defer t.clock.mu.Unlock()
	if t.stopped {
		return
	}
	t.stopped = true
	delete(t.clock.tickers, t)
}
