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

// liveNode is the newest evidence of one node and its recorded traffic.
type liveNode struct {
	instanceID string
	host       *HostMetrics
	// traffic is the newest traffic reading and the base of the next one's
	// rates; trafficCurrent reports whether the node's newest sample carried
	// it, so a sample without traffic hides it but keeps the base.
	traffic        *LiveTrafficSample
	trafficCurrent bool
	persistedAt    time.Time
	ledger         trafficLedger
}

// trafficLedger is a node's recorded traffic as the panel last stored or
// read it, so reads need no query: the period it last stored or read and
// its newest accepted sample. version counts the samples stored since the
// panel started; a read stores what it loaded only while no sample was
// stored after it began, so it never replaces newer totals.
type trafficLedger struct {
	period *store.TrafficPeriod
	// latest is the newest accepted sample once latestKnown; nil then means
	// the node has none.
	latest      *store.TrafficSample
	latestKnown bool
	version     uint64
}

// record keeps what storing a sample returned. A rejected sample changed no
// totals, and its period was summed only up to its own day, so it leaves the
// ledger as it is. The period was read in the storing transaction; when
// another sample was stored since version, either result may be the older
// one, so the period is read again on demand.
func (ledger *trafficLedger) record(result store.TrafficSampleResult, version uint64) {
	sample := result.Sample
	if !sample.Accepted {
		return
	}
	if ledger.version == version {
		period := result.Period
		ledger.period = &period
	} else {
		ledger.period = nil
	}
	// An accepted sample is newer than every sample accepted before it.
	if ledger.latest == nil || newerSample(sample, *ledger.latest) {
		ledger.latest, ledger.latestKnown = &sample, true
	}
	ledger.version++
}

// newerSample orders samples as the store's newest-first reads do.
func newerSample(sample, than store.TrafficSample) bool {
	return sample.SampledAt.After(than.SampledAt) || (sample.SampledAt.Equal(than.SampledAt) && sample.ID > than.ID)
}

type liveNodes struct {
	mu    sync.Mutex
	nodes map[string]*liveNode
}

// entry returns the node's live entry, adding an empty one. The caller holds
// mu.
func (nodes *liveNodes) entry(nodeID string) *liveNode {
	live := nodes.nodes[nodeID]
	if live == nil {
		live = &liveNode{}
		nodes.nodes[nodeID] = live
	}
	return live
}

// keepLoaded applies to the node's ledger what a read of the store loaded,
// unless the node was forgotten or a sample was stored after version, the
// ledger's version when the read began: that sample's totals are newer.
func (nodes *liveNodes) keepLoaded(nodeID string, live *liveNode, version uint64, apply func(*trafficLedger)) {
	nodes.mu.Lock()
	defer nodes.mu.Unlock()
	if nodes.nodes[nodeID] == live && live.ledger.version == version {
		apply(&live.ledger)
	}
}

