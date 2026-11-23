// SPDX-License-Identifier: GPL-3.0-or-later

package telemetry

import (
	"context"
	"time"

	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/store"
)

const dashboardRuntimeHistoryLimit = 4096

// DashboardStreamSnapshot is the complete, cacheable dashboard projection.
// Live host and traffic evidence continue to arrive through the metrics stream.
type DashboardStreamSnapshot struct {
	PersistedThrough *time.Time           `json:"persisted_through"`
	CollectedAt      time.Time            `json:"collected_at"`
	History1H        store.MetricsHistory `json:"history_1h"`
	History24H       store.MetricsHistory `json:"history_24h"`
	Runtime24H       RuntimeHistoryPage   `json:"runtime_24h"`
	Activity         store.PanelLogPage   `json:"activity"`
}

// DashboardSnapshot is one node's dashboard projection.
func (application *Service) DashboardSnapshot(ctx context.Context, nodeID string) (DashboardStreamSnapshot, error) {
	collectedAt := application.now().UTC()
	oneDayAgo := collectedAt.Add(-24 * time.Hour)
	history1H, history24H, watermark, err := application.database.DashboardMetricsHistory(ctx, nodeID, collectedAt)
	if err != nil {
		return DashboardStreamSnapshot{}, err
	}
	runtime24H, err := application.dashboardRuntimeHistory(ctx, nodeID, oneDayAgo, collectedAt)
	if err != nil {
		return DashboardStreamSnapshot{}, err
	}
	activity, err := application.PanelLogs(ctx, store.PanelLogFilter{Limit: 2, NodeID: nodeID})
	if err != nil {
		return DashboardStreamSnapshot{}, err
	}
	return DashboardStreamSnapshot{
		CollectedAt:      collectedAt,
		PersistedThrough: watermark,
		History1H:        history1H,
		History24H:       history24H,
		Runtime24H:       runtime24H,
		Activity:         activity,
	}, nil
}

func (application *Service) dashboardRuntimeHistory(
	ctx context.Context,
	nodeID string,
	from time.Time,
	to time.Time,
) (RuntimeHistoryPage, error) {
	request := RuntimeHistoryRequest{NodeID: nodeID, From: &from, To: &to, Limit: 200}
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

// SubscribeDashboard streams one node's dashboard projection.
func (service *Service) SubscribeDashboard(nodeID string) (<-chan SnapshotResult[DashboardStreamSnapshot], func()) {
	service.dashboardsMu.Lock()
	feed := service.dashboards[nodeID]
	if feed == nil {
		feed = NewFeed(30*time.Second, func(ctx context.Context) (DashboardStreamSnapshot, error) {
			return service.DashboardSnapshot(ctx, nodeID)
		})
		feed.Bind(service.root)
		service.dashboards[nodeID] = feed
	}
	service.dashboardsMu.Unlock()
	return feed.Subscribe()
}
