// SPDX-License-Identifier: GPL-3.0-or-later

package httpapi

import "net/http"

func (handler *Handler) systemStatus(w http.ResponseWriter, request *http.Request) {
	if handler.status == nil {
		writeJSON(w, http.StatusOK, SystemStatus{PanelVersion: handler.build.Version, ConfigurationState: "unavailable"})
		return
	}
	status, err := handler.status.SystemStatus(request.Context())
	if err != nil {
		writeProblem(w, request, http.StatusServiceUnavailable, "status_unavailable", "Status unavailable", "The current system status could not be loaded.")
		return
	}
	writeJSON(w, http.StatusOK, status)
}

func (handler *Handler) dashboardContext(w http.ResponseWriter, request *http.Request) {
	if handler.status == nil {
		writeProblem(w, request, http.StatusServiceUnavailable, "dashboard_unavailable", "Dashboard unavailable", "The control-plane context is not ready.")
		return
	}
	context, err := handler.status.DashboardContext(request.Context())
	if err != nil {
		writeProblem(w, request, http.StatusServiceUnavailable, "dashboard_unavailable", "Dashboard unavailable", "The control-plane context could not be loaded.")
		return
	}
	writeJSON(w, http.StatusOK, context)
}
