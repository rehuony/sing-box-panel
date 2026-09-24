// SPDX-License-Identifier: GPL-3.0-or-later
package server

import (
	"context"
	"github.com/rehuony/sing-box-panel/internal/application"
	"time"
)

func startSubscriptionRefresh(ctx context.Context, commands *application.Application) <-chan struct{} {
	done := make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		for {
			if err := commands.RefreshDueSubscriptionSources(ctx); err != nil && ctx.Err() == nil {
				commands.RecordOperation(ctx, "subscription.refresh", "Automatic subscription refresh", err, application.OperationLogContext{})
			}
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
	return done
}
