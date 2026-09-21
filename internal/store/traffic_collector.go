// SPDX-License-Identifier: GPL-3.0-or-later

package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

type TrafficSampleInput struct {
	ActivationBundleID string
	PID                int
	ProcessStartToken  string
	SampledAt          time.Time
	PeriodStart        time.Time
	PeriodEnd          time.Time
	MemoryBytes        int64
	ActiveConnections  int64
	UploadTotal        int64
	DownloadTotal      int64
}

type TrafficSample struct {
	ID                 int64          `json:"id"`
	ActivationBundleID string         `json:"activation_bundle_id"`
	PID                int            `json:"pid"`
	ProcessStartToken  string         `json:"process_start_token"`
	SampledAt          time.Time      `json:"sampled_at"`
	MemoryBytes        int64          `json:"memory_bytes"`
	ActiveConnections  int64          `json:"active_connections"`
	UploadTotal        int64          `json:"upload_total"`
	DownloadTotal      int64          `json:"download_total"`
	UploadDelta        *int64         `json:"upload_delta"`
	DownloadDelta      *int64         `json:"download_delta"`
	IntervalStart      *time.Time     `json:"interval_start"`
	IntervalEnd        *time.Time     `json:"interval_end"`
	Coverage           CoverageStatus `json:"coverage"`
	Accepted           bool           `json:"accepted"`
	DiagnosticCode     string         `json:"diagnostic_code,omitempty"`
}

type TrafficSampleResult struct {
	Sample TrafficSample `json:"sample"`
	Period TrafficPeriod `json:"period"`
}

type trafficCheckpoint struct {
	PeriodStart, PeriodEnd                 time.Time
	PID                                    int
	ProcessStartToken, ActivationBundleID  string
	LastUpload, LastDownload               int64
	AccumulatedUpload, AccumulatedDownload int64
	HasDelta                               bool
	SampledAt                              time.Time
}

