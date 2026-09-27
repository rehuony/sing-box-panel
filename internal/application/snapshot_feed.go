// SPDX-License-Identifier: GPL-3.0-or-later

package application

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

type snapshotFeed[T any] struct {
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

func newSnapshotFeed[T any](interval time.Duration, load func(context.Context) (T, error)) *snapshotFeed[T] {
	return &snapshotFeed[T]{root: context.Background(), interval: interval, load: load, subscribers: make(map[chan SnapshotResult[T]]struct{})}
}

func (f *snapshotFeed[T]) subscribe() (<-chan SnapshotResult[T], func()) {
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

func (f *snapshotFeed[T]) run(ctx context.Context, generation uint64, delay time.Duration) {
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
