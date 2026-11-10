// SPDX-License-Identifier: GPL-3.0-or-later

package telemetry

import (
	"context"
	"database/sql"
	"errors"
	"sync"
	"time"

	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/store"
)

const (
	// trafficPersistInterval spaces the traffic samples the panel stores per
	// node; samples in between only update the live view. Cumulative counters
	// keep accounting exact whichever samples are stored.
	trafficPersistInterval = 9 * time.Second
	// maximumLiveRateGap is the longest interval a live rate is derived over.
	// It covers the baseline ten-second sampling, so every reporting node
	// has a recent rate; a node someone views samples each second.
	maximumLiveRateGap = 15 * time.Second
	// liveSampleLifetime is how long a node's latest sample counts as current.
	liveSampleLifetime = 30 * time.Second
)

// HostMetrics are host-wide counters a node reported.
type HostMetrics struct {
	SampledAt   time.Time `json:"sampled_at"`
	CPUPercent  *float64  `json:"cpu_percent"`
	CPUCount    int       `json:"cpu_count"`
	LoadOne     *float64  `json:"load_one"`
	MemoryTotal *uint64   `json:"memory_total"`
	MemoryUsed  *uint64   `json:"memory_used"`
	DiskTotal   *uint64   `json:"disk_total"`
	DiskUsed    *uint64   `json:"disk_used"`
}

// NodeTraffic is the running core's traffic in one node sample. Its totals
// are cumulative for the node instance.
type NodeTraffic struct {
	ActivationBundleID string
	PID                int
	ProcessStartToken  string
	UploadTotal        int64
	DownloadTotal      int64
	Connections        int64
	MemoryBytes        int64
}

// NodeSample is one metrics sample a node reported.
type NodeSample struct {
	SampledAt time.Time
	Host      *HostMetrics
	Traffic   *NodeTraffic
}

// LiveTrafficSample is a node's latest traffic reading. It is never added to
// persisted period totals.
type LiveTrafficSample struct {
	ActivationBundleID     string    `json:"activation_bundle_id"`
	PID                    int       `json:"pid"`
	ProcessStartToken      string    `json:"process_start_token"`
	SampledAt              time.Time `json:"sampled_at"`
	MemoryBytes            int64     `json:"memory_bytes"`
	ActiveConnections      int64     `json:"active_connections"`
	UploadTotal            int64     `json:"upload_total"`
	DownloadTotal          int64     `json:"download_total"`
	UploadBytesPerSecond   *float64  `json:"upload_bytes_per_second"`
	DownloadBytesPerSecond *float64  `json:"download_bytes_per_second"`
	Accepted               bool      `json:"accepted"`
}

// AppliedBundle is what a node reports it applied.
type AppliedBundle struct {
	ID             string
	MonitoringTier string
}

// MetricsSnapshot separates a node's live evidence from its durable traffic
// ledger. Every successful read includes recorded usage, even when zero.
type MetricsSnapshot struct {
	LiveSample         *LiveTrafficSample   `json:"live_sample,omitempty"`
	Host               *HostMetrics         `json:"host,omitempty"`
	Available          bool                 `json:"available"`
	ReasonCode         string               `json:"reason_code,omitempty"`
	AppliedBundleID    string               `json:"applied_bundle_id,omitempty"`
	MonitoringTier     string               `json:"monitoring_tier,omitempty"`
	CollectedAt        time.Time            `json:"collected_at"`
	CurrentTrafficData store.TrafficPeriod  `json:"current_traffic_period"`
	LatestSample       *store.TrafficSample `json:"latest_sample,omitempty"`
	QuotaBytes         *int64               `json:"quota_bytes,omitempty"`
	QuotaExceeded      bool                 `json:"quota_exceeded"`
}

// liveNode is the newest evidence of one node.
type liveNode struct {
	instanceID  string
	host        *HostMetrics
	traffic     *LiveTrafficSample
	persistedAt time.Time
}

type liveNodes struct {
	mu    sync.Mutex
	nodes map[string]*liveNode
}

