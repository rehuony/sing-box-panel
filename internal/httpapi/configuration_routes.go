// SPDX-License-Identifier: GPL-3.0-or-later

package httpapi

import (
	"errors"
	"net/http"

	"github.com/rehuony/sing-box-panel/internal/application"
	"github.com/rehuony/sing-box-panel/internal/configuration/adapter"
	"github.com/rehuony/sing-box-panel/internal/store"
)

func (handler *Handler) previewConfiguration(w http.ResponseWriter, request *http.Request) {
	if !handler.requireCommands(w, request) {
		return
	}
	var input application.ConfigurationPreviewRequest
	if !decodeStrictRequest(w, request, maximumRuntimeRequestBytes, &input) {
		return
	}
	if !validCoreArtifactID(input.CoreArtifactID) || (input.CanonicalRevisionID != "" && !validStableIdentifier(input.CanonicalRevisionID)) {
		writeProblem(w, request, http.StatusUnprocessableEntity, "configuration_preview_invalid", "Configuration preview invalid", "A valid core_artifact_id and optional canonical_revision_id are required.")
		return
	}
	result, err := handler.commands.PreviewConfiguration(request.Context(), input)
	if err != nil {
		writeConfigurationProblem(w, request, "configuration_preview_failed", err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (handler *Handler) compileConfiguration(w http.ResponseWriter, request *http.Request) {
	if !handler.requireCommands(w, request) {
		return
	}
	var input application.ConfigurationCompileRequest
	if !decodeStrictRequest(w, request, maximumRuntimeRequestBytes, &input) {
		return
	}
	if !validCoreArtifactID(input.CoreArtifactID) {
		writeProblem(w, request, http.StatusUnprocessableEntity, "configuration_compile_invalid", "Configuration compile invalid", "A valid core_artifact_id is required.")
		return
	}
	result, err := handler.commands.CompileConfiguration(request.Context(), input)
	if err != nil {
		writeConfigurationProblem(w, request, "configuration_compile_failed", err)
		return
	}
	writeJSON(w, http.StatusAccepted, result)
}

func (handler *Handler) coreConfigurationSupport(w http.ResponseWriter, request *http.Request, identifier string) {
	if !handler.requireCommands(w, request) {
		return
	}
	if !validCoreArtifactID(identifier) {
		writeProblem(w, request, http.StatusBadRequest, "core_artifact_id_invalid", "Core artifact ID invalid", "The core artifact ID is invalid.")
		return
	}
	if _, ok := strictCoreQuery(w, request); !ok {
		return
	}
	result, err := handler.commands.ConfigurationSupport(request.Context(), identifier)
	if err != nil {
		writeConfigurationProblem(w, request, "configuration_support_failed", err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func writeConfigurationProblem(w http.ResponseWriter, request *http.Request, code string, err error) {
	switch {
	case application.IsCoreArtifactNotFound(err):
		writeProblem(w, request, http.StatusNotFound, "core_artifact_not_found", "Core artifact not found", "The selected immutable core artifact does not exist.")
	case errors.Is(err, adapter.ErrUnsupportedCoreProfile):
		writeProblem(w, request, http.StatusConflict, "core_profile_unsupported", "Core profile unsupported", err.Error())
	case errors.Is(err, adapter.ErrIgnoredNotAccepted):
		writeProblem(w, request, http.StatusConflict, "ignored_fields_not_accepted", "Ignored fields not accepted", err.Error())
	case errors.Is(err, adapter.ErrProjection), errors.Is(err, adapter.ErrProjectionBlocked):
		writeProblem(w, request, http.StatusUnprocessableEntity, "configuration_projection_failed", "Configuration projection failed", err.Error())
	case errors.Is(err, store.ErrCompiledStartupEvidenceStale):
		writeProblem(w, request, http.StatusPreconditionFailed, "configuration_changed", "Configuration changed", "The global configuration changed during compilation; retry with the current revision.")
	default:
		writeProblem(w, request, http.StatusInternalServerError, code, "Configuration operation failed", "The configuration operation could not be completed.")
	}
}
