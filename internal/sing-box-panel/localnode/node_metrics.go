// SPDX-License-Identifier: GPL-3.0-or-later

package localnode

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/rehuony/sing-box-panel/internal/sing-box-node/host"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/store"
)

// MetricsSnapshot separates live collector availability from the durable current
// period ledger. Every successful read includes recorded usage, even when zero.
type MetricsSnapshot struct {
	LiveSample         *LiveTrafficSample   `json:"live_sample,omitempty"`
	Host               *host.Snapshot       `json:"host,omitempty"`
	Available          bool                 `json:"available"`
	ReasonCode         string               `json:"reason_code,omitempty"`
	AppliedBundleID    string               `json:"applied_bundle_id,omitempty"`
	MonitoringTier     store.MonitoringTier `json:"monitoring_tier,omitempty"`
	CollectedAt        time.Time            `json:"collected_at"`
	CurrentTrafficData store.TrafficPeriod  `json:"current_traffic_period"`
	LatestSample       *store.TrafficSample `json:"latest_sample,omitempty"`
	QuotaBytes         *int64               `json:"quota_bytes,omitempty"`
	QuotaExceeded      bool                 `json:"quota_exceeded"`
}

func (application *Node) Metrics(ctx context.Context) (MetricsSnapshot, error) {
	now := application.now().UTC()
	result := MetricsSnapshot{CollectedAt: now, Host: application.hostSampler.Sample(application.settings.DataDir), LiveSample: application.collector.live.Load()}
	period, quota, err := application.telemetry.CurrentTrafficUsage(ctx, now)
	if err != nil {
		return MetricsSnapshot{}, err
	}
	result.CurrentTrafficData, result.QuotaBytes = period, quota
	if quota != nil {
		result.QuotaExceeded = period.InboundBytes+period.OutboundBytes >= *quota
	}
	hub, err := application.database.RuntimeHubState(ctx)
	if err != nil {
		return MetricsSnapshot{}, err
	}
	result.AppliedBundleID = hub.AppliedBundleID
	if result.AppliedBundleID == "" {
		result.ReasonCode = "not_applied"
		return result, nil
	}
	bundle, err := application.database.GetActivationBundle(ctx, result.AppliedBundleID)
	if err != nil {
		return MetricsSnapshot{}, err
	}
	result.MonitoringTier = bundle.MonitoringTier
	if bundle.MonitoringTier == store.MonitoringProcessOnly {
		result.ReasonCode = "process_only"
		return result, nil
	}
	if bundle.MonitoringTier != store.MonitoringLimited {
		return MetricsSnapshot{}, fmt.Errorf("invalid activation monitoring tier %q", bundle.MonitoringTier)
	}
	sample, err := application.database.LatestAcceptedTrafficSample(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		result.ReasonCode = "no_collector_sample"
		return result, nil
	}
	if err != nil {
		return MetricsSnapshot{}, err
	}
	result.LatestSample = &sample
	if sample.ActivationBundleID != bundle.ID || now.Sub(sample.SampledAt) > 30*time.Second {
		result.ReasonCode = "stale_collector_sample"
		return result, nil
	}
	result.Available = true

	return result, nil
}
func (application *Node) TrafficStatus(ctx context.Context) (MetricsSnapshot, error) {
	return application.Metrics(ctx)
}