// RecordTrafficSample converts process-local counters into monotonic period
// totals. A PID/start-token change begins a new segment while preserving the
// current UTC period total, but its first lifetime counter is not guessed as a
// period delta. Counter regression or out-of-order evidence is retained as a
// rejected sample and cannot corrupt the checkpoint.
func (s *Store) RecordTrafficSample(ctx context.Context, input TrafficSampleInput) (TrafficSampleResult, error) {
	prepared, err := prepareTrafficSampleInput(input)
	if err != nil {
		return TrafficSampleResult{}, err
	}
	var result TrafficSampleResult
	err = s.WithTx(ctx, func(tx *sql.Tx) error {
		checkpoint, checkpointErr := getTrafficCheckpoint(ctx, tx)
		hasCheckpoint := checkpointErr == nil
		samePeriod := hasCheckpoint && checkpoint.PeriodStart.Equal(prepared.PeriodStart) &&
			checkpoint.PeriodEnd.Equal(prepared.PeriodEnd)
		sameProcess := hasCheckpoint && checkpoint.PID == prepared.PID &&
			checkpoint.ProcessStartToken == prepared.ProcessStartToken
		sameSegment := sameProcess && checkpoint.ActivationBundleID == prepared.ActivationBundleID
		if checkpointErr != nil && !errors.Is(checkpointErr, sql.ErrNoRows) {
			return checkpointErr
		}

		accepted := true
		diagnostic := ""
		if hasCheckpoint && !prepared.SampledAt.After(checkpoint.SampledAt) {
			accepted = false
			diagnostic = "sample_out_of_order"
		} else if sameSegment && (prepared.UploadTotal < checkpoint.LastUpload || prepared.DownloadTotal < checkpoint.LastDownload) {
			accepted = false
			diagnostic = "counter_decreased"
		}
		var uploadDelta, downloadDelta *int64
		var intervalStart, intervalEnd *time.Time
		coverage := CoveragePartial
		if accepted && sameSegment {
			upload := prepared.UploadTotal - checkpoint.LastUpload
			download := prepared.DownloadTotal - checkpoint.LastDownload
			uploadDelta, downloadDelta = &upload, &download
			start, end := checkpoint.SampledAt, prepared.SampledAt
			intervalStart, intervalEnd = &start, &end
			if samePeriod && prepared.SampledAt.Sub(checkpoint.SampledAt) <= maximumCompleteSampleGap {
				coverage = CoverageComplete
			}
		}
		sample, err := insertTrafficSample(
			ctx, tx, prepared, uploadDelta, downloadDelta, intervalStart, intervalEnd,
			coverage, accepted, diagnostic,
		)
		if err != nil {
			return err
		}
		result.Sample = sample
		if !accepted {
			periodID := trafficPeriodID(prepared.PeriodStart, prepared.PeriodEnd)
			period, periodErr := getTrafficPeriod(ctx, tx, periodID)
			if errors.Is(periodErr, ErrTrafficPeriodNotFound) {
				return nil
			}
			result.Period = period
			return periodErr
		}

		accumulatedUpload, accumulatedDownload := int64(0), int64(0)
		if samePeriod {
			accumulatedUpload = checkpoint.AccumulatedUpload
			accumulatedDownload = checkpoint.AccumulatedDownload
		}
		if uploadDelta != nil && samePeriod {
			accumulatedUpload += *uploadDelta
			accumulatedDownload += *downloadDelta
		}
		hasDelta := uploadDelta != nil && samePeriod
		if samePeriod {
			hasDelta = hasDelta || checkpoint.HasDelta
		}
		if err := upsertTrafficCheckpoint(
			ctx, tx, prepared, accumulatedUpload, accumulatedDownload, hasDelta,
		); err != nil {
			return err
		}
		period, err := upsertCollectedTrafficPeriod(
			ctx, tx, prepared, accumulatedUpload, accumulatedDownload,
			uploadDelta, downloadDelta, coverage, hasDelta,
		)
		if err != nil {
			return err
		}
		result.Period = period
		return nil
	})
	return result, err
}

func (s *Store) LatestAcceptedTrafficSample(ctx context.Context) (TrafficSample, error) {
	return scanTrafficSample(s.db.QueryRowContext(ctx, `
		SELECT id, activation_bundle_id, pid, process_start_token, sampled_at,
               memory_bytes, active_connections, upload_total, download_total,
               upload_delta, download_delta, interval_start, interval_end,
               coverage, accepted, diagnostic_code
          FROM traffic_samples WHERE accepted = 1
          ORDER BY sampled_at DESC, id DESC LIMIT 1`))
}

func prepareTrafficSampleInput(input TrafficSampleInput) (TrafficSampleInput, error) {
	if strings.TrimSpace(input.ActivationBundleID) == "" || input.PID <= 0 || !validProcessStartToken(input.ProcessStartToken) {
		return TrafficSampleInput{}, errors.New("traffic sample runtime identity is invalid")
	}
	if input.SampledAt.IsZero() || input.PeriodStart.IsZero() || input.PeriodEnd.IsZero() ||
		!input.PeriodEnd.After(input.PeriodStart) || input.SampledAt.Before(input.PeriodStart) || !input.SampledAt.Before(input.PeriodEnd) {
		return TrafficSampleInput{}, errors.New("traffic sample period is invalid")
	}
	if input.MemoryBytes < 0 || input.ActiveConnections < 0 || input.UploadTotal < 0 || input.DownloadTotal < 0 {
		return TrafficSampleInput{}, errors.New("traffic sample counters are invalid")
	}
	input.SampledAt = input.SampledAt.UTC()
	input.PeriodStart = input.PeriodStart.UTC()
	input.PeriodEnd = input.PeriodEnd.UTC()
	return input, nil
}

