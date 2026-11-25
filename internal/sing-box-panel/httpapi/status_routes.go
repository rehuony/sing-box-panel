// SPDX-License-Identifier: GPL-3.0-or-later

package httpapi

import (
	"errors"
	"net/http"
	"runtime"
	"time"

	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/releases"
)

func (handler *Handler) systemStatus(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, SystemStatus{
		Platform: SystemPlatform{OS: runtime.GOOS, Arch: runtime.GOARCH}, PanelVersion: handler.build.Version, StartedAt: handler.startedAt,
	})
}

// SystemRestart answers a restart request with the start of the process that
// is about to stop, so a client can tell the restarted panel from it.
type SystemRestart struct {
	StartedAt time.Time `json:"started_at"`
}

func (handler *Handler) restartSystem(w http.ResponseWriter, request *http.Request) {
	if _, ok := strictCoreQuery(w, request); !ok || !requireEmptyCoreBody(w, request) {
		return
	}
	if handler.restart == nil {
		writeProblem(w, request, http.StatusConflict, "restart_unsupported", "Restart unsupported",
			"The panel cannot restart itself on this platform; restart it where it runs.")
		return
	}
	if err := handler.restart(request.Context()); err != nil {
		switch {
		case errors.Is(err, releases.ErrUpdateRunning):
			writeProblem(w, request, http.StatusConflict, "update_in_progress", "Update in progress",
				"The panel's update restarts it; wait until the update has finished.")
			return
		case errors.Is(err, releases.ErrNodeUpdatesRunning):
			writeProblem(w, request, http.StatusConflict, "node_updates_in_progress", "Node updates in progress",
				"A restart would interrupt the node updates; wait until they finish.")
			return
		}
		writeProblem(w, request, http.StatusServiceUnavailable, "restart_failed", "Restart failed", "The panel could not prepare its restart.")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusAccepted, SystemRestart{StartedAt: handler.startedAt})
}