// RecordNodeSample keeps a node's sample as its live evidence and stores its
// traffic at most every few seconds, and at once when the core process or
// node instance changes.
func (service *Service) RecordNodeSample(ctx context.Context, nodeID, instanceID string, sample NodeSample) error {
	service.live.mu.Lock()
	live := service.live.nodes[nodeID]
	if live == nil || live.instanceID != instanceID {
		live = &liveNode{instanceID: instanceID}
		service.live.nodes[nodeID] = live
	}
	live.host = sample.Host
	traffic := sample.Traffic
	if traffic == nil {
		live.traffic = nil
		service.live.mu.Unlock()
		return nil
	}
	current := &LiveTrafficSample{
		ActivationBundleID: traffic.ActivationBundleID, PID: traffic.PID, ProcessStartToken: traffic.ProcessStartToken,
		SampledAt: sample.SampledAt.UTC(), MemoryBytes: traffic.MemoryBytes, ActiveConnections: traffic.Connections,
		UploadTotal: traffic.UploadTotal, DownloadTotal: traffic.DownloadTotal, Accepted: true,
	}
	previous := live.traffic
	sameProcess := previous != nil && previous.PID == current.PID && previous.ProcessStartToken == current.ProcessStartToken &&
		previous.ActivationBundleID == current.ActivationBundleID
	if sameProcess && current.UploadTotal >= previous.UploadTotal && current.DownloadTotal >= previous.DownloadTotal {
		if elapsed := current.SampledAt.Sub(previous.SampledAt); elapsed > 0 && elapsed <= maximumLiveRateGap {
			upload := float64(current.UploadTotal-previous.UploadTotal) / elapsed.Seconds()
			download := float64(current.DownloadTotal-previous.DownloadTotal) / elapsed.Seconds()
			current.UploadBytesPerSecond, current.DownloadBytesPerSecond = &upload, &download
		}
	}
	persist := !sameProcess || live.persistedAt.IsZero() || current.SampledAt.Sub(live.persistedAt) >= trafficPersistInterval
	live.traffic = current
	service.live.mu.Unlock()
	if !persist {
		return nil
	}
	node, err := service.database.Node(ctx, nodeID)
	if err != nil {
		return err
	}
	start, end, err := TrafficPeriod(current.SampledAt, node.Settings.Traffic)
	if err != nil {
		return err
	}
	result, err := service.database.RecordTrafficSample(ctx, store.TrafficSampleInput{
		NodeID: nodeID, InstanceID: instanceID, ActivationBundleID: current.ActivationBundleID,
		PID: current.PID, ProcessStartToken: current.ProcessStartToken, SampledAt: current.SampledAt,
		PeriodStart: start, PeriodEnd: end, MemoryBytes: current.MemoryBytes, ActiveConnections: current.ActiveConnections,
		UploadTotal: current.UploadTotal, DownloadTotal: current.DownloadTotal,
	})
	if err != nil {
		return err
	}
	service.live.mu.Lock()
	defer service.live.mu.Unlock()
	if live.traffic == current {
		live.traffic.Accepted = result.Sample.Accepted
	}
	if result.Sample.Accepted && current.SampledAt.After(live.persistedAt) {
		live.persistedAt = current.SampledAt
	}
	return nil
}

// ForgetNode drops a node's live evidence, such as when it is removed.
func (service *Service) ForgetNode(nodeID string) {
	service.live.mu.Lock()
	defer service.live.mu.Unlock()
	delete(service.live.nodes, nodeID)
}

// NodeMetrics returns a node's live evidence with its recorded traffic in the
// current period. applied is the bundle the node reports, nil when none.
func (service *Service) NodeMetrics(ctx context.Context, nodeID string, applied *AppliedBundle) (MetricsSnapshot, error) {
	now := service.now().UTC()
	result := MetricsSnapshot{CollectedAt: now}
	usage, err := service.NodeTraffic(ctx, nodeID, now)
	if err != nil {
		return MetricsSnapshot{}, err
	}
	result.CurrentTrafficData, result.QuotaBytes, result.QuotaExceeded = usage.Period, usage.QuotaBytes, usage.QuotaExceeded
	service.live.mu.Lock()
	if live := service.live.nodes[nodeID]; live != nil {
		if live.host != nil && now.Sub(live.host.SampledAt) <= liveSampleLifetime {
			host := *live.host
			result.Host = &host
		}
		if live.traffic != nil && now.Sub(live.traffic.SampledAt) <= liveSampleLifetime {
			traffic := *live.traffic
			result.LiveSample = &traffic
		}
	}
	service.live.mu.Unlock()
	if applied == nil {
		result.ReasonCode = "not_applied"
		return result, nil
	}
	result.AppliedBundleID, result.MonitoringTier = applied.ID, applied.MonitoringTier
	if applied.MonitoringTier != "limited" {
		result.ReasonCode = "process_only"
		return result, nil
	}
	sample, err := service.database.LatestAcceptedTrafficSample(ctx, nodeID)
	if errors.Is(err, sql.ErrNoRows) {
		result.ReasonCode = "no_collector_sample"
		return result, nil
	}
	if err != nil {
		return MetricsSnapshot{}, err
	}
	result.LatestSample = &sample
	if sample.ActivationBundleID != applied.ID || now.Sub(sample.SampledAt) > liveSampleLifetime {
		result.ReasonCode = "stale_collector_sample"
		return result, nil
	}
	result.Available = true
	return result, nil
}
