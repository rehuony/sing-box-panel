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

type TrafficSampleRetentionResult struct {
	Deleted int64     `json:"deleted"`
	Cutoff  time.Time `json:"cutoff"`
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

// NodeTraffic returns a node's recorded traffic in the period that contains
// at. A successful empty ledger is zero; a storage failure remains an error.
func (service *Service) NodeTraffic(ctx context.Context, nodeID string, at time.Time) (NodeTrafficUsage, error) {
	node, err := service.database.Node(ctx, nodeID)
	if err != nil {
		return NodeTrafficUsage{}, err
	}
	return service.nodeTraffic(ctx, node, at)
}

func (service *Service) nodeTraffic(ctx context.Context, node store.Node, at time.Time) (NodeTrafficUsage, error) {
	start, end, err := TrafficPeriod(at, node.Settings.Traffic)
	if err != nil {
		return NodeTrafficUsage{}, err
	}
	period, err := service.database.AggregateTrafficPeriod(ctx, node.ID, start, end, at)
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

// EnforceTrafficSampleRetention removes only raw samples. Aggregated traffic
// periods are intentionally retained as the long-lived accounting record.
func (application *Service) EnforceTrafficSampleRetention(ctx context.Context) (TrafficSampleRetentionResult, error) {
	if application.settings == nil {
		return TrafficSampleRetentionResult{}, errors.New("traffic sample retention setting is unavailable")
	}
	effective, err := application.settings.EffectiveSettings(ctx)
	if err != nil {
		return TrafficSampleRetentionResult{}, err
	}
	days := effective.Traffic.SampleRetentionDays
	if days < 1 || days > 366 {
		return TrafficSampleRetentionResult{}, errors.New("traffic sample retention setting is unavailable")
	}
	cutoff := application.now().UTC().Add(-time.Duration(days) * 24 * time.Hour)
	deleted, err := application.database.DeleteTrafficSamplesBefore(ctx, cutoff)
	if err != nil {
		return TrafficSampleRetentionResult{}, err
	}
	return TrafficSampleRetentionResult{Deleted: deleted, Cutoff: cutoff}, nil
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
