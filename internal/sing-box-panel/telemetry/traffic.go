// SPDX-License-Identifier: GPL-3.0-or-later

package telemetry

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/store"
)

const gibibyte = int64(1 << 30)

// RetentionResult counts the records one retention pass deleted.
type RetentionResult struct {
	TrafficSamples     int64 `json:"traffic_samples"`
	RuntimeTransitions int64 `json:"runtime_transitions"`
}

// NodeTrafficUsage is one node's recorded traffic in its current period,
// against its own quota.
type NodeTrafficUsage struct {
	NodeID        string              `json:"node_id"`
	Period        store.TrafficPeriod `json:"current_traffic_period"`
	QuotaBytes    *int64              `json:"quota_bytes,omitempty"`
	QuotaExceeded bool                `json:"quota_exceeded"`
}

// TrafficStatus is every node's recorded traffic in its own current period.
// Quotas belong to nodes, so the fleet sums only usage.
type TrafficStatus struct {
	CollectedAt   time.Time          `json:"collected_at"`
	UploadBytes   int64              `json:"upload_bytes"`
	DownloadBytes int64              `json:"download_bytes"`
	Nodes         []NodeTrafficUsage `json:"nodes"`
}

// nodeTraffic returns a node's recorded traffic in the period that contains
// at, against the quota in node's settings. A successful empty ledger is
// zero; a storage failure remains an error.
func (service *Service) nodeTraffic(ctx context.Context, node store.Node, at time.Time) (NodeTrafficUsage, error) {
	period, err := service.trafficPeriod(ctx, node, at)
	if err != nil {
		return NodeTrafficUsage{}, err
	}
	usage := NodeTrafficUsage{NodeID: node.ID, Period: period}
	if node.Settings.Traffic.QuotaGiB != nil {
		quota := *node.Settings.Traffic.QuotaGiB * gibibyte
		usage.QuotaBytes = &quota
		usage.QuotaExceeded = period.InboundBytes+period.OutboundBytes >= quota
	}
	return usage, nil
}

// TrafficStatus returns the traffic of every node, or of only the nodes in
// only when it is not nil, each in its current period, and their summed
// usage.
func (service *Service) TrafficStatus(ctx context.Context, at time.Time, only []string) (TrafficStatus, error) {
	nodes, err := service.database.Nodes(ctx)
	if err != nil {
		return TrafficStatus{}, err
	}
	var wanted map[string]bool
	if only != nil {
		wanted = map[string]bool{}
		for _, id := range only {
			wanted[id] = true
		}
	}
	status := TrafficStatus{CollectedAt: at.UTC(), Nodes: make([]NodeTrafficUsage, 0, len(nodes))}
	for _, node := range nodes {
		if wanted != nil && !wanted[node.ID] {
			continue
		}
		usage, err := service.nodeTraffic(ctx, node, at)
		if err != nil {
			return TrafficStatus{}, err
		}
		status.Nodes = append(status.Nodes, usage)
		status.UploadBytes += usage.Period.OutboundBytes
		status.DownloadBytes += usage.Period.InboundBytes
	}
	return status, nil
}

// TrafficPeriod returns the node's period that contains at: periods of
// value.PeriodMonths months start at 00:00 UTC on value.ResetDay, aligned to
// months counted from January 1970.
func TrafficPeriod(at time.Time, value store.NodeTrafficSettings) (time.Time, time.Time, error) {
	months, day := value.PeriodMonths, value.ResetDay
	if at.IsZero() || months < 1 || months > store.MaximumTrafficPeriodMonths || day < 1 || day > store.MaximumTrafficResetDay {
		return time.Time{}, time.Time{}, fmt.Errorf("invalid traffic period configuration")
	}
	at = at.UTC()
	monthIndex := (at.Year()-1970)*12 + int(at.Month()) - 1
	if at.Day() < day {
		monthIndex--
	}
	startIndex := monthIndex - floorMod(monthIndex, months)
	start := time.Date(1970, time.Month(1+startIndex), day, 0, 0, 0, 0, time.UTC)
	return start, time.Date(1970, time.Month(1+startIndex+months), day, 0, 0, 0, 0, time.UTC), nil
}

func floorMod(value, divisor int) int {
	remainder := value % divisor
	if remainder < 0 {
		return remainder + divisor
	}
	return remainder
}

func (application *Service) MetricsHistory(
	ctx context.Context,
	filter store.MetricsHistoryFilter,
) (store.MetricsHistory, error) {
	return application.database.MetricsHistory(ctx, filter)
}

// RecentMetricsWindow is how far back a node's recent metrics reach: the span
// a live chart shows, so one opened now starts with what came before it.
const RecentMetricsWindow = 2 * time.Minute

