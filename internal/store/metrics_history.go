// SPDX-License-Identifier: GPL-3.0-or-later

package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"
)

const (
	maximumMetricsHistorySpan = 90 * 24 * time.Hour
	maximumMetricsBuckets     = 512
	maximumCompleteSampleGap  = 30 * time.Second
)

type CoverageStatus string

const (
	CoverageComplete CoverageStatus = "complete"
	CoveragePartial  CoverageStatus = "partial"
	CoverageMissing  CoverageStatus = "missing"
)

type MetricsHistoryFilter struct {
	From               time.Time
	To                 time.Time
	BucketSeconds      int64
	ActivationBundleID string
}

type MetricsHistoryBucket struct {
	From                     time.Time      `json:"from"`
	To                       time.Time      `json:"to"`
	UploadBytes              *int64         `json:"upload_bytes"`
	DownloadBytes            *int64         `json:"download_bytes"`
	MemoryBytesAverage       *float64       `json:"memory_bytes_avg"`
	MemoryBytesPeak          *int64         `json:"memory_bytes_peak"`
	ActiveConnectionsAverage *float64       `json:"active_connections_avg"`
	ActiveConnectionsPeak    *int64         `json:"active_connections_peak"`
	SampleCount              int64          `json:"sample_count"`
	Coverage                 CoverageStatus `json:"coverage"`
}

type MetricsHistory struct {
	From               time.Time              `json:"from"`
	To                 time.Time              `json:"to"`
	BucketSeconds      int64                  `json:"bucket_seconds"`
	ActivationBundleID string                 `json:"activation_bundle_id,omitempty"`
	Buckets            []MetricsHistoryBucket `json:"buckets"`
}

