// SPDX-License-Identifier: GPL-3.0-or-later

package httpapi

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/rehuony/sing-box-panel/internal/application"
	"github.com/rehuony/sing-box-panel/internal/configuration"
	"github.com/rehuony/sing-box-panel/internal/store"
)

func (handler *Handler) coreRuntimeStatus(w http.ResponseWriter, request *http.Request) {
	if !handler.requireCommands(w, request) {
		return
	}
	if _, ok := strictCoreQuery(w, request); !ok {
		return
	}
	status, err := handler.commands.RuntimeStatus(request.Context())
	if err != nil {
		writeRuntimeProblem(w, request, "runtime_status_failed", err)
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
		(query.Get("activation_bundle_id") != "" && !validStableIdentifier(query.Get("activation_bundle_id"))) {
		writeProblem(w, request, http.StatusBadRequest, "runtime_history_filter_invalid", "Runtime history filter invalid", "The runtime history filter contains an unsupported value.")
		return
	}
	page, err := handler.commands.RuntimeHistory(request.Context(), application.RuntimeHistoryRequest{
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

func (handler *Handler) queueStartupCheck(w http.ResponseWriter, request *http.Request) {
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
	if !validStableIdentifier(input.StartupArtifactID) {
		writeProblem(w, request, http.StatusUnprocessableEntity, "startup_artifact_id_invalid", "Startup artifact ID invalid", "startup_artifact_id must identify one immutable candidate.")
		return
	}
	task, err := handler.commands.QueueStartupCheck(request.Context(), input.StartupArtifactID)
	if err != nil {
		writeRuntimeProblem(w, request, "startup_check_failed", err)
		return
	}
	writeJSON(w, http.StatusAccepted, task)
}

func (handler *Handler) enableCoreArtifact(w http.ResponseWriter, request *http.Request, id string) {
	if !handler.requireCommands(w, request) {
		return
	}
	if _, ok := strictCoreQuery(w, request); !ok || !requireEmptyCoreBody(w, request) {
		return
	}
	task, err := handler.commands.EnableCore(request.Context(), id)
	if err != nil {
		if errors.Is(err, application.ErrCorePlatformMismatch) || errors.Is(err, application.ErrCoreArtifactVerificationBlocked) {
			writeProblem(w, request, http.StatusConflict, "core_enable_blocked", "Core cannot be enabled", "The artifact must be verified and match the deployed panel operating system and architecture.")
		} else if errors.Is(err, store.ErrCoreArtifactNotFound) {
			writeProblem(w, request, http.StatusNotFound, "core_artifact_not_found", "Core artifact not found", "The requested artifact does not exist.")
		} else {
			writeRuntimeProblem(w, request, "core_enable_failed", err)
		}
		return
	}
	writeJSON(w, http.StatusAccepted, task)
}

func (handler *Handler) queueCoreActivate(w http.ResponseWriter, request *http.Request) {
	if !handler.requireCommands(w, request) {
		return
	}
	if _, ok := strictCoreQuery(w, request); !ok {
		return
	}
	var input struct {
		StartupArtifactID string               `json:"startup_artifact_id"`
		MonitoringTier    store.MonitoringTier `json:"monitoring_tier"`
	}
	if !decodeStrictRequest(w, request, maximumRuntimeRequestBytes, &input) {
		return
	}
	if !validStableIdentifier(input.StartupArtifactID) || !validMonitoringTier(input.MonitoringTier, true) {
		writeProblem(w, request, http.StatusUnprocessableEntity, "activation_request_invalid", "Activation request invalid", "A startup artifact ID and a supported monitoring tier are required.")
		return
	}
	prepared, task, err := handler.commands.PrepareAndQueueRuntimeApply(request.Context(), input.StartupArtifactID, input.MonitoringTier)
	if err != nil {
		writeRuntimeProblem(w, request, "runtime_activate_failed", err)
		return
	}
	writeJSON(w, http.StatusAccepted, struct {
		Activation application.ActivationSummary `json:"activation"`
		Task       application.Task              `json:"task"`
	}{prepared.Summary(), task})
}

func (handler *Handler) queueRuntimeLifecycle(w http.ResponseWriter, request *http.Request, operation string) {
	if !handler.requireCommands(w, request) {
		return
	}
	if _, ok := strictCoreQuery(w, request); !ok {
		return
	}
	var task application.Task
	var err error
	switch operation {
	case "start":
		if !requireEmptyCoreBody(w, request) {
			return
		}
		task, err = handler.commands.QueueRuntimeStart(request.Context())
	case "stop":
		if !requireEmptyCoreBody(w, request) {
			return
		}
		task, err = handler.commands.QueueRuntimeStop(request.Context())
	case "restart":
		if !requireEmptyCoreBody(w, request) {
			return
		}
		task, err = handler.commands.QueueRuntimeRestart(request.Context())
	case "rollback":
		var input struct {
			ActivationBundleID string `json:"activation_bundle_id"`
		}
		if !decodeStrictRequest(w, request, maximumRuntimeRequestBytes, &input) {
			return
		}
		if !validStableIdentifier(input.ActivationBundleID) {
			writeProblem(w, request, http.StatusUnprocessableEntity, "rollback_bundle_id_invalid", "Rollback bundle ID invalid", "activation_bundle_id must identify the immutable rollback bundle shown during confirmation.")
			return
		}
		task, err = handler.commands.QueueRuntimeRollback(request.Context(), input.ActivationBundleID)
	default:
		err = errors.New("unsupported runtime operation")
	}
	if err != nil {
		writeRuntimeProblem(w, request, "runtime_"+operation+"_failed", err)
		return
	}
	writeJSON(w, http.StatusAccepted, task)
}

func validStableIdentifier(value string) bool {
	if value == "" || len(value) > 256 || strings.TrimSpace(value) != value {
		return false
	}
	for _, character := range value {
		if unicode.IsControl(character) {
			return false
		}
	}
	return true
}

func validMonitoringTier(value store.MonitoringTier, allowDefault bool) bool {
	if value == "" {
		return allowDefault
	}
	return value == store.MonitoringLimited || value == store.MonitoringProcessOnly
}

func writeRuntimeProblem(w http.ResponseWriter, request *http.Request, code string, err error) {
	switch {
	case errors.Is(err, store.ErrConfigurationFileUnparsed), errors.Is(err, configuration.ErrInvalidDocument), errors.Is(err, application.ErrConfigurationSchemaValidation), errors.Is(err, store.ErrCompiledStartupEvidenceStale):
		writeConfigurationProblem(w, request, code, err)
	case application.IsStartupArtifactNotFound(err):
		writeProblem(w, request, http.StatusNotFound, "startup_artifact_not_found", "Startup artifact not found", "The requested startup artifact does not exist.")
	case errors.Is(err, store.ErrStartupArtifactState):
		writeProblem(w, request, http.StatusConflict, "startup_artifact_state_invalid", "Startup artifact state invalid", err.Error())
	case application.IsActivationBundleNotReady(err):
		writeProblem(w, request, http.StatusConflict, "activation_bundle_not_ready", "Activation bundle not ready", "The immutable startup artifact or core binding is not ready.")
	case application.IsMonitoringTierUnavailable(err):
		writeProblem(w, request, http.StatusConflict, "monitoring_tier_unavailable", "Monitoring tier unavailable", err.Error())
	case application.IsNoAppliedBundle(err):
		writeProblem(w, request, http.StatusConflict, "no_applied_bundle", "No applied bundle", "No successfully applied bundle is available for this operation.")
	case application.IsNoRollbackBundle(err):
		writeProblem(w, request, http.StatusConflict, "no_rollback_bundle", "No rollback bundle", "No rollback bundle is available.")
	case errors.Is(err, store.ErrRuntimeIntentStale):
		writeProblem(w, request, http.StatusPreconditionFailed, "rollback_bundle_changed", "Rollback bundle changed", "The rollback evidence changed after confirmation; reload the current runtime status before retrying.")
	case errors.Is(err, application.ErrStaleObservation), errors.Is(err, application.ErrInspectionUnavailable):
		writeProblem(w, request, http.StatusServiceUnavailable, "runtime_inspection_unavailable", "Runtime inspection unavailable", "The live core identity could not be verified.")
	default:
		writeProblem(w, request, http.StatusInternalServerError, code, "Runtime operation failed", "The runtime operation could not be completed.")
	}
}
