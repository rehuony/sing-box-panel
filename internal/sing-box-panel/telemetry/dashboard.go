// SPDX-License-Identifier: GPL-3.0-or-later

package telemetry

import (
	"context"
	"sync"
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

// DashboardSnapshot is one node's dashboard projection, every bucket of its
// history read anew.
func (application *Service) DashboardSnapshot(ctx context.Context, nodeID string) (DashboardStreamSnapshot, error) {
	return application.dashboardSnapshot(ctx, nodeID, nil)
}

// dashboardSnapshot reads the projection, reusing the closed buckets that
// history keeps when it is not nil.
func (application *Service) dashboardSnapshot(ctx context.Context, nodeID string, history *store.DashboardHistory) (DashboardStreamSnapshot, error) {
	collectedAt := application.now().UTC()
	oneDayAgo := collectedAt.Add(-24 * time.Hour)
	history1H, history24H, watermark, err := application.database.DashboardMetricsHistory(ctx, nodeID, collectedAt, history)
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

// dashboardInterval is how often a viewed node's dashboard is read again.
const dashboardInterval = 30 * time.Second

// dashboardIdleGrace is how long a node's dashboard and its history stay
// after the last viewer leaves, so a stream that renews itself, which every
// stream does each minute, finds them.
const dashboardIdleGrace = time.Minute

// dashboard is one node's shared dashboard feed and the history it reads
// from. Service.dashboardsMu guards viewers and idle.
type dashboard struct {
	feed    *Feed[DashboardStreamSnapshot]
	history store.DashboardHistory
	viewers int
	// idle counts the times the viewers dropped to none or rose from it; a
	// removal waits out the grace only for the count it was scheduled at.
	idle uint64
}

// SubscribeDashboard streams one node's dashboard projection. A node's
// dashboard is dropped once no one has viewed it for dashboardIdleGrace.
func (service *Service) SubscribeDashboard(nodeID string) (<-chan SnapshotResult[DashboardStreamSnapshot], func()) {
	service.dashboardsMu.Lock()
	entry := service.dashboards[nodeID]
	if entry == nil {
		entry = &dashboard{}
		entry.feed = NewFeed(dashboardInterval, func(ctx context.Context) (DashboardStreamSnapshot, error) {
			return service.dashboardSnapshot(ctx, nodeID, &entry.history)
		})
		entry.feed.Bind(service.root)
		service.dashboards[nodeID] = entry
	}
	if entry.viewers == 0 {
		entry.idle++
	}
	entry.viewers++
	service.dashboardsMu.Unlock()
	results, unsubscribe := entry.feed.Subscribe()
	var once sync.Once
	return results, func() {
		once.Do(func() {
			unsubscribe()
			service.dashboardsMu.Lock()
			defer service.dashboardsMu.Unlock()
			if entry.viewers--; entry.viewers > 0 {
				return
			}
			entry.idle++
			idle := entry.idle
			time.AfterFunc(dashboardIdleGrace, func() {
				service.dashboardsMu.Lock()
				defer service.dashboardsMu.Unlock()
				if entry.idle == idle && service.dashboards[nodeID] == entry {
					delete(service.dashboards, nodeID)
				}
			})
		})
	}
}

// resetDashboardHistory makes the next read of the dashboards of nodeID, or
// of every node when nodeID is empty, compute every bucket, after samples or
// transitions within their ranges were deleted.
func (service *Service) resetDashboardHistory(nodeID string) {
	service.dashboardsMu.Lock()
	defer service.dashboardsMu.Unlock()
	for id, entry := range service.dashboards {
		if nodeID == "" || id == nodeID {
			entry.history.Reset()
		}
	}
}
