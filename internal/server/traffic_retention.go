// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"context"
	"encoding/json"
	"time"

	"github.com/rehuony/sing-box-panel/internal/application"
	"github.com/rehuony/sing-box-panel/internal/store"
)

func startTrafficSampleRetention(ctx context.Context, commands *application.Application) <-chan struct{} {
	done := make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(24 * time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				result, err := commands.EnforceTrafficSampleRetention(ctx)
				if err != nil {
					recordOperationalLog(commands, application.LogRecordRequest{
						Source: store.LogSourcePanel, Level: store.LogLevelError,
						Code: "traffic.retention_failed", Message: "Traffic sample retention failed",
						Metadata: json.RawMessage(`{}`),
					})
					continue
				}
				if result.Deleted > 0 {
					recordOperationalLog(commands, application.LogRecordRequest{
						Source: store.LogSourcePanel, Level: store.LogLevelInfo,
						Code: "traffic.retention_enforced", Message: "Expired raw traffic samples were deleted",
						Metadata: mustLogMetadata(map[string]any{
							"deleted": result.Deleted, "cutoff": result.Cutoff,
						}),
					})
				}
			}
		}
	}()
	return done
}
