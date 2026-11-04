// SPDX-License-Identifier: GPL-3.0-or-later

package localnode

import (
	"context"

	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/telemetry"
)

type MetricsStreamSnapshot struct {
	Metrics MetricsSnapshot `json:"metrics"`
	Runtime RuntimeStatus   `json:"runtime"`
}

// SetTelemetryContext binds shared reads to the server lifetime before serving.
func (a *Node) SetTelemetryContext(ctx context.Context) {
	a.metricsFeed.Bind(ctx)
	a.telemetry.Bind(ctx)
}

func (a *Node) metricsSnapshot(ctx context.Context) (MetricsStreamSnapshot, error) {
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

func (a *Node) SubscribeMetrics() (<-chan telemetry.SnapshotResult[MetricsStreamSnapshot], func()) {
	return a.metricsFeed.Subscribe()
}
