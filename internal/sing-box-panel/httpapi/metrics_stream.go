// SPDX-License-Identifier: GPL-3.0-or-later
package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/localnode"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/telemetry"
)

func (handler *Handler) streamMetrics(w http.ResponseWriter, request *http.Request) {
	if !handler.requireCommands(w, request) {
		return
	}
	if _, ok := strictCoreQuery(w, request); !ok {
		return
	}
	streamSnapshots(w, request, "metrics", time.Minute, func(context.Context) (<-chan telemetry.SnapshotResult[localnode.MetricsStreamSnapshot], func()) {
		return handler.services.Node.SubscribeMetrics()
	}, func(value localnode.MetricsStreamSnapshot) ([]byte, error) { return json.Marshal(value) })
}
