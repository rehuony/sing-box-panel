// SPDX-License-Identifier: GPL-3.0-or-later

package httpapi

import (
	"errors"
	"net/http"

	"github.com/rehuony/sing-box-panel/internal/selfupdate"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/fleet"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/releases"
)

func (handler *Handler) releasesRequest(w http.ResponseWriter, request *http.Request) bool {
	_, ok := strictCoreQuery(w, request)
	return ok
}

func (handler *Handler) systemUpdate(w http.ResponseWriter, request *http.Request) {
	if !handler.releasesRequest(w, request) {
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, handler.services.Releases.Status())
}

func (handler *Handler) checkSystemUpdate(w http.ResponseWriter, request *http.Request) {
	if !handler.releasesRequest(w, request) || !requireEmptyCoreBody(w, request) {
		return
	}
	status, err := handler.services.Releases.Check(request.Context())
	if err != nil {
		updateProblem(err).write(w, request)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, status)
}

func (handler *Handler) startSystemUpdate(w http.ResponseWriter, request *http.Request) {
	if !handler.releasesRequest(w, request) || !requireEmptyCoreBody(w, request) {
		return
	}
	status, err := handler.services.Releases.Update(request.Context())
	if err != nil {
		updateProblem(err).write(w, request)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusAccepted, status)
}

func updateProblem(err error) problemDetails {
	switch {
	case errors.Is(err, releases.ErrUpdateUnsupported):
		return problemDetails{http.StatusConflict, "update_unsupported", "Update unsupported", err.Error()}
	case errors.Is(err, releases.ErrUpdateRunning):
		return problemDetails{http.StatusConflict, "update_in_progress", "Update in progress", err.Error()}
	case errors.Is(err, releases.ErrNodeUpdatesRunning):
		return problemDetails{http.StatusConflict, "node_updates_in_progress", "Node updates in progress", err.Error()}
	case errors.Is(err, releases.ErrNoUpdate):
		return problemDetails{http.StatusConflict, "update_not_available", "No update available", err.Error()}
	default:
		return problemDetails{http.StatusServiceUnavailable, "update_check_failed", "Update check failed",
			"The newest release could not be read from GitHub (" + selfupdate.FailureCode(err) + ")."}
	}
}

func (handler *Handler) updateNodeRelease(w http.ResponseWriter, request *http.Request) {
	if !handler.fleetRequest(w, request) || !requireEmptyCoreBody(w, request) {
		return
	}
	result, err := handler.services.Fleet.UpdateNode(request.Context(), requestNodeID(request))
	if err != nil {
		nodeUpdateProblem(err).write(w, request)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, result)
}

// nodeUpdateProblem reports a failed node update; the single-node and the
// batch updates answer with the same problems.
func nodeUpdateProblem(err error) problemDetails {
	var failure *fleet.NodeUpdateFailure
	switch {
	case errors.Is(err, releases.ErrUpdateRunning):
		return problemDetails{http.StatusConflict, "update_in_progress", "Panel update in progress", err.Error()}
	case errors.Is(err, releases.ErrUnavailable):
		return problemDetails{http.StatusConflict, "node_update_unavailable", "Node update unavailable", err.Error()}
	case errors.Is(err, releases.ErrPlatformUnsupported):
		return problemDetails{http.StatusConflict, "node_platform_unsupported", "Node platform unsupported", err.Error()}
	case errors.Is(err, fleet.ErrNodeUpgradeRequired):
		return problemDetails{http.StatusConflict, "node_upgrade_required", "Node upgrade required", err.Error()}
	case errors.Is(err, fleet.ErrNodeRestartRequired):
		return problemDetails{http.StatusConflict, "node_restart_required", "Node restart required", err.Error()}
	case errors.Is(err, fleet.ErrNodeUpdateTimeout):
		return problemDetails{http.StatusServiceUnavailable, "node_update_timeout", "Node did not come back", err.Error()}
	case errors.As(err, &failure):
		return problemDetails{http.StatusServiceUnavailable, "node_update_failed", "Node update failed", failure.Error()}
	}
	var release *releases.Failure
	if errors.As(err, &release) {
		return problemDetails{http.StatusServiceUnavailable, "node_release_unavailable", "Node release unavailable",
			"The panel could not download and verify its node release (" + release.Code + ")."}
	}
	return nodeCommandProblem("node_update_failed", err)
}
