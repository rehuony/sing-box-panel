// SPDX-License-Identifier: GPL-3.0-or-later

package httpapi

import (
	"errors"
	"net/http"

	"github.com/rehuony/sing-box-panel/internal/application"
	"github.com/rehuony/sing-box-panel/internal/store"
)

func (handler *Handler) getPanelSettings(w http.ResponseWriter, request *http.Request) {
	if !handler.requireCommands(w, request) {
		return
	}
	if _, ok := strictCoreQuery(w, request); !ok {
		return
	}
	value, err := handler.commands.PanelSettings(request.Context())
	if err != nil {
		writePanelSettingsProblem(w, request, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, value)
}

func (handler *Handler) savePanelSettings(w http.ResponseWriter, request *http.Request) {
	if !handler.requireCommands(w, request) {
		return
	}
	if _, ok := strictCoreQuery(w, request); !ok {
		return
	}
	var input application.PanelSettingsWrite
	if !decodeStrictRequest(w, request, 64<<10, &input) {
		return
	}
	value, err := handler.commands.SavePanelSettings(request.Context(), input)
	if err != nil {
		writePanelSettingsProblem(w, request, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, value)
}

func writePanelSettingsProblem(w http.ResponseWriter, request *http.Request, err error) {
	switch {
	case errors.Is(err, application.ErrIdentityConfiguration):
		writeProblem(w, request, http.StatusUnprocessableEntity, "identity_configuration_invalid", "Configuration needs attention", "Correct the saved configuration before changing the protocol identity.")
	case errors.Is(err, store.ErrConfigurationFileConflict):
		writeProblem(w, request, http.StatusPreconditionFailed, "identity_configuration_conflict", "Configuration changed", "The configuration changed while saving the identity. Review the configuration and retry.")
	case errors.Is(err, application.ErrPanelSettingsInvalid):
		writeProblem(w, request, http.StatusUnprocessableEntity, "panel_settings_invalid", "Invalid settings", "Check the settings values and try again.")
	case errors.Is(err, store.ErrPanelSettingsConflict):
		writeProblem(w, request, http.StatusPreconditionFailed, "panel_settings_conflict", "Settings changed", "Reload the current settings before saving.")
	default:
		writeProblem(w, request, http.StatusInternalServerError, "panel_settings_failed", "Settings unavailable", "The settings operation could not be completed.")
	}
}

func (handler *Handler) currentManagementToken(w http.ResponseWriter, request *http.Request) (string, bool) {
	if handler.commands == nil {
		return handler.settings.Auth.Token, true
	}
	value, err := handler.commands.EffectiveSettings(request.Context())
	if err != nil {
		writeProblem(w, request, http.StatusServiceUnavailable, "authentication_unavailable", "Authentication unavailable", "Authentication settings could not be read.")
		return "", false
	}
	if value.Auth.Token == "" {
		return handler.settings.Auth.Token, true
	}
	return value.Auth.Token, true
}
