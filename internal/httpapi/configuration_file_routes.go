// SPDX-License-Identifier: GPL-3.0-or-later

package httpapi

import (
	"errors"
	"net/http"

	"github.com/rehuony/sing-box-panel/internal/application"
	"github.com/rehuony/sing-box-panel/internal/configuration"
	"github.com/rehuony/sing-box-panel/internal/store"
)

func (handler *Handler) newInboundDefaults(w http.ResponseWriter, request *http.Request) {
	if !handler.requireCommands(w, request) {
		return
	}
	if _, ok := strictCoreQuery(w, request); !ok {
		return
	}
	var input struct {
		Type string `json:"type"`
	}
	if !decodeStrictRequest(w, request, 1024, &input) {
		return
	}
	result, err := handler.commands.NewInboundDefaults(request.Context(), input.Type)
	if err != nil {
		if errors.Is(err, application.ErrPanelSettingsInvalid) {
			writeProblem(w, request, 422, "inbound_defaults_invalid", "Invalid inbound type", "A protocol type is required.")
			return
		}
		writeConfigurationFileProblem(w, request, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, result)
}

func (handler *Handler) configurationFile(w http.ResponseWriter, request *http.Request) {
	if !handler.requireCommands(w, request) {
		return
	}
	if _, ok := strictCoreQuery(w, request); !ok {
		return
	}
	result, err := handler.commands.ConfigurationFile(request.Context())
	if err != nil {
		writeConfigurationFileProblem(w, request, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, result)
}

func (handler *Handler) saveConfigurationFile(w http.ResponseWriter, request *http.Request) {
	if !handler.requireCommands(w, request) {
		return
	}
	if _, ok := strictCoreQuery(w, request); !ok {
		return
	}
	var input application.ConfigurationFileWrite
	// JSON escaping can expand one input byte into six wire bytes.
	if !decodeStrictRequest(w, request, configuration.MaximumBytes*6+1024, &input) {
		return
	}
	result, err := handler.commands.SaveConfigurationFile(request.Context(), input)
	if err != nil {
		writeConfigurationFileProblem(w, request, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, result)
}

func writeConfigurationFileProblem(w http.ResponseWriter, request *http.Request, err error) {
	switch {
	case errors.Is(err, store.ErrConfigurationFileConflict):
		writeProblem(w, request, http.StatusPreconditionFailed, "configuration_file_conflict", "Configuration changed", "Reload the saved configuration before saving.")
	case errors.Is(err, store.ErrConfigurationFileInvalid):
		writeProblem(w, request, http.StatusUnprocessableEntity, "configuration_file_invalid", "Invalid configuration text", "Configuration must be UTF-8 text without NUL bytes and at most 2 MiB.")
	default:
		writeConfigurationProblem(w, request, "configuration_file_failed", err)
	}
}