func getTrafficCheckpoint(ctx context.Context, tx *sql.Tx) (trafficCheckpoint, error) {
	var checkpoint trafficCheckpoint
	var start, end, sampled string
	err := tx.QueryRowContext(ctx, `
        SELECT period_start, period_end, pid, process_start_token, activation_bundle_id,
               last_upload_total, last_download_total, accumulated_upload,
			   accumulated_download, has_delta, sampled_at
          FROM traffic_checkpoint WHERE singleton = 1`).Scan(
		&start, &end, &checkpoint.PID, &checkpoint.ProcessStartToken, &checkpoint.ActivationBundleID,
		&checkpoint.LastUpload, &checkpoint.LastDownload, &checkpoint.AccumulatedUpload,
		&checkpoint.AccumulatedDownload, &checkpoint.HasDelta, &sampled,
	)
	if err != nil {
		return trafficCheckpoint{}, err
	}
	checkpoint.PeriodStart, err = parseTime(start)
	if err != nil {
		return trafficCheckpoint{}, err
	}
	checkpoint.PeriodEnd, err = parseTime(end)
	if err != nil {
		return trafficCheckpoint{}, err
	}
	checkpoint.SampledAt, err = parseTime(sampled)
	return checkpoint, err
}

func insertTrafficSample(
	ctx context.Context,
	tx *sql.Tx,
	input TrafficSampleInput,
	uploadDelta, downloadDelta *int64,
	intervalStart, intervalEnd *time.Time,
	coverage CoverageStatus,
	accepted bool,
	diagnostic string,
) (TrafficSample, error) {
	result, err := tx.ExecContext(ctx, `
        INSERT INTO traffic_samples(
            activation_bundle_id, pid, process_start_token, sampled_at, memory_bytes,
            active_connections, upload_total, download_total, upload_delta, download_delta,
			interval_start, interval_end, coverage, accepted, diagnostic_code
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		input.ActivationBundleID, input.PID, input.ProcessStartToken, formatTime(input.SampledAt),
		input.MemoryBytes, input.ActiveConnections, input.UploadTotal, input.DownloadTotal,
		uploadDelta, downloadDelta, nullRuntimeTime(intervalStart), nullRuntimeTime(intervalEnd),
		coverage, accepted, diagnostic,
	)
	if err != nil {
		return TrafficSample{}, fmt.Errorf("insert traffic sample: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return TrafficSample{}, err
	}
	return TrafficSample{
		ID: id, ActivationBundleID: input.ActivationBundleID, PID: input.PID,
		ProcessStartToken: input.ProcessStartToken, SampledAt: input.SampledAt,
		MemoryBytes: input.MemoryBytes, ActiveConnections: input.ActiveConnections,
		UploadTotal: input.UploadTotal, DownloadTotal: input.DownloadTotal,
		UploadDelta: uploadDelta, DownloadDelta: downloadDelta,
		IntervalStart: intervalStart, IntervalEnd: intervalEnd, Coverage: coverage,
		Accepted: accepted, DiagnosticCode: diagnostic,
	}, nil
}

func upsertTrafficCheckpoint(
	ctx context.Context,
	tx *sql.Tx,
	input TrafficSampleInput,
	upload, download int64,
	hasDelta bool,
) error {
	_, err := tx.ExecContext(ctx, `
        INSERT INTO traffic_checkpoint(
            singleton, period_start, period_end, pid, process_start_token, activation_bundle_id,
			last_upload_total, last_download_total, accumulated_upload, accumulated_download,
			has_delta, sampled_at
		) VALUES (1, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
        ON CONFLICT(singleton) DO UPDATE SET
            period_start=excluded.period_start, period_end=excluded.period_end,
            pid=excluded.pid, process_start_token=excluded.process_start_token,
            activation_bundle_id=excluded.activation_bundle_id,
            last_upload_total=excluded.last_upload_total, last_download_total=excluded.last_download_total,
            accumulated_upload=excluded.accumulated_upload, accumulated_download=excluded.accumulated_download,
			has_delta=excluded.has_delta, sampled_at=excluded.sampled_at`,
		formatTime(input.PeriodStart), formatTime(input.PeriodEnd), input.PID,
		input.ProcessStartToken, input.ActivationBundleID, input.UploadTotal, input.DownloadTotal,
		upload, download, hasDelta, formatTime(input.SampledAt),
	)
	if err != nil {
		return fmt.Errorf("update traffic checkpoint: %w", err)
	}
	return nil
}

func upsertCollectedTrafficPeriod(
	ctx context.Context,
	tx *sql.Tx,
	input TrafficSampleInput,
	upload, download int64,
	uploadDelta, downloadDelta *int64,
	coverage CoverageStatus,
	hasDelta bool,
) (TrafficPeriod, error) {
	periodID := trafficPeriodID(input.PeriodStart, input.PeriodEnd)
	counters, err := json.Marshal(map[string]any{
		"latest_sample_at": input.SampledAt, "memory_bytes": input.MemoryBytes,
		"active_connections": input.ActiveConnections, "latest_bundle_id": input.ActivationBundleID,
		"upload_total": input.UploadTotal, "download_total": input.DownloadTotal,
		"upload_delta": uploadDelta, "download_delta": downloadDelta, "coverage": coverage,
		"traffic_evidence_available": hasDelta,
	})
	if err != nil {
		return TrafficPeriod{}, err
	}
	_, err = tx.ExecContext(ctx, `
        INSERT INTO traffic_periods(
            id, activation_bundle_id, period_start, period_end, inbound_bytes,
            outbound_bytes, counters_json, created_at
        ) VALUES (?, NULL, ?, ?, ?, ?, ?, ?)
        ON CONFLICT(id) DO UPDATE SET
            inbound_bytes=excluded.inbound_bytes,
            outbound_bytes=excluded.outbound_bytes,
            counters_json=excluded.counters_json`,
		periodID, formatTime(input.PeriodStart), formatTime(input.PeriodEnd),
		download, upload, string(counters), formatTime(input.SampledAt),
	)
	if err != nil {
		return TrafficPeriod{}, fmt.Errorf("update collected traffic period: %w", err)
	}
	return getTrafficPeriod(ctx, tx, periodID)
}

func trafficPeriodID(start, end time.Time) string {
	return "traffic_" + start.UTC().Format("200601") + "_" + end.UTC().Format("200601")
}

func scanTrafficSample(row rowScanner) (TrafficSample, error) {
	var sample TrafficSample
	var sampledAt string
	var uploadDelta, downloadDelta sql.NullInt64
	var intervalStart, intervalEnd sql.NullString
	err := row.Scan(
		&sample.ID, &sample.ActivationBundleID, &sample.PID, &sample.ProcessStartToken,
		&sampledAt, &sample.MemoryBytes, &sample.ActiveConnections, &sample.UploadTotal,
		&sample.DownloadTotal, &uploadDelta, &downloadDelta, &intervalStart, &intervalEnd, &sample.Coverage,
		&sample.Accepted, &sample.DiagnosticCode,
	)
	if err != nil {
		return TrafficSample{}, err
	}
	sample.SampledAt, err = parseTime(sampledAt)
	if uploadDelta.Valid {
		sample.UploadDelta = &uploadDelta.Int64
	}
	if downloadDelta.Valid {
		sample.DownloadDelta = &downloadDelta.Int64
	}
	if intervalStart.Valid {
		parsed, parseErr := parseTime(intervalStart.String)
		if parseErr != nil {
			return TrafficSample{}, fmt.Errorf("parse traffic interval_start: %w", parseErr)
		}
		sample.IntervalStart = &parsed
	}
	if intervalEnd.Valid {
		parsed, parseErr := parseTime(intervalEnd.String)
		if parseErr != nil {
			return TrafficSample{}, fmt.Errorf("parse traffic interval_end: %w", parseErr)
		}
		sample.IntervalEnd = &parsed
	}
	return sample, err
}
