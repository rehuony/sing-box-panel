// SPDX-License-Identifier: GPL-3.0-or-later
package httpapi

import (
	"context"
	"github.com/rehuony/sing-box-panel/internal/application"
	"net/http"
	"time"
)

// A controllable collector for transport tests; production subscriptions are
// shared by Application and have separate concurrency tests.
func streamDashboardSnapshots(w http.ResponseWriter, request *http.Request, load func(context.Context) (application.DashboardStreamSnapshot, error), interval, lifetime time.Duration) {
	streamSnapshots(w, request, "dashboard", lifetime, func(ctx context.Context) (<-chan application.SnapshotResult[application.DashboardStreamSnapshot], func()) {
		ctx, cancel := context.WithCancel(ctx)
		ch := make(chan application.SnapshotResult[application.DashboardStreamSnapshot], 1)
		done := make(chan struct{})
		go func() {
			defer close(done)
			timer := time.NewTimer(0)
			defer timer.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-timer.C:
				}
				value, err := load(ctx)
				select {
				case <-ctx.Done():
					return
				case ch <- application.SnapshotResult[application.DashboardStreamSnapshot]{Value: value, Err: err}:
				}
				timer.Reset(interval)
			}
		}()
		return ch, func() { cancel(); <-done }
	}, encodeDashboardSnapshot)
}
