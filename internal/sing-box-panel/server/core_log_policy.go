// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"context"

	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/app"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/store"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/telemetry"
)

// startCoreLogPolicy sends the connected nodes the core log retention after
// each settings change, until ctx ends. A node that connects receives it in
// Welcome.
func startCoreLogPolicy(ctx context.Context, services app.Services) <-chan struct{} {
	changes, unsubscribe := services.Settings.SettingsChanges()
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer unsubscribe()
		for {
			select {
			case <-ctx.Done():
				return
			case <-changes:
			}
			if err := services.Fleet.SyncCoreLogPolicy(ctx); err != nil && ctx.Err() == nil {
				services.Telemetry.RecordEvent(telemetry.LogRecordRequest{
					Source: store.LogSourcePanel, Level: store.LogLevelError,
					Code: "core.log.policy_sync_failed", Message: "The core log retention could not be sent to the nodes",
					Metadata: telemetry.LogMetadata(map[string]any{"error": err.Error()}),
				})
			}
		}
	}()
	return done
}
