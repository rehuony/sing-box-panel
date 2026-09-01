// SPDX-License-Identifier: GPL-3.0-or-later

package httpapi

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"

	"github.com/rehuony/sing-box-panel/internal/application"
	"github.com/rehuony/sing-box-panel/internal/configuration"
	"github.com/rehuony/sing-box-panel/internal/singbox"
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

func (handler *Handler) coreConfigurationSchema(w http.ResponseWriter, request *http.Request, identifier string) {
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
	result, err := handler.commands.ConfigurationSchema(request.Context(), identifier)
	if err != nil {
		writeConfigurationProblem(w, request, "configuration_schema_failed", err)
		return
	}
	etag := configurationSchemaETag(result)
	w.Header().Set("ETag", etag)
	w.Header().Set("Cache-Control", "private, no-cache")
	if publicSubscriptionETagMatches(request.Header.Get("If-None-Match"), etag) {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func configurationSchemaETag(contract application.ConfigurationSchema) string {
	digest := sha256.New()
	_, _ = digest.Write([]byte(contract.ExactVersion))
	_, _ = digest.Write([]byte{0})
	_, _ = digest.Write([]byte(contract.SchemaSHA256))
	return quoteETag(hex.EncodeToString(digest.Sum(nil)))
}

func writeConfigurationProblem(w http.ResponseWriter, request *http.Request, code string, err error) {
	switch {
	case application.IsCoreArtifactNotFound(err):
		writeProblem(w, request, http.StatusNotFound, "core_artifact_not_found", "Core artifact not found", "The selected immutable core artifact does not exist.")
	case errors.Is(err, singbox.ErrConfigurationSchemaUnavailable):
		writeProblem(w, request, http.StatusConflict, "configuration_schema_unavailable", "Configuration schema unavailable", err.Error())
	case errors.Is(err, configuration.ErrInvalidDocument):
		writeProblem(w, request, http.StatusUnprocessableEntity, "configuration_invalid", "Configuration invalid", err.Error())
	case errors.Is(err, application.ErrConfigurationSchemaValidation):
		writeProblem(w, request, http.StatusUnprocessableEntity, "configuration_schema_validation_failed", "Configuration schema validation failed", err.Error())
	case errors.Is(err, store.ErrCompiledStartupEvidenceStale):
		writeProblem(w, request, http.StatusPreconditionFailed, "configuration_changed", "Configuration changed", "The global configuration changed during compilation; retry with the current revision.")
	default:
		writeProblem(w, request, http.StatusInternalServerError, code, "Configuration operation failed", "The configuration operation could not be completed.")
	}
}
