// SPDX-License-Identifier: GPL-3.0-or-later

package httpapi

import (
	"errors"
	"net/http"
	"strings"
	"unicode"

	"github.com/rehuony/sing-box-panel/internal/application"
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
	if _, ok := strictCoreQuery(w, request); !ok || !requireEmptyCoreBody(w, request) {
		return
	}
	var task application.Task
	var err error
	switch operation {
	case "start":
		task, err = handler.commands.QueueRuntimeStart(request.Context())
	case "stop":
		task, err = handler.commands.QueueRuntimeStop(request.Context())
	case "restart":
		task, err = handler.commands.QueueRuntimeRestart(request.Context())
	case "rollback":
		task, err = handler.commands.QueueRuntimeRollback(request.Context())
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
	return value == store.MonitoringFull || value == store.MonitoringLimited || value == store.MonitoringProcessOnly
}

func writeRuntimeProblem(w http.ResponseWriter, request *http.Request, code string, err error) {
	switch {
	case application.IsStartupArtifactNotFound(err):
		writeProblem(w, request, http.StatusNotFound, "startup_artifact_not_found", "Startup artifact not found", "The requested startup artifact does not exist.")
	case errors.Is(err, store.ErrStartupArtifactState):
		writeProblem(w, request, http.StatusConflict, "startup_artifact_state_invalid", "Startup artifact state invalid", err.Error())
	case application.IsActivationBundleNotReady(err):
		writeProblem(w, request, http.StatusConflict, "activation_bundle_not_ready", "Activation bundle not ready", "The immutable startup artifact, adapter, or core binding is not ready.")
	case application.IsMonitoringTierUnavailable(err):
		writeProblem(w, request, http.StatusConflict, "monitoring_tier_unavailable", "Monitoring tier unavailable", err.Error())
	case application.IsNoAppliedBundle(err):
		writeProblem(w, request, http.StatusConflict, "no_applied_bundle", "No applied bundle", "No successfully applied bundle is available for this operation.")
	case application.IsNoRollbackBundle(err):
		writeProblem(w, request, http.StatusConflict, "no_rollback_bundle", "No rollback bundle", "No rollback bundle is available.")
	case errors.Is(err, application.ErrStaleObservation), errors.Is(err, application.ErrInspectionUnavailable):
		writeProblem(w, request, http.StatusServiceUnavailable, "runtime_inspection_unavailable", "Runtime inspection unavailable", "The live core identity could not be verified.")
	default:
		writeProblem(w, request, http.StatusInternalServerError, code, "Runtime operation failed", "The runtime operation could not be completed.")
	}
}
