// SPDX-License-Identifier: GPL-3.0-or-later

package agent

import (
	"context"
	"errors"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/rehuony/sing-box-panel/internal/nodeapi"
	"github.com/rehuony/sing-box-panel/internal/sing-box-node/settings"
)

// linkOf is a link enrolled with enrollment, outside any data directory.
func linkOf(nodeID string) *Link {
	link := &Link{now: time.Now, changed: make(chan struct{})}
	link.setLocked(&settings.Enrollment{URL: "https://panel.example", PanelID: "panel", NodeID: nodeID})
	return link
}

// TestReconnectBackoffGrowsToTheMaximumAndStartsOverAfterAHeldSession fails
// sessions at once until the wait reaches its maximum, then holds one longer
// than that: the next wait starts over.
func TestReconnectBackoffGrowsToTheMaximumAndStartsOverAfterAHeldSession(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		link := linkOf("node")
		enrollment, _, _ := link.current()
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		var starts, ends []time.Time
		link.reconnect(ctx, 0, *enrollment, func(ctx context.Context, _ settings.Enrollment, connected func()) error {
			starts = append(starts, time.Now())
			if len(starts) == 9 {
				connected()
				time.Sleep(maximumBackoff + time.Second)
			}
			if len(starts) == 12 {
				cancel()
			}
			ends = append(ends, time.Now())
			return errors.New("session ended")
		}, func(string, string, ...any) {})
		// Each wait lies in [backoff/2, backoff] for the backoff of its attempt.
		ceilings := []time.Duration{1, 2, 4, 8, 16, 32, 60, 60, 1, 2, 4}
		if len(starts) != len(ceilings)+1 {
			t.Fatalf("%d sessions, want %d", len(starts), len(ceilings)+1)
		}
		for index, ceiling := range ceilings {
			wait, ceiling := starts[index+1].Sub(ends[index]), ceiling*time.Second
			if wait < ceiling/2 || wait > ceiling {
				t.Errorf("wait %d = %s, want between %s and %s", index+1, wait, ceiling/2, ceiling)
			}
		}
	})
}

// replacingPanel keeps one session per node, as the panel does: a new
// session ends the one it replaces with nodeapi.CodeAborted.
type replacingPanel struct {
	mu       sync.Mutex
	current  chan struct{}
	sessions int
}

func (panel *replacingPanel) hold(ctx context.Context, _ settings.Enrollment, connected func()) error {
	panel.mu.Lock()
	if panel.current != nil {
		close(panel.current)
	}
	replaced := make(chan struct{})
	panel.current = replaced
	panel.sessions++
	panel.mu.Unlock()
	connected()
	select {
	case <-replaced:
		return nodeapi.Errorf(nodeapi.CodeAborted, "a newer session for this node replaced this one")
	case <-ctx.Done():
		return ctx.Err()
	}
}

// TestDuplicateInstancesReplaceEachOtherWithoutAStorm runs two instances of
// one node, as a copied data directory would: each replaces the other's
// session, and their backoff keeps the turns far apart.
func TestDuplicateInstancesReplaceEachOtherWithoutAStorm(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		panel := &replacingPanel{}
		ctx, cancel := context.WithCancel(t.Context())
		var instances sync.WaitGroup
		for range 2 {
			link := linkOf("node")
			instances.Go(func() { link.supervise(ctx, panel.hold, func(string, string, ...any) {}) })
		}
		time.Sleep(30 * time.Minute)
		cancel()
		instances.Wait()
		t.Logf("%d sessions in 30 minutes", panel.sessions)
		// Without backoff the two would trade the session every second or
		// so; with it, turns settle to one every half minute or more.
		if panel.sessions > 90 {
			t.Fatalf("%d sessions in 30 minutes, want at most 90", panel.sessions)
		}
		if panel.sessions < 10 {
			t.Fatalf("%d sessions in 30 minutes; the instances stopped reconnecting", panel.sessions)
		}
	})
}