// RecordNodeSample keeps a node's sample as its live evidence and stores its
// traffic at most every few seconds, and at once when the core process or
// node instance changes. A node reads its counters and reports on clocks of
// its own, so it may report one reading twice; the repeat changes nothing.
func (service *Service) RecordNodeSample(ctx context.Context, nodeID, instanceID string, sample NodeSample) error {
	service.live.mu.Lock()
	live := service.live.entry(nodeID)
	if live.instanceID != instanceID {
		// A recreated node database starts new live evidence; its recorded
		// traffic stays.
		live.instanceID, live.traffic, live.trafficCurrent, live.persistedAt = instanceID, nil, false, time.Time{}
	}
	live.host = sample.Host
	traffic := sample.Traffic
	if traffic == nil {
		live.trafficCurrent = false
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
	if sameProcess && current.SampledAt.Equal(previous.SampledAt) &&
		current.UploadTotal == previous.UploadTotal && current.DownloadTotal == previous.DownloadTotal {
		live.trafficCurrent = true
		service.live.mu.Unlock()
		return nil
	}
	if sameProcess && current.UploadTotal >= previous.UploadTotal && current.DownloadTotal >= previous.DownloadTotal {
		if elapsed := current.SampledAt.Sub(previous.SampledAt); elapsed > 0 && elapsed <= maximumLiveRateGap {
			upload := float64(current.UploadTotal-previous.UploadTotal) / elapsed.Seconds()
			download := float64(current.DownloadTotal-previous.DownloadTotal) / elapsed.Seconds()
			current.UploadBytesPerSecond, current.DownloadBytesPerSecond = &upload, &download
		}
	}
	persist := !sameProcess || live.persistedAt.IsZero() || current.SampledAt.Sub(live.persistedAt) >= trafficPersistInterval
	live.traffic, live.trafficCurrent = current, true
	version := live.ledger.version
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
	live.ledger.record(result, version)
	return nil
}

// ForgetNode drops a node's live evidence and recorded traffic, such as when
// it is removed.
func (service *Service) ForgetNode(nodeID string) {
	service.live.mu.Lock()
	defer service.live.mu.Unlock()
	delete(service.live.nodes, nodeID)
}

// trafficPeriod returns the node's recorded traffic in its period that
// contains at. The ledger answers while it holds that period; a period
// rollover or a changed reset day or period length is a different range,
// which is read once and kept.
func (service *Service) trafficPeriod(ctx context.Context, node store.Node, at time.Time) (store.TrafficPeriod, error) {
	start, end, err := TrafficPeriod(at, node.Settings.Traffic)
	if err != nil {
		return store.TrafficPeriod{}, err
	}
	service.live.mu.Lock()
	live := service.live.entry(node.ID)
	if cached := live.ledger.period; cached != nil && cached.PeriodStart.Equal(start) && cached.PeriodEnd.Equal(end) {
		period := *cached
		service.live.mu.Unlock()
		return period, nil
	}
	version := live.ledger.version
	service.live.mu.Unlock()
	period, err := service.database.AggregateTrafficPeriod(ctx, node.ID, start, end, at)
	if err != nil {
		return store.TrafficPeriod{}, err
	}
	cached := period
	service.live.keepLoaded(node.ID, live, version, func(ledger *trafficLedger) { ledger.period = &cached })
	return period, nil
}

// latestSample returns the node's newest accepted sample, or nil when it has
// none. It is read once; stored samples keep it current after that.
func (service *Service) latestSample(ctx context.Context, nodeID string) (*store.TrafficSample, error) {
	service.live.mu.Lock()
	live := service.live.entry(nodeID)
	if live.ledger.latestKnown {
		latest := copySample(live.ledger.latest)
		service.live.mu.Unlock()
		return latest, nil
	}
	version := live.ledger.version
	service.live.mu.Unlock()
	sample, err := service.database.LatestAcceptedTrafficSample(ctx, nodeID)
	var latest *store.TrafficSample
	switch {
	case errors.Is(err, sql.ErrNoRows):
	case err != nil:
		return nil, err
	default:
		latest = &sample
	}
	service.live.keepLoaded(nodeID, live, version, func(ledger *trafficLedger) { ledger.latest, ledger.latestKnown = latest, true })
	return copySample(latest), nil
}

func copySample(sample *store.TrafficSample) *store.TrafficSample {
	if sample == nil {
		return nil
	}
	value := *sample
	return &value
}

// NodeMetrics returns a node's live evidence with its recorded traffic in the
// current period. node is the node's record as the caller read it, whose
// settings give its period and quota; applied is the bundle the node
// reports, nil when none.
func (service *Service) NodeMetrics(ctx context.Context, node store.Node, applied *AppliedBundle) (MetricsSnapshot, error) {
	now := service.now().UTC()
	result := MetricsSnapshot{CollectedAt: now}
	usage, err := service.nodeTraffic(ctx, node, now)
	if err != nil {
		return MetricsSnapshot{}, err
	}
	result.CurrentTrafficData, result.QuotaBytes, result.QuotaExceeded = usage.Period, usage.QuotaBytes, usage.QuotaExceeded
	service.live.mu.Lock()
	if live := service.live.nodes[node.ID]; live != nil {
		if live.host != nil && now.Sub(live.host.SampledAt) <= liveSampleLifetime {
			host := *live.host
			result.Host = &host
		}
		if live.traffic != nil && live.trafficCurrent && now.Sub(live.traffic.SampledAt) <= liveSampleLifetime {
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
	sample, err := service.latestSample(ctx, node.ID)
	if err != nil {
		return MetricsSnapshot{}, err
	}
	if sample == nil {
		result.ReasonCode = "no_collector_sample"
		return result, nil
	}
	result.LatestSample = sample
	if sample.ActivationBundleID != applied.ID || now.Sub(sample.SampledAt) > liveSampleLifetime {
		result.ReasonCode = "stale_collector_sample"
		return result, nil
	}
	result.Available = true
	return result, nil
}
