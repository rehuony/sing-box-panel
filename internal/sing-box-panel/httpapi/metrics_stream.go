// SPDX-License-Identifier: GPL-3.0-or-later
package httpapi

import "net/http"

func (handler *Handler) streamMetrics(w http.ResponseWriter, request *http.Request) {
	if !handler.requireCommands(w, request) {
		return
	}
	if _, ok := strictCoreQuery(w, request); !ok {
		return
	}
	writeNodeTelemetryUnavailable(w, request, "metrics_unavailable", "Metrics unavailable")
}
