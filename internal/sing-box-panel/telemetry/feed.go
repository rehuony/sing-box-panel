// SPDX-License-Identifier: GPL-3.0-or-later

package telemetry

import (
	"context"
	"sync"
	"time"
)

// SnapshotResult carries immutable read models. A slow reader always receives
// the newest result, never an unbounded backlog.
type SnapshotResult[T any] struct {
	Value T
	Err   error
}

// Feed shares one periodically refreshed read model among subscribers and
// stops reading when the last subscriber leaves.
type Feed[T any] struct {
	mu          sync.Mutex
	root        context.Context
	load        func(context.Context) (T, error)
	interval    time.Duration
	subscribers map[chan SnapshotResult[T]]struct{}
	cancel      context.CancelFunc
	generation  uint64
	cached      *SnapshotResult[T]
	collected   time.Time
}

// NewFeed returns a feed that loads a value every interval while subscribed.
func NewFeed[T any](interval time.Duration, load func(context.Context) (T, error)) *Feed[T] {
	return &Feed[T]{root: context.Background(), interval: interval, load: load, subscribers: make(map[chan SnapshotResult[T]]struct{})}
}

// Subscribe returns the newest results and a function that unsubscribes.
func (f *Feed[T]) Subscribe() (<-chan SnapshotResult[T], func()) {
	f.mu.Lock()
	ch := make(chan SnapshotResult[T], 1)
	f.subscribers[ch] = struct{}{}
	fresh := f.cached != nil && time.Since(f.collected) < f.interval
	if fresh {
		ch <- *f.cached
	}
	if f.cancel == nil {
		ctx, cancel := context.WithCancel(f.root)
		f.cancel = cancel
		f.generation++
		generation := f.generation
		delay := time.Duration(0)
		if fresh {
			delay = f.interval - time.Since(f.collected)
		}
		go f.run(ctx, generation, delay)
	}
	f.mu.Unlock()
	var once sync.Once
	return ch, func() {
		once.Do(func() {
			f.mu.Lock()
			defer f.mu.Unlock()
			delete(f.subscribers, ch)
			if len(f.subscribers) == 0 {
				f.cancel()
				f.cancel = nil
				f.generation++
			}
		})
	}
}

func (f *Feed[T]) run(ctx context.Context, generation uint64, delay time.Duration) {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		}
		value, err := f.load(ctx)
		if ctx.Err() != nil {
			return
		}
		result := SnapshotResult[T]{Value: value, Err: err}
		f.mu.Lock()
		if generation != f.generation {
			f.mu.Unlock()
			return
		}
		if err == nil {
			f.cached = &result
			f.collected = time.Now()
		}
		for ch := range f.subscribers {
			select {
			case <-ch:
			default:
			}
			ch <- result
		}
		f.mu.Unlock()
		timer.Reset(f.interval)
	}
}

// Latest returns the newest value read within maxAge, if any.
func (f *Feed[T]) Latest(maxAge time.Duration) (T, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.cached == nil || time.Since(f.collected) > maxAge {
		var zero T
		return zero, false
	}
	return f.cached.Value, true
}

// Expire stops the newest value counting as fresh for Latest and for new
// subscribers. A running feed keeps its schedule.
func (f *Feed[T]) Expire() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.collected = time.Time{}
}

// Bind sets the lifetime of shared reads before the first subscriber.
func (f *Feed[T]) Bind(ctx context.Context) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.root = ctx
}
