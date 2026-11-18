// SPDX-License-Identifier: GPL-3.0-or-later
package httpapi

import (
	"context"
	"net/http"
	"time"

	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/telemetry"
)

// dashboardStreamLifetime ends each stream so the client reauthenticates.
const dashboardStreamLifetime = time.Minute

func (handler *Handler) streamDashboard(w http.ResponseWriter, request *http.Request) {
	if _, ok := strictCoreQuery(w, request); !ok {
		return
	}
	nodeID := requestNodeID(request)
	streamSnapshots(w, request, "dashboard", dashboardStreamLifetime, maximumSnapshotFrameBytes, func(context.Context) (<-chan telemetry.SnapshotResult[telemetry.DashboardStreamSnapshot], func()) {
		return handler.services.Telemetry.SubscribeDashboard(nodeID)
	}, encodeDashboardSnapshot)
}
