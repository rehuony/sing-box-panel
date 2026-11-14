// SPDX-License-Identifier: GPL-3.0-or-later
package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/fleet"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/telemetry"
)

// streamMetrics streams one node's live metrics. While it streams, the node
// samples once a second.
func (handler *Handler) streamMetrics(w http.ResponseWriter, request *http.Request) {
	if !handler.requireCommands(w, request) {
		return
	}
	if _, ok := strictCoreQuery(w, request); !ok {
		return
	}
	nodeID := requestNodeID(request)
	streamSnapshots(w, request, "metrics", time.Minute, maximumSnapshotFrameBytes, func(context.Context) (<-chan telemetry.SnapshotResult[fleet.MetricsStreamSnapshot], func()) {
		return handler.services.Fleet.SubscribeMetrics(nodeID)
	}, func(value fleet.MetricsStreamSnapshot) ([]byte, error) { return json.Marshal(value) })
}
