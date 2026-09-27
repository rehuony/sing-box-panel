// SPDX-License-Identifier: GPL-3.0-or-later
package httpapi

import (
	"context"
	"net/http"
	"time"

	"github.com/rehuony/sing-box-panel/internal/application"
)

const (
	dashboardStreamInterval = 30 * time.Second
	dashboardStreamLifetime = time.Minute
)

func (handler *Handler) streamDashboard(w http.ResponseWriter, request *http.Request) {
	if !handler.requireCommands(w, request) {
		return
	}
	if _, ok := strictCoreQuery(w, request); !ok {
		return
	}
	streamSnapshots(w, request, "dashboard", dashboardStreamLifetime, func(context.Context) (<-chan application.SnapshotResult[application.DashboardStreamSnapshot], func()) {
		return handler.commands.SubscribeDashboard()
	}, encodeDashboardSnapshot)
}
