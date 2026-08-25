// SPDX-License-Identifier: GPL-3.0-or-later

package httpapi

import (
	"errors"
	"net/http"

	"github.com/rehuony/sing-box-panel/internal/application"
	"github.com/rehuony/sing-box-panel/internal/reconcile"
	"github.com/rehuony/sing-box-panel/internal/runtimeidentity"
	"github.com/rehuony/sing-box-panel/internal/store"
)

const maximumManualReattachRequestBytes = 1 << 20

func (handler *Handler) renderStructuredConfiguration(w http.ResponseWriter, request *http.Request) {
	if !handler.requireCommands(w, request) {
		return
	}
	if _, ok := strictCoreQuery(w, request); !ok {
		return
	}
	var input struct {
		CoreVersion     string `json:"core_version"`
		CoreArtifactID  string `json:"core_artifact_id"`
		AllowCompatible bool   `json:"allow_compatible"`
	}
	if !decodeStrictRequest(w, request, maximumRuntimeRequestBytes, &input) {
		return
	}
	if input.CoreVersion != "" && !validOptionalExactVersion(input.CoreVersion, true) {
		writeProblem(w, request, http.StatusUnprocessableEntity, "core_version_invalid", "Core version invalid", "core_version must be a non-zero exact stable semantic version when supplied.")
		return
	}
	if input.CoreArtifactID != "" && !validStableIdentifier(input.CoreArtifactID) {
		writeProblem(w, request, http.StatusUnprocessableEntity, "core_artifact_id_invalid", "Core artifact invalid", "core_artifact_id is invalid.")
		return
	}
	result, err := handler.commands.RenderStructured(request.Context(), application.StructuredRenderRequest{
		CoreVersion: input.CoreVersion, CoreArtifactID: input.CoreArtifactID,
		AllowCompatible: input.AllowCompatible,
	})
	if err != nil {
		writeStructuredRenderProblem(w, request, err)
		return
	}
	writeJSON(w, http.StatusAccepted, result)
}

func (handler *Handler) previewManualReattach(w http.ResponseWriter, request *http.Request, identifier string) {
	if !handler.requireCommands(w, request) {
		return
	}
	if !validStableIdentifier(identifier) {
		writeProblem(w, request, http.StatusBadRequest, "startup_artifact_id_invalid", "Startup artifact ID invalid", "The startup artifact ID is invalid.")
		return
	}
	if _, ok := strictCoreQuery(w, request); !ok || !requireEmptyCoreBody(w, request) {
		return
	}
	preview, err := handler.commands.PreviewManualReattach(request.Context(), identifier)
	if err != nil {
		writeManualReattachProblem(w, request, "manual_reattach_preview_failed", err)
		return
	}
	writeJSON(w, http.StatusOK, preview)
}

func (handler *Handler) applyManualReattach(w http.ResponseWriter, request *http.Request, identifier string) {
	if !handler.requireCommands(w, request) {
		return
	}
	if !validStableIdentifier(identifier) {
		writeProblem(w, request, http.StatusBadRequest, "startup_artifact_id_invalid", "Startup artifact ID invalid", "The startup artifact ID is invalid.")
		return
	}
	if _, ok := strictCoreQuery(w, request); !ok {
		return
	}
	var input application.ManualReattachApplyRequest
	if !decodeStrictRequest(w, request, maximumManualReattachRequestBytes, &input) {
		return
	}
	input.StartupArtifactID = identifier
	result, err := handler.commands.ApplyManualReattach(request.Context(), input)
	if err != nil {
		writeManualReattachProblem(w, request, "manual_reattach_apply_failed", err)
		return
	}
	w.Header().Set("ETag", quoteETag(result.Revision.ID))
	writeJSON(w, http.StatusAccepted, result)
}

func writeStructuredRenderProblem(w http.ResponseWriter, request *http.Request, err error) {
	switch {
	case application.IsNoRunningCore(err):
		writeProblem(w, request, http.StatusConflict, "core_not_running", "Core is not running", "core_version was omitted and no verified core is currently running.")
	case errors.Is(err, runtimeidentity.ErrStaleObservation), errors.Is(err, runtimeidentity.ErrInspectionUnavailable):
		writeProblem(w, request, http.StatusServiceUnavailable, "runtime_inspection_unavailable", "Runtime inspection unavailable", "core_version was omitted and the live core identity could not be verified.")
	case application.IsCoreArtifactNotFound(err):
		writeProblem(w, request, http.StatusNotFound, "core_artifact_not_found", "Core artifact not found", "The selected immutable core artifact does not exist.")
	case application.IsCompatibleCapabilityNotAccepted(err):
		writeProblem(w, request, http.StatusConflict, "compatible_capability_not_accepted", "Compatible capability not accepted", "compatible_structured support requires an explicit allow_compatible decision.")
	case application.IsStructuredCapabilityUnavailable(err):
		writeProblem(w, request, http.StatusConflict, "structured_capability_unavailable", "Structured capability unavailable", "The exact version has no usable pinned structured capability; use manual JSON.")
	case application.IsUnsupportedActiveFact(err):
		writeProblem(w, request, http.StatusConflict, "unsupported_active_fact", "Active configuration is unsupported", err.Error())
	case errors.Is(err, store.ErrStartupArtifactExists):
		writeProblem(w, request, http.StatusConflict, "startup_artifact_conflict", "Startup artifact conflict", "The immutable startup candidate could not be created because its identity already exists.")
	default:
		writeProblem(w, request, http.StatusInternalServerError, "structured_render_failed", "Structured render failed", "The exact-version configuration could not be rendered and queued for checking.")
	}
}

func writeManualReattachProblem(w http.ResponseWriter, request *http.Request, code string, err error) {
	switch {
	case application.IsStartupArtifactNotFound(err):
		writeProblem(w, request, http.StatusNotFound, "startup_artifact_not_found", "Startup artifact not found", "The requested manual startup artifact does not exist.")
	case errors.Is(err, application.ErrManualReattachUnavailable):
		writeProblem(w, request, http.StatusConflict, "manual_reattach_unavailable", "Manual reattach unavailable", "The exact pinned structured capability or immutable source evidence is unavailable.")
	case errors.Is(err, application.ErrManualReattachPreviewStale), application.IsRevisionConflict(err):
		writeProblem(w, request, http.StatusPreconditionFailed, "manual_reattach_preview_stale", "Manual reattach preview stale", "The canonical head or immutable evidence changed; request a new preview.")
	case errors.Is(err, reconcile.ErrUnresolvedConflict):
		writeProblem(w, request, http.StatusUnprocessableEntity, "manual_reattach_decisions_invalid", "Manual reattach decisions invalid", err.Error())
	case application.IsInvalidManualJSON(err):
		writeProblem(w, request, http.StatusUnprocessableEntity, "manual_json_invalid", "Manual configuration invalid", err.Error())
	default:
		writeProblem(w, request, http.StatusInternalServerError, code, "Manual reattach failed", "The manual candidate could not be reconciled.")
	}
}
