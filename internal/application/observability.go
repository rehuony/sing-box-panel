// SPDX-License-Identifier: GPL-3.0-or-later

package application

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/rehuony/sing-box-panel/internal/hostmetrics"
	"github.com/rehuony/sing-box-panel/internal/settings"
	"github.com/rehuony/sing-box-panel/internal/store"
)

const gibibyte = int64(1 << 30)

// MetricsSnapshot separates live collector availability from the durable current
// period ledger. Every successful read includes recorded usage, even when zero.
type MetricsSnapshot struct {
	LiveSample         *LiveTrafficSample    `json:"live_sample,omitempty"`
	Host               *hostmetrics.Snapshot `json:"host,omitempty"`
	Available          bool                  `json:"available"`
	ReasonCode         string                `json:"reason_code,omitempty"`
	AppliedBundleID    string                `json:"applied_bundle_id,omitempty"`
	MonitoringTier     store.MonitoringTier  `json:"monitoring_tier,omitempty"`
	CollectedAt        time.Time             `json:"collected_at"`
	CurrentTrafficData store.TrafficPeriod   `json:"current_traffic_period"`
	LatestSample       *store.TrafficSample  `json:"latest_sample,omitempty"`
	QuotaBytes         *int64                `json:"quota_bytes,omitempty"`
	QuotaExceeded      bool                  `json:"quota_exceeded"`
}

type TrafficSampleRetentionResult struct {
	Deleted int64     `json:"deleted"`
	Cutoff  time.Time `json:"cutoff"`
}

func (application *Application) Metrics(ctx context.Context) (MetricsSnapshot, error) {
	now := application.now().UTC()
	result := MetricsSnapshot{CollectedAt: now, Host: application.hostSampler.Sample(application.settings.DataDir), LiveSample: application.collector.live.Load()}
	period, quota, err := application.currentTrafficUsage(ctx, now)
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

// currentTrafficUsage is shared by metrics and public subscription headers.
// A successful empty ledger is zero; a storage or settings failure remains an error.
func (application *Application) currentTrafficUsage(ctx context.Context, at time.Time) (store.TrafficPeriod, *int64, error) {
	policy, err := application.trafficAccounting(ctx)
	if err != nil {
		return store.TrafficPeriod{}, nil, err
	}
	start, end, err := naturalTrafficPeriod(at, policy.PeriodMonths)
	if err != nil {
		return store.TrafficPeriod{}, nil, err
	}
	period, err := application.database.AggregateTrafficPeriod(ctx, start, end, at)
	if err != nil {
		return store.TrafficPeriod{}, nil, err
	}
	var quota *int64
	if policy.QuotaGiB != nil && *policy.QuotaGiB > 0 {
		value := *policy.QuotaGiB * gibibyte
		quota = &value
	}
	return period, quota, nil
}

func (application *Application) trafficQuota(ctx context.Context) (*int64, error) {
	policy, err := application.trafficAccounting(ctx)
	return policy.QuotaGiB, err
}

func (application *Application) trafficAccounting(ctx context.Context) (settings.Traffic, error) {
	if application.settingsPath == "" {
		value := application.settings.Traffic
		if value.PeriodMonths == 0 {
			value.PeriodMonths = 1
		}
		return value, settings.ValidateTrafficQuota(value.QuotaGiB)
	}
	lock, err := settings.Lock(ctx, application.settingsPath)
	if err != nil {
		return settings.Traffic{}, err
	}
	defer lock.Close()
	if err := application.recoverSettingsFile(ctx); err != nil {
		return settings.Traffic{}, err
	}
	return settings.LoadTrafficAccounting(application.settingsPath)
}

func naturalTrafficPeriod(at time.Time, months int) (time.Time, time.Time, error) {
	if at.IsZero() || months < 1 || months > 120 {
		return time.Time{}, time.Time{}, fmt.Errorf("invalid traffic period configuration")
	}
	at = at.UTC()
	monthIndex := (at.Year()-1970)*12 + int(at.Month()) - 1
	startIndex := monthIndex - floorMod(monthIndex, months)
	start := time.Date(1970, time.January, 1, 0, 0, 0, 0, time.UTC).AddDate(0, startIndex, 0)
	return start, start.AddDate(0, months, 0), nil
}

func floorMod(value, divisor int) int {
	remainder := value % divisor
	if remainder < 0 {
		return remainder + divisor
	}
	return remainder
}

func (application *Application) TrafficStatus(ctx context.Context) (MetricsSnapshot, error) {
	return application.Metrics(ctx)
}

func (application *Application) MetricsHistory(
	ctx context.Context,
	filter store.MetricsHistoryFilter,
) (store.MetricsHistory, error) {
	return application.database.MetricsHistory(ctx, filter)
}

// EnforceTrafficSampleRetention removes only raw samples. Aggregated traffic
// periods are intentionally retained as the long-lived accounting record.
func (application *Application) EnforceTrafficSampleRetention(ctx context.Context) (TrafficSampleRetentionResult, error) {
	effective, err := application.EffectiveSettings(ctx)
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

func (application *Application) TrafficPeriod(ctx context.Context, periodID string) (store.TrafficPeriod, error) {
	return application.database.GetTrafficPeriod(ctx, periodID)
}

func (application *Application) ListTrafficPeriods(
	ctx context.Context,
	filter store.TrafficPeriodFilter,
) ([]store.TrafficPeriod, error) {
	return application.database.ListTrafficPeriods(ctx, filter)
}

func (application *Application) ListTrafficPeriodPage(
	ctx context.Context,
	filter store.TrafficPeriodFilter,
) (store.TrafficPeriodPage, error) {
	return application.database.ListTrafficPeriodPage(ctx, filter)
}

func IsTrafficPeriodNotFound(err error) bool {
	return errors.Is(err, store.ErrTrafficPeriodNotFound)
}