// RecentMetrics is a node's stored samples of the last RecentMetricsWindow,
// oldest first, and the parts of it in which no core ran, which hold no
// samples but are known zero traffic and connections.
type RecentMetrics struct {
	From    time.Time             `json:"from"`
	To      time.Time             `json:"to"`
	Samples []RecentMetricsSample `json:"samples"`
	Idle    []RecentIdleSpan      `json:"idle"`
}

// RecentIdleSpan is a part of the window in which the node's runtime history
// shows no core running.
type RecentIdleSpan struct {
	From time.Time `json:"from"`
	To   time.Time `json:"to"`
}

// RecentMetricsSample is one stored sample as a live chart draws it. Its
// rates cover the interval that ends at the sample and are nil when the
// interval is unknown, as after a first sample or a process change.
type RecentMetricsSample struct {
	SampledAt              time.Time `json:"sampled_at"`
	IntervalSeconds        *float64  `json:"interval_seconds"`
	UploadBytesPerSecond   *float64  `json:"upload_bytes_per_second"`
	DownloadBytesPerSecond *float64  `json:"download_bytes_per_second"`
	ActiveConnections      int64     `json:"active_connections"`
}

// RecentMetrics returns the node's accepted samples of the last
// RecentMetricsWindow and the spans in which its core did not run. Samples
// are stored about every ten seconds whatever the node's live sampling, so
// they are as far apart; a stopped core stores none.
func (application *Service) RecentMetrics(ctx context.Context, nodeID string) (RecentMetrics, error) {
	to := application.now().UTC()
	from := to.Add(-RecentMetricsWindow)
	samples, err := application.database.AcceptedTrafficSamplesSince(ctx, nodeID, from)
	if err != nil {
		return RecentMetrics{}, err
	}
	idle, err := application.database.RuntimeIdleSpans(ctx, nodeID, from, to)
	if err != nil {
		return RecentMetrics{}, err
	}
	result := RecentMetrics{From: from, To: to, Samples: make([]RecentMetricsSample, 0, len(samples)),
		Idle: make([]RecentIdleSpan, 0, len(idle))}
	for _, span := range idle {
		result.Idle = append(result.Idle, RecentIdleSpan{From: span.From, To: span.To})
	}
	for _, sample := range samples {
		point := RecentMetricsSample{SampledAt: sample.SampledAt, ActiveConnections: sample.ActiveConnections}
		if sample.IntervalStart != nil && sample.UploadDelta != nil && sample.DownloadDelta != nil {
			if seconds := sample.SampledAt.Sub(*sample.IntervalStart).Seconds(); seconds > 0 {
				upload := float64(*sample.UploadDelta) / seconds
				download := float64(*sample.DownloadDelta) / seconds
				point.IntervalSeconds, point.UploadBytesPerSecond, point.DownloadBytesPerSecond = &seconds, &upload, &download
			}
		}
		result.Samples = append(result.Samples, point)
	}
	return result, nil
}

// EnforceRetention deletes raw traffic samples older than the sample
// retention and runtime history older than the runtime history retention.
// Daily traffic and traffic periods are the long-lived accounting record and
// stay; the panel log is cleared only on request.
func (application *Service) EnforceRetention(ctx context.Context) (RetentionResult, error) {
	if application.settings == nil {
		return RetentionResult{}, errors.New("retention settings are unavailable")
	}
	effective, err := application.settings.EffectiveSettings(ctx)
	if err != nil {
		return RetentionResult{}, err
	}
	sampleDays, historyDays := effective.Traffic.SampleRetentionDays, effective.Nodes.RuntimeHistoryRetentionDays
	if sampleDays < 1 || sampleDays > 366 || historyDays < 1 || historyDays > 3650 {
		return RetentionResult{}, errors.New("retention settings are unavailable")
	}
	now := application.now().UTC()
	var result RetentionResult
	// A retention of one day reaches into the dashboard's day.
	defer func() {
		if result.TrafficSamples > 0 || result.RuntimeTransitions > 0 {
			application.resetDashboardHistory("")
		}
	}()
	if result.TrafficSamples, err = application.database.DeleteTrafficSamplesBefore(ctx, now.AddDate(0, 0, -sampleDays)); err != nil {
		return result, err
	}
	result.RuntimeTransitions, err = application.database.DeleteRuntimeTransitionsBefore(ctx, now.AddDate(0, 0, -historyDays))
	return result, err
}

func (application *Service) TrafficPeriod(ctx context.Context, nodeID, periodID string) (store.TrafficPeriod, error) {
	return application.database.GetTrafficPeriod(ctx, nodeID, periodID)
}

func (application *Service) ListTrafficPeriodPage(
	ctx context.Context,
	filter store.TrafficPeriodFilter,
) (store.TrafficPeriodPage, error) {
	return application.database.ListTrafficPeriodPage(ctx, filter)
}

func IsTrafficPeriodNotFound(err error) bool {
	return errors.Is(err, store.ErrTrafficPeriodNotFound)
}
