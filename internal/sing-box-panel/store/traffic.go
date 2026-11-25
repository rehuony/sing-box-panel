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

var ErrTrafficPeriodNotFound = errors.New("traffic period not found")

// TrafficPeriod is a node's recorded traffic in one period, or the fleet's
// when it aggregates every node.
type TrafficPeriod struct {
	ID                 string          `json:"id"`
	ActivationBundleID string          `json:"activation_bundle_id,omitempty"`
	PeriodStart        time.Time       `json:"period_start"`
	PeriodEnd          time.Time       `json:"period_end"`
	InboundBytes       int64           `json:"inbound_bytes"`
	OutboundBytes      int64           `json:"outbound_bytes"`
	Counters           json.RawMessage `json:"counters,omitempty"`
	CreatedAt          time.Time       `json:"created_at"`
}

// TrafficPeriodFilter selects a newest-first page of one node's periods.
type TrafficPeriodFilter struct {
	NodeID        string
	OverlapsStart *time.Time
	OverlapsEnd   *time.Time
	Cursor        *TrafficPeriodCursor
	Limit         int
}

// TrafficPeriodCursor is an exclusive cursor in the stable newest-first
// (period_start, id) ordering.
type TrafficPeriodCursor struct {
	PeriodStart time.Time `json:"period_start"`
	ID          string    `json:"id"`
}

type TrafficPeriodPage struct {
	Items []TrafficPeriod      `json:"items"`
	Next  *TrafficPeriodCursor `json:"next,omitempty"`
}

// TrafficSampleInput is one traffic reading a node reported. Its byte totals
// are cumulative for the node instance.
type TrafficSampleInput struct {
	NodeID             string
	InstanceID         string
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
	InstanceID               string
	LastUpload, LastDownload int64
	SampledAt                time.Time
}

// RecordTrafficSample turns a node's cumulative counters into period totals.
// The period is the node's period that contains the sample.
// A delta is proven only from the node's last accepted sample of the same
// instance, so a recreated node database starts a new segment whose first
// counters are not guessed as traffic. Out-of-order evidence is kept as a
// rejected sample and cannot corrupt the checkpoint. Counters that decrease
// within one instance start a new baseline, like a new instance: the sample
// proves no traffic, and later samples prove theirs from it instead of being
// refused until the counters pass their old totals.
func (s *Store) RecordTrafficSample(ctx context.Context, input TrafficSampleInput) (TrafficSampleResult, error) {
	prepared, err := prepareTrafficSampleInput(input)
	if err != nil {
		return TrafficSampleResult{}, err
	}
	var result TrafficSampleResult
	err = s.WithTx(ctx, func(tx *sql.Tx) error {
		checkpoint, checkpointErr := getTrafficCheckpoint(ctx, tx, prepared.NodeID)
		if checkpointErr != nil && !errors.Is(checkpointErr, sql.ErrNoRows) {
			return checkpointErr
		}
		hasCheckpoint := checkpointErr == nil
		sameSegment := hasCheckpoint && checkpoint.InstanceID == prepared.InstanceID

		accepted := true
		diagnostic := ""
		if hasCheckpoint && !prepared.SampledAt.After(checkpoint.SampledAt) {
			accepted = false
			diagnostic = "sample_out_of_order"
		} else if sameSegment && (prepared.UploadTotal < checkpoint.LastUpload || prepared.DownloadTotal < checkpoint.LastDownload) {
			sameSegment = false
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
			if dayStart(prepared.SampledAt).Equal(dayStart(checkpoint.SampledAt)) && prepared.SampledAt.Sub(checkpoint.SampledAt) <= maximumCompleteSampleGap {
				coverage = CoverageComplete
			}
		}
		sample, err := insertTrafficSample(ctx, tx, prepared, uploadDelta, downloadDelta, intervalStart, intervalEnd, coverage, accepted, diagnostic)
		if err != nil {
			return err
		}
		result.Sample = sample
		if accepted {
			if err := recordTrafficDay(ctx, tx, prepared.NodeID, sample); err != nil {
				return err
			}
			if err := upsertTrafficCheckpoint(ctx, tx, prepared); err != nil {
				return err
			}
			if err := recordTrafficPeriod(ctx, tx, prepared); err != nil {
				return err
			}
		}
		result.Period, err = aggregateTrafficPeriod(ctx, tx, prepared.NodeID, prepared.PeriodStart, prepared.PeriodEnd, prepared.SampledAt)
		return err
	})
	return result, err
}

