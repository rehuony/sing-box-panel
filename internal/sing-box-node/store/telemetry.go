// SPDX-License-Identifier: GPL-3.0-or-later

package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// InstanceID returns the random identity of this database.
func (s *Store) InstanceID(ctx context.Context) (string, error) {
	var id string
	if err := s.db.QueryRowContext(ctx, `SELECT id FROM node_instance WHERE singleton = 1`).Scan(&id); err != nil {
		return "", fmt.Errorf("read node instance: %w", err)
	}
	return id, nil
}

// RuntimeTransitionsAfter returns up to limit transitions with IDs above
// afterID, oldest first.
func (s *Store) RuntimeTransitionsAfter(ctx context.Context, afterID int64, limit int) ([]RuntimeTransition, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+runtimeTransitionColumns+`
           FROM runtime_transitions WHERE id > ? ORDER BY id ASC LIMIT ?`, afterID, limit)
	if err != nil {
		return nil, fmt.Errorf("list runtime transitions after %d: %w", afterID, err)
	}
	defer rows.Close()
	var items []RuntimeTransition
	for rows.Next() {
		transition, err := scanRuntimeTransition(rows)
		if err != nil {
			return nil, fmt.Errorf("scan runtime transition: %w", err)
		}
		items = append(items, transition)
	}
	return items, rows.Err()
}

// TrafficCheckpoint is the instance's cumulative traffic and the counters of
// the core process it last added from.
type TrafficCheckpoint struct {
	PID                int
	ProcessStartToken  string
	ActivationBundleID string
	// ProcessUpload and ProcessDownload are the process's own counters at
	// SampledAt.
	ProcessUpload   int64
	ProcessDownload int64
	// UploadTotal and DownloadTotal are cumulative for the instance.
	UploadTotal   int64
	DownloadTotal int64
	SampledAt     time.Time
}

// TrafficCheckpoint returns the checkpoint, or false before any core was
// sampled.
func (s *Store) TrafficCheckpoint(ctx context.Context) (TrafficCheckpoint, bool, error) {
	var checkpoint TrafficCheckpoint
	var sampledAt string
	err := s.db.QueryRowContext(ctx, `SELECT pid, process_start_token, activation_bundle_id,
           process_upload, process_download, upload_total, download_total, sampled_at
           FROM traffic_checkpoint WHERE singleton = 1`).Scan(
		&checkpoint.PID, &checkpoint.ProcessStartToken, &checkpoint.ActivationBundleID,
		&checkpoint.ProcessUpload, &checkpoint.ProcessDownload, &checkpoint.UploadTotal, &checkpoint.DownloadTotal, &sampledAt)
	if errors.Is(err, sql.ErrNoRows) {
		return TrafficCheckpoint{}, false, nil
	}
	if err != nil {
		return TrafficCheckpoint{}, false, fmt.Errorf("read traffic checkpoint: %w", err)
	}
	if checkpoint.SampledAt, err = parseTime(sampledAt); err != nil {
		return TrafficCheckpoint{}, false, fmt.Errorf("parse traffic checkpoint time: %w", err)
	}
	return checkpoint, true, nil
}

// SaveTrafficCheckpoint replaces the checkpoint. Cumulative totals never
// decrease.
func (s *Store) SaveTrafficCheckpoint(ctx context.Context, checkpoint TrafficCheckpoint) error {
	result, err := s.db.ExecContext(ctx, `INSERT INTO traffic_checkpoint(
           singleton, pid, process_start_token, activation_bundle_id,
           process_upload, process_download, upload_total, download_total, sampled_at)
         VALUES (1, ?, ?, ?, ?, ?, ?, ?, ?)
         ON CONFLICT(singleton) DO UPDATE SET
           pid = excluded.pid, process_start_token = excluded.process_start_token,
           activation_bundle_id = excluded.activation_bundle_id,
           process_upload = excluded.process_upload, process_download = excluded.process_download,
           upload_total = excluded.upload_total, download_total = excluded.download_total,
           sampled_at = excluded.sampled_at
         WHERE excluded.upload_total >= traffic_checkpoint.upload_total
           AND excluded.download_total >= traffic_checkpoint.download_total`,
		checkpoint.PID, checkpoint.ProcessStartToken, checkpoint.ActivationBundleID,
		checkpoint.ProcessUpload, checkpoint.ProcessDownload, checkpoint.UploadTotal, checkpoint.DownloadTotal,
		formatTime(checkpoint.SampledAt))
	if err != nil {
		return fmt.Errorf("save traffic checkpoint: %w", err)
	}
	if changed, err := result.RowsAffected(); err == nil && changed == 0 {
		return errors.New("save traffic checkpoint: cumulative traffic cannot decrease")
	}
	return nil
}
