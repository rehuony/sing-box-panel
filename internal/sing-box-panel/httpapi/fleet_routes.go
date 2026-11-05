// SPDX-License-Identifier: GPL-3.0-or-later

package httpapi

import (
	"errors"
	"net/http"

	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/fleet"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/store"
)

const maximumNodeRequestBytes = 4 << 10

func (handler *Handler) fleetRequest(w http.ResponseWriter, request *http.Request) bool {
	if handler.services.Fleet == nil {
		writeProblem(w, request, http.StatusServiceUnavailable, "application_unavailable", "Application unavailable", "Node services are not ready.")
		return false
	}
	_, ok := strictCoreQuery(w, request)
	return ok
}

type nodeNameRequest struct {
	Name string `json:"name"`
}

func (handler *Handler) listNodes(w http.ResponseWriter, request *http.Request) {
	if !handler.fleetRequest(w, request) {
		return
	}
	nodes, err := handler.services.Fleet.Nodes(request.Context())
	if err != nil {
		writeFleetProblem(w, request, err)
		return
	}
	writeJSON(w, http.StatusOK, struct {
		Items []fleet.Node `json:"items"`
	}{nodes})
}

func (handler *Handler) createNode(w http.ResponseWriter, request *http.Request) {
	if !handler.fleetRequest(w, request) {
		return
	}
	var input nodeNameRequest
	if !decodeStrictRequest(w, request, maximumNodeRequestBytes, &input) {
		return
	}
	enrollment, err := handler.services.Fleet.AddNode(request.Context(), input.Name)
	if err != nil {
		writeFleetProblem(w, request, err)
		return
	}
	writeEnrollment(w, http.StatusCreated, enrollment)
}

func (handler *Handler) getNode(w http.ResponseWriter, request *http.Request, id string) {
	if !handler.fleetRequest(w, request) {
		return
	}
	node, err := handler.services.Fleet.Node(request.Context(), id)
	if err != nil {
		writeFleetProblem(w, request, err)
		return
	}
	writeJSON(w, http.StatusOK, node)
}

func (handler *Handler) updateNode(w http.ResponseWriter, request *http.Request, id string) {
	if !handler.fleetRequest(w, request) {
		return
	}
	var input nodeNameRequest
	if !decodeStrictRequest(w, request, maximumNodeRequestBytes, &input) {
		return
	}
	node, err := handler.services.Fleet.RenameNode(request.Context(), id, input.Name)
	if err != nil {
		writeFleetProblem(w, request, err)
		return
	}
	writeJSON(w, http.StatusOK, node)
}

func (handler *Handler) deleteNode(w http.ResponseWriter, request *http.Request, id string) {
	if !handler.fleetRequest(w, request) {
		return
	}
	if err := handler.services.Fleet.DeleteNode(request.Context(), id); err != nil {
		writeFleetProblem(w, request, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (handler *Handler) createNodeEnrollment(w http.ResponseWriter, request *http.Request, id string) {
	if !handler.fleetRequest(w, request) {
		return
	}
	enrollment, err := handler.services.Fleet.IssueEnrollment(request.Context(), id)
	if err != nil {
		writeFleetProblem(w, request, err)
		return
	}
	writeEnrollment(w, http.StatusCreated, enrollment)
}

// writeEnrollment returns a one-time code, which must never be cached.
func writeEnrollment(w http.ResponseWriter, statusCode int, enrollment fleet.Enrollment) {
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, statusCode, enrollment)
}

func writeFleetProblem(w http.ResponseWriter, request *http.Request, err error) {
	switch {
	case errors.Is(err, store.ErrNodeNotFound):
		writeProblem(w, request, http.StatusNotFound, "node_not_found", "Node not found", "The node does not exist.")
	case errors.Is(err, store.ErrNodeNameConflict):
		writeProblem(w, request, http.StatusConflict, "node_name_conflict", "Node name in use", "Another node already uses this name.")
	case errors.Is(err, fleet.ErrNameInvalid):
		writeProblem(w, request, http.StatusUnprocessableEntity, "node_name_invalid", "Node name invalid", err.Error())
	case errors.Is(err, store.ErrNodeSettingsConflict):
		writeProblem(w, request, http.StatusPreconditionFailed, "node_settings_conflict", "Node settings changed", "Reload the node's settings before saving.")
	case errors.Is(err, store.ErrNodeSettingsInvalid):
		writeProblem(w, request, http.StatusUnprocessableEntity, "node_settings_invalid", "Node settings invalid", err.Error())
	case errors.Is(err, fleet.ErrEndpointRequired):
		writeProblem(w, request, http.StatusConflict, "node_endpoint_required", "Node endpoint required", err.Error())
	default:
		writeProblem(w, request, http.StatusInternalServerError, "node_operation_failed", "Node operation failed", "The node operation could not be completed.")
	}
}

func (handler *Handler) nodeSettings(w http.ResponseWriter, request *http.Request) {
	if !handler.fleetRequest(w, request) {
		return
	}
	result, err := handler.services.Fleet.NodeSettings(request.Context(), requestNodeID(request))
	if err != nil {
		writeFleetProblem(w, request, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, result)
}

func (handler *Handler) saveNodeSettings(w http.ResponseWriter, request *http.Request) {
	if !handler.fleetRequest(w, request) {
		return
	}
	var input fleet.NodeSettings
	if !decodeStrictRequest(w, request, 4096, &input) {
		return
	}
	result, err := handler.services.Fleet.SaveNodeSettings(request.Context(), requestNodeID(request), input)
	if err != nil {
		writeFleetProblem(w, request, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, result)
}
