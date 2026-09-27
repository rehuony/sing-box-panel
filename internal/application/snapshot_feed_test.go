// SPDX-License-Identifier: GPL-3.0-or-later
package application

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
		feed := newSnapshotFeed(2*time.Second, func(context.Context) (int32, error) { return calls.Add(1), nil })
		fast, releaseFast := feed.subscribe()
		slow, releaseSlow := feed.subscribe()
		synctest.Wait()
		if got := <-fast; got.Value != 1 {
			t.Fatal(got)
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
	})
}

func TestSnapshotFeedCancellationAndCachedRotation(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		root, cancel := context.WithCancel(context.Background())
		defer cancel()
		var calls atomic.Int32
		feed := newSnapshotFeed(30*time.Second, func(context.Context) (int32, error) { return calls.Add(1), nil })
		feed.root = root
		first, release := feed.subscribe()
		synctest.Wait()
		<-first
		release()
		second, releaseSecond := feed.subscribe()
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
		feed := newSnapshotFeed(time.Second, func(ctx context.Context) (int, error) { <-ctx.Done(); close(stopped); return 0, ctx.Err() })
		_, release := feed.subscribe()
		synctest.Wait()
		release()
		<-stopped
	})
}
