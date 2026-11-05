// SPDX-License-Identifier: GPL-3.0-or-later

package httpapi

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/fleet"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/store"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/telemetry"
)

func (handler *Handler) coreRuntimeStatus(w http.ResponseWriter, request *http.Request) {
	if !handler.requireCommands(w, request) {
		return
	}
	if _, ok := strictCoreQuery(w, request); !ok {
		return
	}
	status, err := handler.services.Fleet.RuntimeStatus(request.Context(), requestNodeID(request))
	if err != nil {
		writeNodeProblem(w, request, "runtime_status_failed", err)
		return
	}
	writeJSON(w, http.StatusOK, status)
}

func (handler *Handler) coreRuntimeHistory(w http.ResponseWriter, request *http.Request) {
	if !handler.requireCommands(w, request) {
		return
	}
	query, ok := strictCoreQuery(
		w,
		request,
		"from", "to", "state", "reason", "activation_bundle_id",
		"before_time", "before_id", "limit",
	)
	if !ok {
		return
	}
	from, ok := optionalHTTPTime(w, request, query.Get("from"), "from")
	if !ok {
		return
	}
	to, ok := optionalHTTPTime(w, request, query.Get("to"), "to")
	if !ok {
		return
	}
	if from != nil && to != nil && !to.After(*from) {
		writeProblem(w, request, http.StatusBadRequest, "runtime_history_range_invalid", "Runtime history range invalid", "to must be later than from.")
		return
	}
	cursor, ok := runtimeTransitionCursor(
		w,
		request,
		query.Get("before_time"),
		query.Get("before_id"),
	)
	if !ok {
		return
	}
	limit, ok := optionalLimit(w, request)
	if !ok {
		return
	}
	state := store.RuntimeTransitionState(query.Get("state"))
	if !validOptionalRuntimeTransitionState(state) ||
		!validOptionalRuntimeTransitionReason(query.Get("reason")) ||
		(query.Get("activation_bundle_id") != "" && !store.ValidIdentifier(query.Get("activation_bundle_id"))) {
		writeProblem(w, request, http.StatusBadRequest, "runtime_history_filter_invalid", "Runtime history filter invalid", "The runtime history filter contains an unsupported value.")
		return
	}
	page, err := handler.services.Telemetry.RuntimeHistory(request.Context(), telemetry.RuntimeHistoryRequest{
		NodeID:             requestNodeID(request),
		From:               from,
		To:                 to,
		State:              state,
		Reason:             query.Get("reason"),
		ActivationBundleID: query.Get("activation_bundle_id"),
		Cursor:             cursor,
		Limit:              limit,
	})
	if err != nil {
		writeProblem(w, request, http.StatusInternalServerError, "runtime_history_read_failed", "Runtime history operation failed", "The runtime transition records could not be listed.")
		return
	}
	writeJSON(w, http.StatusOK, page)
}

func validOptionalRuntimeTransitionState(state store.RuntimeTransitionState) bool {
	switch state {
	case "", store.RuntimeTransitionRunning, store.RuntimeTransitionStopped,
		store.RuntimeTransitionFailed, store.RuntimeTransitionUnknown:
		return true
	default:
		return false
	}
}

func validOptionalRuntimeTransitionReason(reason string) bool {
	if reason == "" {
		return true
	}
	if len(reason) > 128 || strings.TrimSpace(reason) != reason {
		return false
	}
	for index, character := range reason {
		if index == 0 && (character < 'a' || character > 'z') {
			return false
		}
		if (character >= 'a' && character <= 'z') || (character >= '0' && character <= '9') ||
			character == '_' || character == '-' || character == '.' {
			continue
		}
		return false
	}
	return true
}

func runtimeTransitionCursor(
	w http.ResponseWriter,
	request *http.Request,
	rawTime string,
	rawID string,
) (*store.RuntimeTransitionCursor, bool) {
	if rawTime == "" && rawID == "" {
		return nil, true
	}
	if rawTime == "" || rawID == "" {
		writeProblem(w, request, http.StatusBadRequest, "query_invalid", "Query invalid", "before_time and before_id must be supplied together.")
		return nil, false
	}
	occurredAt, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(rawTime))
	if err != nil {
		writeProblem(w, request, http.StatusBadRequest, "query_invalid", "Query invalid", "before_time must be an RFC 3339 timestamp.")
		return nil, false
	}
	identifier, err := strconv.ParseInt(strings.TrimSpace(rawID), 10, 64)
	if err != nil || identifier < 1 {
		writeProblem(w, request, http.StatusBadRequest, "query_invalid", "Query invalid", "before_id must be a positive integer.")
		return nil, false
	}
	return &store.RuntimeTransitionCursor{OccurredAt: occurredAt.UTC(), ID: identifier}, true
}

