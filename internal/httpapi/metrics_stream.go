// SPDX-License-Identifier: GPL-3.0-or-later
package httpapi

import (
	"context"
	"encoding/json"
	"github.com/rehuony/sing-box-panel/internal/application"
	"net/http"
	"time"
)

func (handler *Handler) streamMetrics(w http.ResponseWriter, request *http.Request) {
	if !handler.requireCommands(w, request) {
		return
	}
	if _, ok := strictCoreQuery(w, request); !ok {
		return
	}
	streamSnapshots(w, request, "metrics", time.Minute, func(context.Context) (<-chan application.SnapshotResult[application.MetricsStreamSnapshot], func()) {
		return handler.commands.SubscribeMetrics()
	}, func(value application.MetricsStreamSnapshot) ([]byte, error) { return json.Marshal(value) })
}
