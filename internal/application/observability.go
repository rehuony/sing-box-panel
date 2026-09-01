// SPDX-License-Identifier: GPL-3.0-or-later

package application

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	coreruntime "github.com/rehuony/sing-box-panel/internal/runtime"
	"github.com/rehuony/sing-box-panel/internal/store"
)

const gibibyte = int64(1 << 30)

// MetricsSnapshot never invents missing counters. Available identifies a fresh
// collector sample for the exact applied bundle. TrafficAvailable separately
// proves that at least one process-local counter delta contributes to the
// current period, so an unproven first lifetime counter is not exposed as zero.
type MetricsSnapshot struct {
	Available          bool                 `json:"available"`
	ReasonCode         string               `json:"reason_code,omitempty"`
	AppliedBundleID    string               `json:"applied_bundle_id,omitempty"`
	MonitoringTier     store.MonitoringTier `json:"monitoring_tier,omitempty"`
	CollectedAt        time.Time            `json:"collected_at"`
	CurrentTrafficData *store.TrafficPeriod `json:"current_traffic_period,omitempty"`
	LatestSample       *store.TrafficSample `json:"latest_sample,omitempty"`
	TrafficAvailable   bool                 `json:"traffic_available"`
	QuotaBytes         *int64               `json:"quota_bytes,omitempty"`
	QuotaExceeded      bool                 `json:"quota_exceeded"`
}

type TrafficSampleRetentionResult struct {
	Deleted int64     `json:"deleted"`
	Cutoff  time.Time `json:"cutoff"`
}

func (application *Application) Metrics(ctx context.Context) (MetricsSnapshot, error) {
	now := application.now().UTC()
	result := MetricsSnapshot{CollectedAt: now}
	bootstrap, err := application.database.Bootstrap(ctx)
	if err != nil {
		return MetricsSnapshot{}, err
	}
	result.AppliedBundleID = bootstrap.Hub.AppliedBundleID
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
	period, err := application.database.CurrentTrafficPeriod(ctx, now)
	if errors.Is(err, store.ErrTrafficPeriodNotFound) {
		result.ReasonCode = "no_collector_sample"
		return result, nil
	}
	if err != nil {
		return MetricsSnapshot{}, err
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
	result.CurrentTrafficData = &period
	var counters struct {
		TrafficEvidenceAvailable bool `json:"traffic_evidence_available"`
	}
	if err := json.Unmarshal(period.Counters, &counters); err != nil {
		return MetricsSnapshot{}, fmt.Errorf("decode traffic period evidence: %w", err)
	}
	result.TrafficAvailable = counters.TrafficEvidenceAvailable
	if configured := application.settings.Traffic.QuotaGiB; result.TrafficAvailable && configured != nil && *configured > 0 {
		quota := *configured * gibibyte
		result.QuotaBytes = &quota
		result.QuotaExceeded = period.InboundBytes+period.OutboundBytes >= quota
	}
	return result, nil
}

// CollectLimitedTrafficSample reads the configured loopback API for the exact
// observed process and persists a monotonic UTC-period sample.
func (application *Application) CollectLimitedTrafficSample(
	ctx context.Context,
	observation store.RuntimeObservation,
) (store.TrafficSampleResult, error) {
	material, err := application.LoadRuntimeMaterial(ctx, observation.ActivationBundleID)
	if err != nil {
		return store.TrafficSampleResult{}, err
	}
	if material.Activation.MonitoringTier != store.MonitoringLimited {
		return store.TrafficSampleResult{}, ErrMonitoringTierUnavailable
	}
	endpoint, err := coreruntime.ParseClashEndpoint(material.Bundle.StartupConfig)
	if err != nil {
		return store.TrafficSampleResult{}, err
	}
	client, err := coreruntime.NewClashClient(endpoint)
	if err != nil {
		return store.TrafficSampleResult{}, err
	}
	sample, err := client.Connections(ctx)
	if err != nil {
		return store.TrafficSampleResult{}, err
	}
	now := application.now().UTC()
	periodStart, periodEnd, err := naturalTrafficPeriod(now, application.settings.Traffic.PeriodMonths)
	if err != nil {
		return store.TrafficSampleResult{}, err
	}
	return application.database.RecordTrafficSample(ctx, store.TrafficSampleInput{
		ActivationBundleID: observation.ActivationBundleID,
		PID:                observation.PID, ProcessStartToken: observation.ProcessStartToken,
		SampledAt: now, PeriodStart: periodStart, PeriodEnd: periodEnd,
		MemoryBytes: sample.Memory, ActiveConnections: sample.Connections,
		UploadTotal: sample.UploadTotal, DownloadTotal: sample.DownloadTotal,
	})
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
	days := application.settings.Traffic.SampleRetentionDays
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
