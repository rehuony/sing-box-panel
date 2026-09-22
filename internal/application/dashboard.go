// SPDX-License-Identifier: GPL-3.0-or-later

package application

import (
	"context"
	"time"

	"github.com/rehuony/sing-box-panel/internal/store"
)

const dashboardRuntimeHistoryLimit = 4096

// DashboardStreamSnapshot is the complete, cacheable dashboard projection.
// Live host and traffic evidence continue to arrive through the metrics stream.
type DashboardStreamSnapshot struct {
	CollectedAt time.Time            `json:"collected_at"`
	History1H   store.MetricsHistory `json:"history_1h"`
	History24H  store.MetricsHistory `json:"history_24h"`
	Runtime24H  RuntimeHistoryPage   `json:"runtime_24h"`
	Activity    store.PanelLogPage   `json:"activity"`
}

func (application *Application) DashboardSnapshot(ctx context.Context) (DashboardStreamSnapshot, error) {
	collectedAt := application.now().UTC()
	oneHourAgo := collectedAt.Add(-time.Hour)
	oneDayAgo := collectedAt.Add(-24 * time.Hour)

	history1H, err := application.MetricsHistory(ctx, store.MetricsHistoryFilter{
		From: oneHourAgo, To: collectedAt, BucketSeconds: 60,
	})
	if err != nil {
		return DashboardStreamSnapshot{}, err
	}
	history24H, err := application.MetricsHistory(ctx, store.MetricsHistoryFilter{
		From: oneDayAgo, To: collectedAt, BucketSeconds: 300,
	})
	if err != nil {
		return DashboardStreamSnapshot{}, err
	}
	runtime24H, err := application.dashboardRuntimeHistory(ctx, oneDayAgo, collectedAt)
	if err != nil {
		return DashboardStreamSnapshot{}, err
	}
	activity, err := application.PanelLogs(ctx, store.PanelLogFilter{Limit: 2})
	if err != nil {
		return DashboardStreamSnapshot{}, err
	}
	return DashboardStreamSnapshot{
		CollectedAt: collectedAt,
		History1H:   history1H,
		History24H:  history24H,
		Runtime24H:  runtime24H,
		Activity:    activity,
	}, nil
}

func (application *Application) dashboardRuntimeHistory(
	ctx context.Context,
	from time.Time,
	to time.Time,
) (RuntimeHistoryPage, error) {
	request := RuntimeHistoryRequest{From: &from, To: &to, Limit: 200}
	combined := RuntimeHistoryPage{Items: []RuntimeTransition{}}
	for len(combined.Items) < dashboardRuntimeHistoryLimit {
		request.Limit = min(200, dashboardRuntimeHistoryLimit-len(combined.Items))
		page, err := application.RuntimeHistory(ctx, request)
		if err != nil {
			return RuntimeHistoryPage{}, err
		}
		if combined.Preceding == nil {
			combined.Preceding = page.Preceding
			combined.HistoryStartedAt = page.HistoryStartedAt
		}
		combined.Items = append(combined.Items, page.Items...)
		combined.Next = page.Next
		if page.Next == nil || len(combined.Items) >= dashboardRuntimeHistoryLimit {
			break
		}
		request.Cursor = &store.RuntimeTransitionCursor{OccurredAt: page.Next.OccurredAt, ID: page.Next.ID}
	}
	return combined, nil
}