// MetricsHistory returns a bounded, gap-preserving sequence. Missing buckets
// are emitted explicitly and byte deltas remain null when no proven delta was
// persisted; missing evidence is never drawn as zero traffic.
func (s *Store) MetricsHistory(ctx context.Context, filter MetricsHistoryFilter) (MetricsHistory, error) {
	prepared, bucketCount, err := prepareMetricsHistoryFilter(filter)
	if err != nil {
		return MetricsHistory{}, err
	}
	bucketDuration := time.Duration(prepared.BucketSeconds) * time.Second
	result := MetricsHistory{
		From: prepared.From, To: prepared.To, BucketSeconds: prepared.BucketSeconds,
		ActivationBundleID: prepared.ActivationBundleID,
		Buckets:            make([]MetricsHistoryBucket, bucketCount),
	}
	for index := range result.Buckets {
		start := prepared.From.Add(time.Duration(index) * bucketDuration)
		end := start.Add(bucketDuration)
		if end.After(prepared.To) {
			end = prepared.To
		}
		result.Buckets[index] = MetricsHistoryBucket{From: start, To: end, Coverage: CoverageMissing}
	}

	clauses := []string{"sampled_at >= ?", "sampled_at < ?"}
	args := []any{
		formatTaskTime(prepared.From), prepared.BucketSeconds,
		formatTaskTime(prepared.From), formatTaskTime(prepared.To),
	}
	if prepared.ActivationBundleID != "" {
		clauses = append(clauses, "activation_bundle_id = ?")
		args = append(args, prepared.ActivationBundleID)
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT CAST((unixepoch(sampled_at, 'subsec') - unixepoch(?, 'subsec')) / ? AS INTEGER) AS bucket_index,
		       AVG(memory_bytes), MAX(memory_bytes),
		       AVG(active_connections), MAX(active_connections), COUNT(*)
		  FROM traffic_samples
		 WHERE `+strings.Join(clauses, " AND ")+`
		 GROUP BY bucket_index
		 ORDER BY bucket_index ASC`, args...)
	if err != nil {
		return MetricsHistory{}, fmt.Errorf("query metrics history: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var (
			index                             int
			memoryAverage, connectionsAverage sql.NullFloat64
			memoryPeak, connectionsPeak       sql.NullInt64
			sampleCount                       int64
		)
		if err := rows.Scan(
			&index, &memoryAverage, &memoryPeak,
			&connectionsAverage, &connectionsPeak, &sampleCount,
		); err != nil {
			return MetricsHistory{}, fmt.Errorf("scan metrics history bucket: %w", err)
		}
		if index < 0 || index >= len(result.Buckets) {
			return MetricsHistory{}, errors.New("metrics history bucket index is outside the requested range")
		}
		bucket := &result.Buckets[index]
		bucket.SampleCount = sampleCount
		if memoryAverage.Valid {
			bucket.MemoryBytesAverage = &memoryAverage.Float64
		}
		if memoryPeak.Valid {
			bucket.MemoryBytesPeak = &memoryPeak.Int64
		}
		if connectionsAverage.Valid {
			bucket.ActiveConnectionsAverage = &connectionsAverage.Float64
		}
		if connectionsPeak.Valid {
			bucket.ActiveConnectionsPeak = &connectionsPeak.Int64
		}
		bucket.Coverage = CoveragePartial
	}
	if err := rows.Err(); err != nil {
		return MetricsHistory{}, fmt.Errorf("iterate metrics history: %w", err)
	}
	if err := rows.Close(); err != nil {
		return MetricsHistory{}, fmt.Errorf("close metrics history samples: %w", err)
	}

	evidenceClauses := []string{
		"sample.interval_start IS NOT NULL",
		"sample.accepted = 1",
		"sample.interval_end > ?",
		"sample.interval_start < ?",
	}
	evidenceArgs := []any{
		formatTaskTime(prepared.From), formatTaskTime(prepared.To),
		prepared.BucketSeconds, bucketCount,
		formatTaskTime(prepared.From), formatTaskTime(prepared.To),
	}
	if prepared.ActivationBundleID != "" {
		evidenceClauses = append(evidenceClauses, "sample.activation_bundle_id = ?")
		evidenceArgs = append(evidenceArgs, prepared.ActivationBundleID)
	}
	evidenceRows, err := s.db.QueryContext(ctx, `
		WITH RECURSIVE parameters AS (
			SELECT unixepoch(?, 'subsec') AS range_start,
			       unixepoch(?, 'subsec') AS range_end,
			       CAST(? AS REAL) AS bucket_seconds,
			       CAST(? AS INTEGER) AS bucket_count
		), candidates AS (
			SELECT sample.id, sample.interval_start, sample.interval_end,
			       unixepoch(sample.interval_start, 'subsec') AS start_epoch,
			       unixepoch(sample.interval_end, 'subsec') AS end_epoch,
			       sample.upload_delta, sample.download_delta, sample.coverage
			  FROM traffic_samples AS sample
			 WHERE `+strings.Join(evidenceClauses, " AND ")+`
		), mapped AS (
			SELECT candidate.*,
			       CASE
			         WHEN candidate.start_epoch <= parameter.range_start THEN 0
			         ELSE MIN(
			           parameter.bucket_count - 1,
			           CAST((candidate.start_epoch - parameter.range_start) / parameter.bucket_seconds AS INTEGER)
			         )
			       END AS first_bucket,
			       CASE
			         WHEN candidate.end_epoch >= parameter.range_end THEN parameter.bucket_count - 1
			         ELSE MAX(
			           0,
			           CAST((candidate.end_epoch - parameter.range_start - 0.000001) / parameter.bucket_seconds AS INTEGER)
			         )
			       END AS last_bucket,
			       parameter.range_start, parameter.range_end,
			       parameter.bucket_seconds, parameter.bucket_count
			  FROM candidates AS candidate
			 CROSS JOIN parameters AS parameter
		), expanded AS (
			SELECT mapped.*, mapped.first_bucket AS bucket_index
			  FROM mapped
			 WHERE mapped.first_bucket <= mapped.last_bucket
			UNION ALL
			SELECT expanded.id, expanded.interval_start, expanded.interval_end,
			       expanded.start_epoch, expanded.end_epoch,
			       expanded.upload_delta, expanded.download_delta, expanded.coverage,
			       expanded.first_bucket, expanded.last_bucket,
			       expanded.range_start, expanded.range_end,
			       expanded.bucket_seconds, expanded.bucket_count,
			       expanded.bucket_index + 1
			  FROM expanded
			 WHERE expanded.bucket_index < expanded.last_bucket
		), bucketed AS (
			SELECT expanded.*,
			       expanded.range_start + expanded.bucket_index * expanded.bucket_seconds AS bucket_start,
			       MIN(
			         expanded.range_end,
			         expanded.range_start + (expanded.bucket_index + 1) * expanded.bucket_seconds
			       ) AS bucket_end
			  FROM expanded
		)
		SELECT bucket_index,
		       SUM(CASE
		             WHEN start_epoch >= bucket_start - 0.000001 AND end_epoch <= bucket_end + 0.000001
		             THEN upload_delta
		           END),
		       SUM(CASE
		             WHEN start_epoch >= bucket_start - 0.000001 AND end_epoch <= bucket_end + 0.000001
		             THEN download_delta
		           END),
		       COUNT(*),
		       COUNT(CASE WHEN coverage = 'complete' THEN 1 END),
		       MIN(CASE WHEN coverage = 'complete' THEN MAX(start_epoch, bucket_start) END),
		       MAX(CASE WHEN coverage = 'complete' THEN MIN(end_epoch, bucket_end) END),
		       SUM(CASE
		             WHEN coverage = 'complete' THEN MAX(
		               0.0,
		               MIN(end_epoch, bucket_end) - MAX(start_epoch, bucket_start)
		             )
		             ELSE 0.0
		           END)
		  FROM bucketed
		 GROUP BY bucket_index
		 ORDER BY bucket_index`, evidenceArgs...)
	if err != nil {
		return MetricsHistory{}, fmt.Errorf("query metrics interval evidence: %w", err)
	}
	defer evidenceRows.Close()
	for evidenceRows.Next() {
		var (
			index                                int
			upload, download                     sql.NullInt64
			evidenceCount, completeIntervalCount int64
			firstCoveredStart, lastCoveredEnd    sql.NullFloat64
			coveredSeconds                       sql.NullFloat64
		)
		if err := evidenceRows.Scan(
			&index, &upload, &download, &evidenceCount, &completeIntervalCount,
			&firstCoveredStart, &lastCoveredEnd, &coveredSeconds,
		); err != nil {
			return MetricsHistory{}, fmt.Errorf("scan metrics interval evidence: %w", err)
		}
		if index < 0 || index >= len(result.Buckets) {
			return MetricsHistory{}, errors.New("metrics interval bucket index is outside the requested range")
		}
		bucket := &result.Buckets[index]
		if upload.Valid {
			bucket.UploadBytes = &upload.Int64
		}
		if download.Valid {
			bucket.DownloadBytes = &download.Int64
		}
		if evidenceCount == 0 {
			continue
		}
		bucket.Coverage = CoveragePartial
		bucketStart := float64(bucket.From.UnixNano()) / float64(time.Second)
		bucketEnd := float64(bucket.To.UnixNano()) / float64(time.Second)
		// Accepted intervals are serialized through the singleton traffic
		// checkpoint and therefore cannot overlap. Their clipped duration can
		// cover the bucket exactly only when both boundaries and every point
		// between them have complete interval evidence.
		if completeIntervalCount > 0 && coveredSeconds.Valid &&
			firstCoveredStart.Valid && firstCoveredStart.Float64 <= bucketStart+0.000001 &&
			lastCoveredEnd.Valid && lastCoveredEnd.Float64 >= bucketEnd-0.000001 &&
			math.Abs(coveredSeconds.Float64-(bucketEnd-bucketStart)) <= 0.000001 {
			bucket.Coverage = CoverageComplete
		}
	}
	if err := evidenceRows.Err(); err != nil {
		return MetricsHistory{}, fmt.Errorf("iterate metrics interval evidence: %w", err)
	}
	return result, nil
}

func prepareMetricsHistoryFilter(filter MetricsHistoryFilter) (MetricsHistoryFilter, int, error) {
	if filter.From.IsZero() || filter.To.IsZero() || !filter.To.After(filter.From) {
		return MetricsHistoryFilter{}, 0, errors.New("metrics history requires an increasing from/to range")
	}
	filter.From, filter.To = filter.From.UTC(), filter.To.UTC()
	span := filter.To.Sub(filter.From)
	if span > maximumMetricsHistorySpan {
		return MetricsHistoryFilter{}, 0, errors.New("metrics history range exceeds 90 days")
	}
	if filter.BucketSeconds < 1 || filter.BucketSeconds > int64(maximumMetricsHistorySpan/time.Second) {
		return MetricsHistoryFilter{}, 0, errors.New("metrics history bucket_seconds is invalid")
	}
	bucketDuration := time.Duration(filter.BucketSeconds) * time.Second
	bucketCount := int((span + bucketDuration - 1) / bucketDuration)
	if bucketCount < 1 || bucketCount > maximumMetricsBuckets {
		return MetricsHistoryFilter{}, 0, errors.New("metrics history exceeds 512 buckets")
	}
	if filter.ActivationBundleID != strings.TrimSpace(filter.ActivationBundleID) {
		return MetricsHistoryFilter{}, 0, errors.New("metrics history activation bundle id is invalid")
	}
	return filter, bucketCount, nil
}

func (s *Store) DeleteTrafficSamplesBefore(ctx context.Context, cutoff time.Time) (int64, error) {
	if cutoff.IsZero() {
		return 0, errors.New("traffic sample retention cutoff is required")
	}
	result, err := s.db.ExecContext(
		ctx,
		`DELETE FROM traffic_samples WHERE sampled_at < ?`,
		formatTaskTime(cutoff.UTC()),
	)
	if err != nil {
		return 0, fmt.Errorf("delete expired traffic samples: %w", err)
	}
	deleted, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("read deleted traffic sample count: %w", err)
	}
	return deleted, nil
}
