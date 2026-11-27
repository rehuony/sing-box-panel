// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"context"
	"strings"
	"time"

	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/app"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/backup"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/store"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/telemetry"
)

// backupCheckInterval is how often the schedule looks whether a backup is
// due; a changed setting takes effect at the next look.
const backupCheckInterval = 10 * time.Minute

// backupStartDelay lets the panel settle before a backup due at startup.
const backupStartDelay = time.Minute

// startBackupSchedule makes a scheduled backup whenever the newest one is
// older than the settings' interval, and keeps only the newest ones, until
// ctx ends. Restarts do not add backups: the schedule follows the newest
// scheduled backup in the directory.
func startBackupSchedule(ctx context.Context, services app.Services) <-chan struct{} {
	done := make(chan struct{})
	go func() {
		defer close(done)
		timer := time.NewTimer(backupStartDelay)
		defer timer.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-timer.C:
			}
			scheduledBackup(ctx, services, time.Now())
			timer.Reset(backupCheckInterval)
		}
	}()
	return done
}

// scheduledBackup makes one scheduled backup when one is due, then removes
// the scheduled backups beyond those to keep.
func scheduledBackup(ctx context.Context, services app.Services, now time.Time) {
	configuration, err := services.Settings.EffectiveSettings(ctx)
	if err != nil || configuration.Backup.IntervalHours == 0 {
		return
	}
	backups := services.Backups
	latest, found, err := backups.LatestScheduled(ctx)
	if err != nil {
		return
	}
	interval := time.Duration(configuration.Backup.IntervalHours) * time.Hour
	if found && now.Sub(latest) < interval {
		return
	}
	startedAt := time.Now()
	info, err := backups.Create(ctx, backup.CreateOptions{Kind: backup.KindScheduled, Categories: backup.DefaultCategories})
	if ctx.Err() != nil {
		return
	}
	services.Telemetry.RecordOperation(ctx, "panel.backup.scheduled", "Scheduled panel backup", err,
		telemetry.OperationLogContext{StartedAt: startedAt, File: info.Name})
	if err != nil {
		return
	}
	if removed, err := backups.PruneScheduled(ctx, configuration.Backup.Keep); err == nil && removed > 0 {
		services.Telemetry.RecordEvent(telemetry.LogRecordRequest{
			Source: store.LogSourcePanel, Level: store.LogLevelInfo,
			Code: "panel.backup.pruned", Message: "Older scheduled backups were removed",
			Metadata: telemetry.LogMetadata(map[string]any{"removed": removed, "kept": configuration.Backup.Keep}),
		})
	}
}

// recordRestore notes in the panel log the restore applied at startup, or
// why a requested one was dropped.
func recordRestore(records *telemetry.Service, applied *backup.Applied) {
	if applied == nil {
		return
	}
	if applied.Skipped != "" {
		records.RecordEvent(telemetry.LogRecordRequest{
			Source: store.LogSourcePanel, Level: store.LogLevelWarn,
			Code: "panel.restore.skipped", Message: "The requested restore was not applied: " + applied.Skipped,
			Metadata: telemetry.LogMetadata(map[string]any{}),
		})
		return
	}
	manifest := applied.Manifest
	categories := make([]string, len(applied.Categories))
	for index, category := range applied.Categories {
		categories[index] = string(category)
	}
	metadata := map[string]any{
		"backup_created_at": manifest.CreatedAt.Format(time.RFC3339), "version": manifest.PanelVersion,
		"categories": strings.Join(categories, ", "), "took_service": applied.TakeService,
	}
	// Only the repairs the restore made to the categories it kept.
	for key, count := range map[string]int{
		"pool_nodes": applied.Effects.PoolNodes, "exit_rules": applied.Effects.ExitRules,
		"rejected_exits": applied.Effects.RejectedExits, "direct_exits": applied.Effects.DirectExits,
		"grants": applied.Effects.Grants, "history_nodes": applied.Effects.HistoryNodes,
	} {
		if count > 0 {
			metadata[key] = count
		}
	}
	records.RecordEvent(telemetry.LogRecordRequest{
		Source: store.LogSourcePanel, Level: store.LogLevelInfo,
		Code: "panel.restore.applied", Message: "The panel was restored from a backup",
		Metadata: telemetry.LogMetadata(metadata),
	})
}
