// SPDX-License-Identifier: GPL-3.0-or-later

package httpapi

import (
	"errors"
	"net/http"

	"github.com/rehuony/sing-box-panel/internal/configuration"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/nodeconfig"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/store"
)

func (handler *Handler) configurationFile(w http.ResponseWriter, request *http.Request) {
	if _, ok := strictCoreQuery(w, request); !ok {
		return
	}
	result, err := handler.services.Configs.ConfigurationFile(request.Context(), requestNodeID(request))
	if err != nil {
		writeConfigurationFileProblem(w, request, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, result)
}

func (handler *Handler) saveConfigurationFile(w http.ResponseWriter, request *http.Request) {
	if _, ok := strictCoreQuery(w, request); !ok {
		return
	}
	var input nodeconfig.ConfigurationFileWrite
	// JSON escaping can expand one input byte into six wire bytes.
	if !decodeStrictRequest(w, request, configuration.MaximumBytes*6+1024, &input) {
		return
	}
	result, err := handler.services.Configs.SaveConfigurationFile(request.Context(), requestNodeID(request), input)
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
