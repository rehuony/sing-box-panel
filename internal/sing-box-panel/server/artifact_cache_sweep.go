// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"context"
	"time"

	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/fleet"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/store"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/telemetry"
)

// startArtifactCacheSweep removes cached core archives that no node needs,
// once an interval after nodes had time to report what they installed.
func startArtifactCacheSweep(ctx context.Context, nodes *fleet.Service, records *telemetry.Service) <-chan struct{} {
	done := make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(fleet.ArtifactCacheSweepInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
			removed, err := nodes.SweepArtifactCache(ctx)
			switch {
			case err != nil && ctx.Err() == nil:
				records.RecordEvent(telemetry.LogRecordRequest{
					Source: store.LogSourcePanel, Level: store.LogLevelError, Code: "core.cache_sweep_failed",
					Message: "Unused cached core archives could not be removed", Metadata: telemetry.LogMetadata(map[string]any{"removed": removed}),
				})
			case removed > 0:
				records.RecordEvent(telemetry.LogRecordRequest{
					Source: store.LogSourcePanel, Level: store.LogLevelInfo, Code: "core.cache_swept",
					Message: "Cached core archives that no node uses were removed", Metadata: telemetry.LogMetadata(map[string]any{"removed": removed}),
				})
			}
		}
	}()
	return done
}
