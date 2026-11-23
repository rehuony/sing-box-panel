// SPDX-License-Identifier: GPL-3.0-or-later
package httpapi

import (
	"context"
	"net/http"
	"time"

	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/telemetry"
)

// testSnapshotInterval paces the controllable collector of transport tests.
const testSnapshotInterval = 30 * time.Second

// A controllable collector for transport tests; production subscriptions are
// shared by Application and have separate concurrency tests.
func streamDashboardSnapshots(w http.ResponseWriter, request *http.Request, load func(context.Context) (telemetry.DashboardStreamSnapshot, error), interval, lifetime time.Duration) {
	streamSnapshots(w, request, "dashboard", lifetime, maximumSnapshotFrameBytes, func(ctx context.Context) (<-chan telemetry.SnapshotResult[telemetry.DashboardStreamSnapshot], func()) {
		ctx, cancel := context.WithCancel(ctx)
		ch := make(chan telemetry.SnapshotResult[telemetry.DashboardStreamSnapshot], 1)
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
				case ch <- telemetry.SnapshotResult[telemetry.DashboardStreamSnapshot]{Value: value, Err: err}:
				}
				timer.Reset(interval)
			}
		}()
		return ch, func() { cancel(); <-done }
	}, encodeDashboardSnapshot)
}
