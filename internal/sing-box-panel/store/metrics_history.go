// SPDX-License-Identifier: GPL-3.0-or-later

package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"strings"
	"sync"
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
	NodeID             string
	From               time.Time
	To                 time.Time
	BucketSeconds      int64
	ActivationBundleID string
}

type MetricsHistoryBucket struct {
	From          time.Time `json:"from"`
	To            time.Time `json:"to"`
	UploadBytes   *int64    `json:"upload_bytes"`
	DownloadBytes *int64    `json:"download_bytes"`
	// TrafficSeconds is how long the intervals whose deltas the traffic
	// holds last, so their rate is the traffic over it; nil with no traffic.
	TrafficSeconds           *float64       `json:"traffic_seconds"`
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
// persisted; missing evidence is never drawn as zero traffic. A bucket without
// samples that the node's runtime history shows wholly without a running core
// is known instead: it holds zero traffic, connections and memory.
func (s *Store) MetricsHistory(ctx context.Context, filter MetricsHistoryFilter) (MetricsHistory, error) {
	return metricsHistory(ctx, s.db, filter)
}

type metricsHistoryQuery interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

func metricsHistory(ctx context.Context, db metricsHistoryQuery, filter MetricsHistoryFilter) (MetricsHistory, error) {
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

	clauses := []string{"node_id = ?", "sampled_at >= ?", "sampled_at < ?"}
	args := []any{
		formatTime(prepared.From), prepared.BucketSeconds,
		prepared.NodeID, formatTime(prepared.From), formatTime(prepared.To),
	}
	if prepared.ActivationBundleID != "" {
		clauses = append(clauses, "activation_bundle_id = ?")
		args = append(args, prepared.ActivationBundleID)
	}
	rows, err := db.QueryContext(ctx, `
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

	// An interval ends at its sample, so sampled_at bounds the node's
	// history from below: the history index reads only the samples after
	// From, where the interval index would read every interval before To.
	evidenceClauses := []string{
		"sample.node_id = ?",
		"sample.sampled_at > ?",
		"sample.interval_start IS NOT NULL",
		"sample.accepted = 1",
		"sample.interval_end > ?",
		"sample.interval_start < ?",
	}
	evidenceArgs := []any{
		formatTime(prepared.From), formatTime(prepared.To),
		prepared.BucketSeconds, bucketCount,
		prepared.NodeID, formatTime(prepared.From), formatTime(prepared.From), formatTime(prepared.To),
	}
	if prepared.ActivationBundleID != "" {
		evidenceClauses = append(evidenceClauses, "sample.activation_bundle_id = ?")
		evidenceArgs = append(evidenceArgs, prepared.ActivationBundleID)
	}
	evidenceRows, err := db.QueryContext(ctx, `
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
			  FROM traffic_samples AS sample INDEXED BY traffic_samples_history
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
		       SUM(CASE
		             WHEN start_epoch >= bucket_start - 0.000001 AND end_epoch <= bucket_end + 0.000001
		                  AND upload_delta IS NOT NULL AND download_delta IS NOT NULL
		             THEN end_epoch - start_epoch
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
			trafficSeconds                       sql.NullFloat64
			evidenceCount, completeIntervalCount int64
			firstCoveredStart, lastCoveredEnd    sql.NullFloat64
			coveredSeconds                       sql.NullFloat64
		)
		if err := evidenceRows.Scan(
			&index, &upload, &download, &trafficSeconds, &evidenceCount, &completeIntervalCount,
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
		if trafficSeconds.Valid {
			bucket.TrafficSeconds = &trafficSeconds.Float64
		}
		if evidenceCount == 0 {
			continue
		}
		bucket.Coverage = CoveragePartial
		bucketStart := float64(bucket.From.UnixNano()) / float64(time.Second)
		bucketEnd := float64(bucket.To.UnixNano()) / float64(time.Second)
		// Accepted intervals of a node are serialized through its traffic
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
	if err := evidenceRows.Close(); err != nil {
		return MetricsHistory{}, fmt.Errorf("close metrics interval evidence: %w", err)
	}
	idle, err := runtimeIdleSpans(ctx, db, prepared.NodeID, prepared.From, prepared.To)
	if err != nil {
		return MetricsHistory{}, err
	}
	for index := range result.Buckets {
		bucket := &result.Buckets[index]
		if bucket.Coverage == CoverageMissing && idle.covers(bucket.From, bucket.To) {
			zeroIdleBucket(bucket)
		}
	}
	return result, nil
}

// timeSpans are ordered, disjoint, half-open intervals of time.
type timeSpans []struct{ from, to time.Time }

// covers reports whether one span holds all of [from, to).
func (spans timeSpans) covers(from, to time.Time) bool {
	for _, span := range spans {
		if !span.from.After(from) && !span.to.Before(to) {
			return true
		}
	}
	return false
}

// IdleSpan is a part of a range in which a node's runtime history shows no
// core running.
type IdleSpan struct {
	From time.Time
	To   time.Time
}

// RuntimeIdleSpans lists the parts of [from, to) in which the node's runtime
// history shows no core running, oldest first.
func (s *Store) RuntimeIdleSpans(ctx context.Context, nodeID string, from, to time.Time) ([]IdleSpan, error) {
	spans, err := runtimeIdleSpans(ctx, s.db, nodeID, from, to)
	if err != nil {
		return nil, err
	}
	result := make([]IdleSpan, 0, len(spans))
	for _, span := range spans {
		result = append(result, IdleSpan{From: span.from, To: span.to})
	}
	return result, nil
}

// runtimeIdleSpans lists the parts of [from, to) in which the node's runtime
// history shows no core running: a stopped or failed state that no later
// uncertainty reaches back over. Before the node's first transition the state
// is unknown; the latest state lasts until to. This mirrors how the Web
// dashboard draws the runtime timeline from the same history.
func runtimeIdleSpans(ctx context.Context, db metricsHistoryQuery, nodeID string, from, to time.Time) (timeSpans, error) {
	// The state at from is the latest transition before it; an unknown state
	// recorded after to can still reach back into the range.
	rows, err := db.QueryContext(ctx, `
		SELECT state, occurred_at, uncertain_since
		  FROM runtime_transitions
		 WHERE node_id = ?
		   AND occurred_at >= COALESCE(
		         (SELECT MAX(occurred_at) FROM runtime_transitions WHERE node_id = ? AND occurred_at < ?), ?)
		   AND (occurred_at < ? OR uncertain_since < ?)
		 ORDER BY occurred_at ASC, id ASC`,
		nodeID, nodeID, formatTime(from), formatTime(from), formatTime(to), formatTime(to))
	if err != nil {
		return nil, fmt.Errorf("query runtime idle spans: %w", err)
	}
	defer rows.Close()
	type boundary struct {
		at    time.Time
		state RuntimeTransitionState
	}
	var boundaries []boundary
	for rows.Next() {
		var (
			state          RuntimeTransitionState
			occurredRaw    string
			uncertainSince sql.NullString
		)
		if err := rows.Scan(&state, &occurredRaw, &uncertainSince); err != nil {
			return nil, fmt.Errorf("scan runtime idle span: %w", err)
		}
		at, err := parseTime(occurredRaw)
		if err != nil {
			return nil, fmt.Errorf("parse runtime transition time: %w", err)
		}
		if state == RuntimeTransitionUnknown && uncertainSince.Valid {
			since, err := parseTime(uncertainSince.String)
			if err != nil {
				return nil, fmt.Errorf("parse runtime uncertainty time: %w", err)
			}
			if since.Before(at) {
				at = since
			}
			// Uncertainty replaces whatever the history said from its start on.
			for len(boundaries) > 0 && !boundaries[len(boundaries)-1].at.Before(at) {
				boundaries = boundaries[:len(boundaries)-1]
			}
		}
		boundaries = append(boundaries, boundary{at: at, state: state})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate runtime idle spans: %w", err)
	}
	var spans timeSpans
	for index, current := range boundaries {
		if current.state != RuntimeTransitionStopped && current.state != RuntimeTransitionFailed {
			continue
		}
		start, end := current.at, to
		if index+1 < len(boundaries) {
			end = boundaries[index+1].at
		}
		start, end = maxTime(start, from), minTime(end, to)
		if !start.Before(end) {
			continue
		}
		// A failure after a stop, or the reverse, leaves the core idle throughout.
		if last := len(spans) - 1; last >= 0 && spans[last].to.Equal(start) {
			spans[last].to = end
			continue
		}
		spans = append(spans, struct{ from, to time.Time }{start, end})
	}
	return spans, nil
}

// zeroIdleBucket marks a bucket in which no core ran: nothing was moved,
// connected or held in memory, which is complete evidence of the bucket.
func zeroIdleBucket(bucket *MetricsHistoryBucket) {
	upload, download, memoryPeak, connectionsPeak := int64(0), int64(0), int64(0), int64(0)
	memoryAverage, connectionsAverage := float64(0), float64(0)
	seconds := bucket.To.Sub(bucket.From).Seconds()
	bucket.UploadBytes, bucket.DownloadBytes, bucket.TrafficSeconds = &upload, &download, &seconds
	bucket.MemoryBytesAverage, bucket.MemoryBytesPeak = &memoryAverage, &memoryPeak
	bucket.ActiveConnectionsAverage, bucket.ActiveConnectionsPeak = &connectionsAverage, &connectionsPeak
	bucket.Coverage = CoverageComplete
}

func maxTime(left, right time.Time) time.Time {
	if left.After(right) {
		return left
	}
	return right
}

func minTime(left, right time.Time) time.Time {
	if left.Before(right) {
		return left
	}
	return right
}

func prepareMetricsHistoryFilter(filter MetricsHistoryFilter) (MetricsHistoryFilter, int, error) {
	if !ValidIdentifier(filter.NodeID) {
		return MetricsHistoryFilter{}, 0, invalidQuery("metrics history requires a node")
	}
	if filter.From.IsZero() || filter.To.IsZero() || !filter.To.After(filter.From) {
		return MetricsHistoryFilter{}, 0, invalidQuery("metrics history requires an increasing from/to range")
	}
	filter.From, filter.To = filter.From.UTC(), filter.To.UTC()
	span := filter.To.Sub(filter.From)
	if span > maximumMetricsHistorySpan {
		return MetricsHistoryFilter{}, 0, invalidQuery("metrics history range exceeds 90 days")
	}
	if filter.BucketSeconds < 1 || filter.BucketSeconds > int64(maximumMetricsHistorySpan/time.Second) {
		return MetricsHistoryFilter{}, 0, invalidQuery("metrics history bucket_seconds is invalid")
	}
	bucketDuration := time.Duration(filter.BucketSeconds) * time.Second
	bucketCount := int((span + bucketDuration - 1) / bucketDuration)
	if bucketCount < 1 || bucketCount > maximumMetricsBuckets {
		return MetricsHistoryFilter{}, 0, invalidQuery("metrics history exceeds 512 buckets")
	}
	if filter.ActivationBundleID != strings.TrimSpace(filter.ActivationBundleID) {
		return MetricsHistoryFilter{}, 0, invalidQuery("metrics history activation bundle id is invalid")
	}
	return filter, bucketCount, nil
}

// DashboardHistory keeps one node's dashboard ranges between reads: the
// closed buckets of each range and the newest traffic sample and runtime
// transition the last read saw. Both are only ever appended, each row with a
// larger ID than every row committed before it, so a later read recomputes
// only from the earliest instant that the node's rows added since reach, and
// the open bucket, and keeps the closed buckets before it. Deleting samples
// or transitions within the ranges, such as by retention or a node's removal,
// must Reset it. The zero value is empty. It is safe for concurrent use; its
// reads run one at a time, and readers must not change what they return.
type DashboardHistory struct {
	mu     sync.Mutex
	nodeID string
	// seen is whether sampleID and transitionID hold a read's newest IDs.
	seen                   bool
	sampleID, transitionID int64
	hour, day              []MetricsHistoryBucket
}

// Reset makes the next read compute every bucket.
func (history *DashboardHistory) Reset() {
	history.mu.Lock()
	defer history.mu.Unlock()
	history.reset()
}

func (history *DashboardHistory) reset() {
	history.nodeID, history.seen, history.sampleID, history.transitionID = "", false, 0, 0
	history.hour, history.day = nil, nil
}

// DashboardMetricsHistory reads the last hour in minute buckets and the last
// day in five-minute buckets, with their accepted persistence watermark, from
// one SQLite snapshot, so clients can replace overlapping tails. Each range
// starts on a whole bucket, so a later read changes only its open bucket and
// what newly recorded rows reach. A nil history reads every bucket.
func (s *Store) DashboardMetricsHistory(ctx context.Context, nodeID string, at time.Time, history *DashboardHistory) (MetricsHistory, MetricsHistory, *time.Time, error) {
	if history == nil {
		history = &DashboardHistory{}
	}
	history.mu.Lock()
	defer history.mu.Unlock()
	if history.nodeID != nodeID {
		history.reset()
		history.nodeID = nodeID
	}
	at = at.UTC()
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return MetricsHistory{}, MetricsHistory{}, nil, err
	}
	defer tx.Rollback()
	var raw sql.NullString
	err = tx.QueryRowContext(ctx, `SELECT MAX(sampled_at) FROM traffic_samples WHERE node_id = ? AND accepted=1 AND sampled_at <= ?`, nodeID, formatTime(at)).Scan(&raw)
	if err != nil {
		return MetricsHistory{}, MetricsHistory{}, nil, err
	}
	var watermark *time.Time
	if raw.Valid {
		parsed, parseErr := parseTime(raw.String)
		if parseErr != nil {
			return MetricsHistory{}, MetricsHistory{}, nil, parseErr
		}
		watermark = &parsed
	}
	unchangedBefore, sampleID, transitionID, err := history.changes(ctx, tx, at)
	if err != nil {
		return MetricsHistory{}, MetricsHistory{}, nil, err
	}
	hour, hourClosed, err := dashboardRange(ctx, tx, nodeID, at, time.Hour, 60, history.hour, unchangedBefore)
	if err != nil {
		return MetricsHistory{}, MetricsHistory{}, nil, err
	}
	day, dayClosed, err := dashboardRange(ctx, tx, nodeID, at, 24*time.Hour, 300, history.day, unchangedBefore)
	if err != nil {
		return MetricsHistory{}, MetricsHistory{}, nil, err
	}
	if err := tx.Commit(); err != nil {
		return MetricsHistory{}, MetricsHistory{}, nil, err
	}
	history.seen, history.sampleID, history.transitionID = true, sampleID, transitionID
	history.hour, history.day = hourClosed, dayClosed
	return hour, day, watermark, nil
}

// changes reads the newest sample and transition IDs and the instant before
// which the node's rows added since the last read change no bucket: the
// earliest that one of them reaches, at when none was added, and the zero
// time before a first read.
func (history *DashboardHistory) changes(ctx context.Context, tx *sql.Tx, at time.Time) (time.Time, int64, int64, error) {
	if !history.seen {
		var sampleID, transitionID int64
		err := tx.QueryRowContext(ctx, `SELECT (SELECT coalesce(MAX(id), 0) FROM traffic_samples),
		        (SELECT coalesce(MAX(id), 0) FROM runtime_transitions)`).Scan(&sampleID, &transitionID)
		return time.Time{}, sampleID, transitionID, err
	}
	unchangedBefore := at
	ids := [2]int64{history.sampleID, history.transitionID}
	// A sample's interval starts before it was taken, and a transition's
	// uncertainty before it occurred; IDs are the rowids, so each query reads
	// only the rows added since.
	for index, query := range []string{
		`SELECT MAX(id), MIN(CASE WHEN node_id = ? THEN coalesce(interval_start, sampled_at) END)
		   FROM traffic_samples WHERE id > ?`,
		`SELECT MAX(id), MIN(CASE WHEN node_id = ? THEN coalesce(uncertain_since, occurred_at) END)
		   FROM runtime_transitions WHERE id > ?`,
	} {
		var newest sql.NullInt64
		var earliest sql.NullString
		if err := tx.QueryRowContext(ctx, query, history.nodeID, ids[index]).Scan(&newest, &earliest); err != nil {
			return time.Time{}, 0, 0, fmt.Errorf("read dashboard history changes: %w", err)
		}
		if newest.Valid {
			ids[index] = newest.Int64
		}
		if earliest.Valid {
			reached, err := parseTime(earliest.String)
			if err != nil {
				return time.Time{}, 0, 0, err
			}
			unchangedBefore = minTime(unchangedBefore, reached)
		}
	}
	return unchangedBefore, ids[0], ids[1], nil
}

// dashboardRange reads the span that ends at at in buckets of bucketSeconds
// from a whole bucket. It keeps the closed buckets of cached that end by
// unchangedBefore and reads the rest; it returns the range and its closed
// buckets.
func dashboardRange(
	ctx context.Context, db metricsHistoryQuery, nodeID string, at time.Time, span time.Duration, bucketSeconds int64,
	cached []MetricsHistoryBucket, unchangedBefore time.Time,
) (MetricsHistory, []MetricsHistoryBucket, error) {
	bucket := time.Duration(bucketSeconds) * time.Second
	from := at.Add(-span).Truncate(bucket)
	result := MetricsHistory{From: from, To: at, BucketSeconds: bucketSeconds}
	result.Buckets = make([]MetricsHistoryBucket, 0, int((at.Sub(from)+bucket-1)/bucket))
	if len(cached) > 0 && !cached[0].From.After(from) {
		// Buckets lie on one grid, so the range's first bucket is a whole
		// number of buckets after the first cached one.
		reusable := minTime(at, unchangedBefore)
		for _, closed := range cached[min(len(cached), int(from.Sub(cached[0].From)/bucket)):] {
			if !closed.From.Equal(from.Add(time.Duration(len(result.Buckets))*bucket)) || closed.To.After(reusable) {
				break
			}
			result.Buckets = append(result.Buckets, closed)
		}
	}
	if tailFrom := from.Add(time.Duration(len(result.Buckets)) * bucket); tailFrom.Before(at) {
		tail, err := metricsHistory(ctx, db, MetricsHistoryFilter{NodeID: nodeID, From: tailFrom, To: at, BucketSeconds: bucketSeconds})
		if err != nil {
			return MetricsHistory{}, nil, err
		}
		result.Buckets = append(result.Buckets, tail.Buckets...)
	}
	closed := result.Buckets
	if last := len(closed) - 1; last >= 0 && closed[last].To.Sub(closed[last].From) < bucket {
		closed = closed[:last]
	}
	return result, closed, nil
}