func (handler *Handler) checkStartup(w http.ResponseWriter, request *http.Request) {
	if !handler.requireCommands(w, request) {
		return
	}
	if _, ok := strictCoreQuery(w, request); !ok {
		return
	}
	var input struct {
		StartupArtifactID string `json:"startup_artifact_id"`
	}
	if !decodeStrictRequest(w, request, maximumRuntimeRequestBytes, &input) {
		return
	}
	if !store.ValidIdentifier(input.StartupArtifactID) {
		writeProblem(w, request, http.StatusUnprocessableEntity, "startup_artifact_id_invalid", "Startup artifact ID invalid", "startup_artifact_id must identify one immutable candidate.")
		return
	}
	result, err := handler.services.Fleet.CheckStartup(request.Context(), requestNodeID(request), input.StartupArtifactID)
	if err != nil {
		writeNodeProblem(w, request, "startup_check_failed", err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (handler *Handler) enableCoreArtifact(w http.ResponseWriter, request *http.Request, id string) {
	if !handler.requireCommands(w, request) {
		return
	}
	if _, ok := strictCoreQuery(w, request); !ok || !requireEmptyCoreBody(w, request) {
		return
	}
	result, err := handler.services.Fleet.EnableCore(request.Context(), requestNodeID(request), id)
	if err != nil {
		writeNodeProblem(w, request, "core_enable_failed", err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (handler *Handler) disableCoreArtifact(w http.ResponseWriter, request *http.Request, id string) {
	if !handler.requireCommands(w, request) {
		return
	}
	if _, ok := strictCoreQuery(w, request); !ok || !requireEmptyCoreBody(w, request) {
		return
	}
	result, err := handler.services.Fleet.DisableCore(request.Context(), requestNodeID(request), id)
	if err != nil {
		writeNodeProblem(w, request, "core_disable_failed", err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (handler *Handler) activateCore(w http.ResponseWriter, request *http.Request) {
	if !handler.requireCommands(w, request) {
		return
	}
	if _, ok := strictCoreQuery(w, request); !ok {
		return
	}
	var input struct {
		StartupArtifactID string `json:"startup_artifact_id"`
		MonitoringTier    string `json:"monitoring_tier"`
	}
	if !decodeStrictRequest(w, request, maximumRuntimeRequestBytes, &input) {
		return
	}
	if !store.ValidIdentifier(input.StartupArtifactID) || !validMonitoringTier(input.MonitoringTier) {
		writeProblem(w, request, http.StatusUnprocessableEntity, "activation_request_invalid", "Activation request invalid", "A startup artifact ID and a supported monitoring tier are required.")
		return
	}
	result, err := handler.services.Fleet.ActivateStartup(request.Context(), requestNodeID(request), input.StartupArtifactID, input.MonitoringTier)
	if err != nil {
		writeNodeProblem(w, request, "runtime_activate_failed", err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (handler *Handler) runtimeLifecycle(w http.ResponseWriter, request *http.Request, operation string) {
	if !handler.requireCommands(w, request) {
		return
	}
	if _, ok := strictCoreQuery(w, request); !ok {
		return
	}
	nodeID := requestNodeID(request)
	var result fleet.RuntimeStatus
	var err error
	switch operation {
	case "start":
		if !requireEmptyCoreBody(w, request) {
			return
		}
		result, err = handler.services.Fleet.StartRuntime(request.Context(), nodeID)
	case "stop":
		if !requireEmptyCoreBody(w, request) {
			return
		}
		result, err = handler.services.Fleet.StopRuntime(request.Context(), nodeID)
	case "restart":
		if !requireEmptyCoreBody(w, request) {
			return
		}
		result, err = handler.services.Fleet.RestartRuntime(request.Context(), nodeID)
	case "rollback":
		var input struct {
			ActivationBundleID string `json:"activation_bundle_id"`
		}
		if !decodeStrictRequest(w, request, maximumRuntimeRequestBytes, &input) {
			return
		}
		if !store.ValidIdentifier(input.ActivationBundleID) {
			writeProblem(w, request, http.StatusUnprocessableEntity, "rollback_bundle_id_invalid", "Rollback bundle ID invalid", "activation_bundle_id must identify the immutable rollback bundle shown during confirmation.")
			return
		}
		result, err = handler.services.Fleet.RollbackRuntime(request.Context(), nodeID, input.ActivationBundleID)
	default:
		err = errors.New("unsupported runtime operation")
	}
	if err != nil {
		writeNodeProblem(w, request, "runtime_"+operation+"_failed", err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// validMonitoringTier accepts a supported tier or none, which selects the
// node's default.
func validMonitoringTier(value string) bool {
	return value == "" || value == "limited" || value == "process_only"
}
