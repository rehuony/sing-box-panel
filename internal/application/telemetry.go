// SPDX-License-Identifier: GPL-3.0-or-later

package application

import "context"

type MetricsStreamSnapshot struct {
	Metrics MetricsSnapshot `json:"metrics"`
	Runtime RuntimeStatus   `json:"runtime"`
}

// SetTelemetryContext binds shared reads to the server lifetime before serving.
func (a *Application) SetTelemetryContext(ctx context.Context) {
	a.metricsFeed.root = ctx
	a.dashboardFeed.root = ctx
}

func (a *Application) metricsSnapshot(ctx context.Context) (MetricsStreamSnapshot, error) {
	metrics, err := a.Metrics(ctx)
	if err != nil {
		return MetricsStreamSnapshot{}, err
	}
	runtime, err := a.RuntimeStatus(ctx)
	if err != nil {
		return MetricsStreamSnapshot{}, err
	}
	if live := metrics.LiveSample; live != nil {
		if runtime.Running == nil || runtime.ObservationState != "running" || runtime.Running.PID != live.PID || runtime.Running.ProcessStartToken != live.ProcessStartToken || runtime.Running.ActivationBundleID != live.ActivationBundleID {
			metrics.LiveSample = nil
		}
	}
	return MetricsStreamSnapshot{Metrics: metrics, Runtime: runtime}, nil
}

func (a *Application) SubscribeMetrics() (<-chan SnapshotResult[MetricsStreamSnapshot], func()) {
	return a.metricsFeed.subscribe()
}
func (a *Application) SubscribeDashboard() (<-chan SnapshotResult[DashboardStreamSnapshot], func()) {
	return a.dashboardFeed.subscribe()
}
