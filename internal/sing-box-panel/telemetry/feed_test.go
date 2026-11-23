// SPDX-License-Identifier: GPL-3.0-or-later
package telemetry

import (
	"context"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"
)

func TestSnapshotFeedSharesLatestAndStopsWithoutSubscribers(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var calls atomic.Int32
		feed := NewFeed(2*time.Second, func(context.Context) (int32, error) { return calls.Add(1), nil })
		fast, releaseFast := feed.Subscribe()
		slow, releaseSlow := feed.Subscribe()
		synctest.Wait()
		if got := <-fast; got.Value != 1 {
			t.Fatal(got)
		}
		if value, err := feed.Read(t.Context(), 2*time.Second); err != nil || value != 1 || calls.Load() != 1 {
			t.Fatalf("read = %d, %v after %d loads; want the shared read", value, err, calls.Load())
		}
		time.Sleep(6 * time.Second)
		synctest.Wait()
		if calls.Load() != 4 {
			t.Fatalf("loads=%d, want one per interval for both readers", calls.Load())
		}
		if got := <-slow; got.Value != 4 {
			t.Fatalf("slow reader received backlog: %+v", got)
		}
		releaseFast()
		releaseSlow()
		time.Sleep(2 * time.Second)
		synctest.Wait()
		if calls.Load() != 4 {
			t.Fatal("collector continued without subscribers")
		}
		if value, err := feed.Read(t.Context(), time.Second); err != nil || value != 5 {
			t.Fatalf("read = %d, %v; want a new read instead of one older than the allowed age", value, err)
		}
		if value, _ := feed.Read(t.Context(), time.Second); value != 5 || calls.Load() != 5 {
			t.Fatal("a direct read was not shared with the next reader")
		}
	})
}

func TestSnapshotFeedReadBeforeExpireIsNotFresh(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var calls atomic.Int32
		loading := make(chan struct{})
		release := make(chan struct{})
		feed := NewFeed(time.Minute, func(context.Context) (int32, error) {
			call := calls.Add(1)
			if call == 1 {
				close(loading)
				<-release
			}
			return call, nil
		})
		results, unsubscribe := feed.Subscribe()
		defer unsubscribe()
		<-loading
		// A write lands while the shared read runs.
		feed.Expire()
		close(release)
		synctest.Wait()
		if value, err := feed.Read(t.Context(), time.Minute); err != nil || value != 2 {
			t.Fatalf("read = %d, %v; want a read that follows the expiring write", value, err)
		}
		if got := <-results; got.Value != 2 {
			t.Fatalf("subscriber kept the read from before the write: %+v", got)
		}
	})
}

func TestSnapshotFeedCancellationAndCachedRotation(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		root, cancel := context.WithCancel(context.Background())
		defer cancel()
		var calls atomic.Int32
		feed := NewFeed(30*time.Second, func(context.Context) (int32, error) { return calls.Add(1), nil })
		feed.root = root
		first, release := feed.Subscribe()
		synctest.Wait()
		<-first
		release()
		second, releaseSecond := feed.Subscribe()
		defer releaseSecond()
		if got := <-second; got.Value != 1 {
			t.Fatal(got)
		}
		synctest.Wait()
		if calls.Load() != 1 {
			t.Fatal("normal rotation recollected a fresh snapshot")
		}
		cancel()
		time.Sleep(time.Minute)
		synctest.Wait()
		if calls.Load() != 1 {
			t.Fatal("collector escaped server lifetime")
		}
	})
}

func TestSnapshotFeedCancelsPendingReadOnLastRelease(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		stopped := make(chan struct{})
		feed := NewFeed(time.Second, func(ctx context.Context) (int, error) { <-ctx.Done(); close(stopped); return 0, ctx.Err() })
		_, release := feed.Subscribe()
		synctest.Wait()
		release()
		<-stopped
	})
}