// LatestAcceptedTrafficSample returns the node's newest accepted sample.
func (s *Store) LatestAcceptedTrafficSample(ctx context.Context, nodeID string) (TrafficSample, error) {
	return scanTrafficSample(s.db.QueryRowContext(ctx, `
		SELECT id, activation_bundle_id, pid, process_start_token, sampled_at,
               memory_bytes, active_connections, upload_total, download_total,
               upload_delta, download_delta, interval_start, interval_end,
               coverage, accepted, diagnostic_code
          FROM traffic_samples WHERE node_id = ? AND accepted = 1
          ORDER BY sampled_at DESC, id DESC LIMIT 1`, nodeID))
}

// AcceptedTrafficSamplesSince returns the node's accepted samples taken at or
// after since, oldest first.
func (s *Store) AcceptedTrafficSamplesSince(ctx context.Context, nodeID string, since time.Time) ([]TrafficSample, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, activation_bundle_id, pid, process_start_token, sampled_at,
               memory_bytes, active_connections, upload_total, download_total,
               upload_delta, download_delta, interval_start, interval_end,
               coverage, accepted, diagnostic_code
          FROM traffic_samples WHERE node_id = ? AND sampled_at >= ? AND accepted = 1
          ORDER BY sampled_at ASC, id ASC`, nodeID, formatTime(since))
	if err != nil {
		return nil, fmt.Errorf("query traffic samples: %w", err)
	}
	defer rows.Close()
	samples := []TrafficSample{}
	for rows.Next() {
		sample, err := scanTrafficSample(rows)
		if err != nil {
			return nil, fmt.Errorf("scan traffic sample: %w", err)
		}
		samples = append(samples, sample)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate traffic samples: %w", err)
	}
	return samples, nil
}

// AggregateTrafficPeriod returns the node's recorded traffic in a range of
// whole UTC days, or every node's when nodeID is empty.
func (s *Store) AggregateTrafficPeriod(ctx context.Context, nodeID string, start, end, at time.Time) (TrafficPeriod, error) {
	return aggregateTrafficPeriod(ctx, s.db, nodeID, start, end, at)
}

// GetTrafficPeriod returns one of the node's recorded periods with its
// current totals.
func (s *Store) GetTrafficPeriod(ctx context.Context, nodeID, periodID string) (TrafficPeriod, error) {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return TrafficPeriod{}, err
	}
	defer tx.Rollback()
	var start, end string
	err = tx.QueryRowContext(ctx, `SELECT period_start, period_end FROM traffic_periods WHERE node_id = ? AND id = ?`,
		nodeID, periodID).Scan(&start, &end)
	if errors.Is(err, sql.ErrNoRows) {
		return TrafficPeriod{}, fmt.Errorf("%w: %s", ErrTrafficPeriodNotFound, periodID)
	}
	if err != nil {
		return TrafficPeriod{}, fmt.Errorf("get traffic period: %w", err)
	}
	period, err := periodTotals(ctx, tx, nodeID, start, end)
	if err != nil {
		return TrafficPeriod{}, err
	}
	return period, tx.Commit()
}

// ListTrafficPeriodPage returns the node's recorded periods, newest first.
func (s *Store) ListTrafficPeriodPage(ctx context.Context, filter TrafficPeriodFilter) (TrafficPeriodPage, error) {
	limit, err := normalizePageLimit(filter.Limit)
	if err != nil {
		return TrafficPeriodPage{}, err
	}
	if err := validateTrafficPeriodCursor(filter.Cursor); err != nil {
		return TrafficPeriodPage{}, err
	}
	clauses := []string{"node_id = ?"}
	args := []any{filter.NodeID}
	if filter.OverlapsStart != nil {
		clauses = append(clauses, "period_end > ?")
		args = append(args, formatTime(filter.OverlapsStart.UTC()))
	}
	if filter.OverlapsEnd != nil {
		clauses = append(clauses, "period_start < ?")
		args = append(args, formatTime(filter.OverlapsEnd.UTC()))
	}
	if filter.Cursor != nil {
		cursorTime := formatTime(filter.Cursor.PeriodStart.UTC())
		clauses = append(clauses, "(period_start < ? OR (period_start = ? AND id < ?))")
		args = append(args, cursorTime, cursorTime, filter.Cursor.ID)
	}
	args = append(args, limit+1)
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return TrafficPeriodPage{}, err
	}
	defer tx.Rollback()
	rows, err := tx.QueryContext(ctx, `SELECT period_start, period_end FROM traffic_periods
          WHERE `+strings.Join(clauses, " AND ")+`
          ORDER BY period_start DESC, id DESC LIMIT ?`, args...)
	if err != nil {
		return TrafficPeriodPage{}, fmt.Errorf("list traffic periods: %w", err)
	}
	var ranges [][2]string
	for rows.Next() {
		var start, end string
		if err := rows.Scan(&start, &end); err != nil {
			_ = rows.Close()
			return TrafficPeriodPage{}, fmt.Errorf("scan traffic period: %w", err)
		}
		ranges = append(ranges, [2]string{start, end})
	}
	if err := errors.Join(rows.Err(), rows.Close()); err != nil {
		return TrafficPeriodPage{}, fmt.Errorf("iterate traffic periods: %w", err)
	}
	periods := make([]TrafficPeriod, 0, len(ranges))
	for _, value := range ranges {
		period, err := periodTotals(ctx, tx, filter.NodeID, value[0], value[1])
		if err != nil {
			return TrafficPeriodPage{}, err
		}
		periods = append(periods, period)
	}
	page := TrafficPeriodPage{Items: periods}
	if len(periods) > limit {
		page.Items = periods[:limit]
		last := page.Items[len(page.Items)-1]
		page.Next = &TrafficPeriodCursor{PeriodStart: last.PeriodStart, ID: last.ID}
	}
	return page, tx.Commit()
}

func periodTotals(ctx context.Context, q queryRower, nodeID, rawStart, rawEnd string) (TrafficPeriod, error) {
	start, err := parseTime(rawStart)
	if err != nil {
		return TrafficPeriod{}, fmt.Errorf("parse period_start: %w", err)
	}
	end, err := parseTime(rawEnd)
	if err != nil {
		return TrafficPeriod{}, fmt.Errorf("parse period_end: %w", err)
	}
	return aggregateTrafficPeriod(ctx, q, nodeID, start, end, time.Now().UTC())
}

func validateTrafficPeriodCursor(cursor *TrafficPeriodCursor) error {
	if cursor == nil {
		return nil
	}
	if cursor.PeriodStart.IsZero() || strings.TrimSpace(cursor.ID) == "" || strings.TrimSpace(cursor.ID) != cursor.ID {
		return invalidQuery("traffic period cursor is invalid")
	}
	return nil
}

func dayStart(at time.Time) time.Time {
	at = at.UTC()
	return time.Date(at.Year(), at.Month(), at.Day(), 0, 0, 0, 0, time.UTC)
}

// recordTrafficDay books each accepted delta in the node's UTC day of
// observation. This ledger does not use the sampling quality metadata that
// historical charts need.
func recordTrafficDay(ctx context.Context, tx *sql.Tx, nodeID string, sample TrafficSample) error {
	upload, download := int64(0), int64(0)
	if sample.UploadDelta != nil && sample.DownloadDelta != nil {
		upload, download = *sample.UploadDelta, *sample.DownloadDelta
	}
	_, err := tx.ExecContext(ctx, `INSERT INTO traffic_days
        (node_id,day_start,inbound_bytes,outbound_bytes,first_observed_at,last_sample_at)
        VALUES(?,?,?,?,?,?)
        ON CONFLICT(node_id,day_start) DO UPDATE SET
          inbound_bytes=traffic_days.inbound_bytes+excluded.inbound_bytes,
          outbound_bytes=traffic_days.outbound_bytes+excluded.outbound_bytes,
          last_sample_at=excluded.last_sample_at`,
		nodeID, formatTime(dayStart(sample.SampledAt)), download, upload, formatTime(sample.SampledAt), formatTime(sample.SampledAt))
	return err
}

// aggregateTrafficPeriod returns recorded usage, including zero for an empty
// range. It never depends on the availability of a running core or raw
// samples. An empty nodeID aggregates every node.
func aggregateTrafficPeriod(ctx context.Context, q queryRower, nodeID string, start, end, at time.Time) (TrafficPeriod, error) {
	if start.IsZero() || !start.Equal(dayStart(start)) || !end.Equal(dayStart(end)) || !end.After(start) || at.Before(start) {
		return TrafficPeriod{}, fmt.Errorf("invalid traffic period range")
	}
	var inbound, outbound int64
	var first, last sql.NullString
	// A node's range reads its own days through the primary key; the whole
	// fleet's reads the day index.
	query := `SELECT coalesce(sum(inbound_bytes),0),coalesce(sum(outbound_bytes),0),
        min(first_observed_at),max(last_sample_at)
        FROM traffic_days WHERE day_start>=? AND day_start<? AND day_start<=?`
	args := []any{formatTime(start), formatTime(end), formatTime(dayStart(at))}
	if nodeID != "" {
		query += ` AND node_id = ?`
		args = append(args, nodeID)
	}
	err := q.QueryRowContext(ctx, query, args...).Scan(&inbound, &outbound, &first, &last)
	if err != nil {
		return TrafficPeriod{}, err
	}
	firstAt := start
	metadata := map[string]any{"aggregation": "daily"}
	if first.Valid {
		firstAt, err = parseTime(first.String)
		if err != nil {
			return TrafficPeriod{}, err
		}
	}
	if last.Valid {
		lastAt, err := parseTime(last.String)
		if err != nil {
			return TrafficPeriod{}, err
		}
		metadata["latest_sample_at"] = lastAt
	}
	counters, err := json.Marshal(metadata)
	if err != nil {
		return TrafficPeriod{}, err
	}
	return TrafficPeriod{ID: trafficPeriodID(start, end), PeriodStart: start, PeriodEnd: end,
		InboundBytes: inbound, OutboundBytes: outbound, Counters: counters, CreatedAt: firstAt}, nil
}

func prepareTrafficSampleInput(input TrafficSampleInput) (TrafficSampleInput, error) {
	if !ValidIdentifier(input.NodeID) || input.InstanceID == "" || len(input.InstanceID) > 64 {
		return TrafficSampleInput{}, errors.New("traffic sample node is invalid")
	}
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

func getTrafficCheckpoint(ctx context.Context, tx *sql.Tx, nodeID string) (trafficCheckpoint, error) {
	var checkpoint trafficCheckpoint
	var sampled string
	err := tx.QueryRowContext(ctx, `
        SELECT instance_id,last_upload_total,last_download_total,sampled_at
        FROM traffic_checkpoint WHERE node_id=?`, nodeID).Scan(
		&checkpoint.InstanceID, &checkpoint.LastUpload, &checkpoint.LastDownload, &sampled)
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
            node_id, instance_id, activation_bundle_id, pid, process_start_token, sampled_at, memory_bytes,
            active_connections, upload_total, download_total, upload_delta, download_delta,
			interval_start, interval_end, coverage, accepted, diagnostic_code
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		input.NodeID, input.InstanceID, input.ActivationBundleID, input.PID, input.ProcessStartToken, formatTime(input.SampledAt),
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

func upsertTrafficCheckpoint(ctx context.Context, tx *sql.Tx, input TrafficSampleInput) error {
	_, err := tx.ExecContext(ctx, `
        INSERT INTO traffic_checkpoint(node_id,instance_id,last_upload_total,last_download_total,sampled_at)
        VALUES (?,?,?,?,?)
        ON CONFLICT(node_id) DO UPDATE SET
            instance_id=excluded.instance_id,
            last_upload_total=excluded.last_upload_total,last_download_total=excluded.last_download_total,
            sampled_at=excluded.sampled_at`,
		input.NodeID, input.InstanceID, input.UploadTotal, input.DownloadTotal, formatTime(input.SampledAt))
	if err != nil {
		return fmt.Errorf("update traffic checkpoint: %w", err)
	}
	return nil
}

// recordTrafficPeriod remembers the period an accepted sample belongs to, in
// the period length in force when it was recorded.
func recordTrafficPeriod(ctx context.Context, tx *sql.Tx, input TrafficSampleInput) error {
	_, err := tx.ExecContext(ctx, `
        INSERT INTO traffic_periods(node_id,id,period_start,period_end,created_at) VALUES (?,?,?,?,?)
        ON CONFLICT(node_id,id) DO NOTHING`,
		input.NodeID, trafficPeriodID(input.PeriodStart, input.PeriodEnd), formatTime(input.PeriodStart),
		formatTime(input.PeriodEnd), formatTime(input.SampledAt))
	if err != nil {
		return fmt.Errorf("record traffic period: %w", err)
	}
	return nil
}

func trafficPeriodID(start, end time.Time) string {
	return "traffic_" + start.UTC().Format("20060102") + "_" + end.UTC().Format("20060102")
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
