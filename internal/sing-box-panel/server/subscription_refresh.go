// SPDX-License-Identifier: GPL-3.0-or-later
package server

import (
	"context"
	"time"

	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/subscription"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/telemetry"
)

// startSubscriptionRefresh refreshes due sources every minute and, between
// those rounds, serves requested pool synchronizations, such as after a fleet
// node's report, one at a time.
func startSubscriptionRefresh(ctx context.Context, subscriptions *subscription.Service, records *telemetry.Service) <-chan struct{} {
	done := make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		for {
			if err := subscriptions.RefreshDueSubscriptionSources(ctx); err != nil && ctx.Err() == nil {
				records.RecordOperation(ctx, "subscription.refresh", "Automatic subscription refresh", err, telemetry.OperationLogContext{})
			}
			for waiting := true; waiting; {
				select {
				case <-ctx.Done():
					return
				case <-subscriptions.PoolSyncRequests():
					subscriptions.PoolInputsChanged(ctx)
				case <-ticker.C:
					waiting = false
				}
			}
		}
	}()
	return done
}
