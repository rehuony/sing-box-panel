// SPDX-License-Identifier: GPL-3.0-or-later

package httpapi

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"

	"github.com/rehuony/sing-box-panel/internal/configuration"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/nodeconfig"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/schema"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/store"
)

func (handler *Handler) previewConfiguration(w http.ResponseWriter, request *http.Request) {
	if !handler.requireCommands(w, request) {
		return
	}
	var input nodeconfig.ConfigurationPreviewRequest
	if !decodeStrictRequest(w, request, maximumRuntimeRequestBytes, &input) {
		return
	}
	if !store.ValidIdentifier(input.CoreArtifactID) {
		writeProblem(w, request, http.StatusUnprocessableEntity, "configuration_preview_invalid", "Configuration preview invalid", "A valid core_artifact_id is required.")
		return
	}
	result, err := handler.services.Configs.PreviewConfiguration(request.Context(), requestNodeID(request), input)
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
	var input struct {
		CoreArtifactID string `json:"core_artifact_id"`
	}
	if !decodeStrictRequest(w, request, maximumRuntimeRequestBytes, &input) {
		return
	}
	if !store.ValidIdentifier(input.CoreArtifactID) {
		writeProblem(w, request, http.StatusUnprocessableEntity, "configuration_compile_invalid", "Configuration compile invalid", "A valid core_artifact_id is required.")
		return
	}
	result, err := handler.services.Fleet.CompileConfiguration(request.Context(), requestNodeID(request), input.CoreArtifactID)
	if err != nil {
		writeNodeProblem(w, request, "configuration_compile_failed", err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (handler *Handler) coreConfigurationSupport(w http.ResponseWriter, request *http.Request, identifier string) {
	if !handler.requireCommands(w, request) {
		return
	}
	if _, ok := strictCoreQuery(w, request); !ok {
		return
	}
	result, err := handler.services.Configs.ConfigurationSupport(request.Context(), identifier)
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
	if _, ok := strictCoreQuery(w, request); !ok {
		return
	}
	result, err := handler.services.Configs.ConfigurationSchema(request.Context(), identifier)
	if err != nil {
		writeConfigurationProblem(w, request, "configuration_schema_failed", err)
		return
	}
	etag := configurationSchemaETag(result)
	w.Header().Set("ETag", etag)
	w.Header().Set("Cache-Control", "private, no-cache")
	if noneMatchHits(request.Header.Get("If-None-Match"), etag) {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func configurationSchemaETag(contract nodeconfig.ConfigurationSchema) string {
	digest := sha256.New()
	_, _ = digest.Write([]byte(contract.ExactVersion))
	_, _ = digest.Write([]byte{0})
	_, _ = digest.Write([]byte(contract.SchemaSHA256))
	return quoteETag(hex.EncodeToString(digest.Sum(nil)))
}

func writeConfigurationProblem(w http.ResponseWriter, request *http.Request, code string, err error) {
	configurationProblem(code, err).write(w, request)
}

func configurationProblem(code string, err error) problemDetails {
	switch {
	case errors.Is(err, nodeconfig.ErrConfigurationNotSaved):
		return problemDetails{http.StatusConflict, "configuration_not_saved", "Configuration not saved", "Save a sing-box configuration in Configuration before enabling or checking a core."}
	case errors.Is(err, store.ErrConfigurationFileUnparsed):
		return problemDetails{http.StatusUnprocessableEntity, "configuration_file_unparsed", "Saved configuration invalid", "Correct the saved JSON before checking or running the configuration."}
	case errors.Is(err, store.ErrCachedArtifactNotFound):
		return problemDetails{http.StatusNotFound, "core_artifact_not_found", "Core artifact not found", "The selected immutable core artifact does not exist."}
	case errors.Is(err, schema.ErrConfigurationSchemaUnavailable):
		return problemDetails{http.StatusConflict, "configuration_schema_unavailable", "Configuration schema unavailable", err.Error()}
	case errors.Is(err, configuration.ErrInvalidDocument):
		return problemDetails{http.StatusUnprocessableEntity, "configuration_invalid", "Configuration invalid", err.Error()}
	case errors.Is(err, nodeconfig.ErrConfigurationSchemaValidation):
		return problemDetails{http.StatusUnprocessableEntity, "configuration_schema_validation_failed", "Configuration schema validation failed", err.Error()}
	default:
		return problemDetails{http.StatusInternalServerError, code, "Configuration operation failed", "The configuration operation could not be completed."}
	}
}
