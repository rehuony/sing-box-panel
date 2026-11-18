// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"context"
	"time"

	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/app"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/store"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/telemetry"
)

// enforceRetention runs one retention pass: expired traffic samples, log
// entries and runtime history, then old configuration revisions and source
// versions. It records what it deleted, and records and returns a failure.
func enforceRetention(ctx context.Context, services app.Services) error {
	records := services.Telemetry
	deleted, err := records.EnforceRetention(ctx)
	var revisions, sourceVersions int64
	if err == nil {
		revisions, err = services.Configs.PruneRevisions(ctx)
	}
	if err == nil {
		sourceVersions, err = services.Subscriptions.PruneSourceVersions(ctx)
	}
	if err != nil {
		if ctx.Err() == nil {
			records.RecordEvent(telemetry.LogRecordRequest{
				Source: store.LogSourcePanel, Level: store.LogLevelError,
				Code: "records.retention_failed", Message: "Expired records could not be deleted",
				Metadata: telemetry.LogMetadata(map[string]any{}),
			})
		}
		return err
	}
	if deleted.TrafficSamples+deleted.LogEntries+deleted.RuntimeTransitions+revisions+sourceVersions > 0 {
		records.RecordEvent(telemetry.LogRecordRequest{
			Source: store.LogSourcePanel, Level: store.LogLevelInfo,
			Code: "records.retention_enforced", Message: "Expired records were deleted",
			Metadata: telemetry.LogMetadata(map[string]any{
				"traffic_samples": deleted.TrafficSamples, "log_entries": deleted.LogEntries,
				"runtime_transitions": deleted.RuntimeTransitions, "configuration_revisions": revisions,
				"source_versions": sourceVersions,
			}),
		})
	}
	return nil
}

// startRetention runs a retention pass every day and whenever the panel
// settings change, until ctx ends.
func startRetention(ctx context.Context, services app.Services) <-chan struct{} {
	changes, unsubscribe := services.Settings.SettingsChanges()
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer unsubscribe()
		ticker := time.NewTicker(24 * time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			case <-changes:
			}
			_ = enforceRetention(ctx, services)
		}
	}()
	return done
}
