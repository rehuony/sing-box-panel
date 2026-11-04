// SPDX-License-Identifier: GPL-3.0-or-later

package telemetry

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/settings"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/store"
)

const gibibyte = int64(1 << 30)

type TrafficSampleRetentionResult struct {
	Deleted int64     `json:"deleted"`
	Cutoff  time.Time `json:"cutoff"`
}

// CurrentTrafficUsage is shared by metrics and public subscription headers.
// A successful empty ledger is zero; a storage or settings failure remains an error.
func (service *Service) CurrentTrafficUsage(ctx context.Context, at time.Time) (store.TrafficPeriod, *int64, error) {
	policy, err := service.TrafficAccounting(ctx)
	if err != nil {
		return store.TrafficPeriod{}, nil, err
	}
	start, end, err := NaturalTrafficPeriod(at, policy.PeriodMonths)
	if err != nil {
		return store.TrafficPeriod{}, nil, err
	}
	period, err := service.database.AggregateTrafficPeriod(ctx, start, end, at)
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

// TrafficAccounting reads the traffic quota and period from the panel
// settings at each operation boundary.
func (service *Service) TrafficAccounting(ctx context.Context) (settings.Traffic, error) {
	if service.settings == nil {
		return settings.Traffic{}, errors.New("traffic policy is unavailable")
	}
	return service.settings.TrafficAccounting(ctx)
}

// NaturalTrafficPeriod returns the UTC calendar period of months that contains at.
func NaturalTrafficPeriod(at time.Time, months int) (time.Time, time.Time, error) {
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

func (application *Service) TrafficPeriod(ctx context.Context, periodID string) (store.TrafficPeriod, error) {
	return application.database.GetTrafficPeriod(ctx, periodID)
}

func (application *Service) ListTrafficPeriods(
	ctx context.Context,
	filter store.TrafficPeriodFilter,
) ([]store.TrafficPeriod, error) {
	return application.database.ListTrafficPeriods(ctx, filter)
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
