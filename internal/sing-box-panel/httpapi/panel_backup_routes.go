// SPDX-License-Identifier: GPL-3.0-or-later
package httpapi

import (
	"errors"
	"net/http"

	"github.com/rehuony/sing-box-panel/internal/configuration"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/settings"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/store"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/telemetry"
)

func (handler *Handler) exportPanelBackup(w http.ResponseWriter, r *http.Request) {
	if !handler.requireCommands(w, r) {
		return
	}
	if _, ok := strictCoreQuery(w, r); !ok {
		return
	}
	backup, err := handler.services.Settings.ExportPanelBackup(r.Context())
	if err != nil {
		writePanelSettingsProblem(w, r, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Disposition", `attachment; filename="sing-box-panel-backup.json"`)
	writeJSON(w, http.StatusOK, backup)
}

func (handler *Handler) restorePanelBackup(w http.ResponseWriter, r *http.Request) {
	if !handler.requireCommands(w, r) {
		return
	}
	if _, ok := strictCoreQuery(w, r); !ok {
		return
	}
	var input settings.PanelRestoreRequest
	if !decodeStrictRequest(w, r, configuration.MaximumBytes*6+settings.MaximumBytes+4096, &input) {
		return
	}
	result, err := handler.services.Settings.RestorePanelBackup(r.Context(), input)
	handler.services.Telemetry.RecordOperation(r.Context(), "panel.backup.restore", "Panel configuration restore", err, telemetry.OperationLogContext{})
	switch {
	case errors.Is(err, settings.ErrPanelBackupInvalid), errors.Is(err, store.ErrConfigurationFileInvalid):
		writeProblem(w, r, http.StatusUnprocessableEntity, "panel_backup_invalid", "Invalid backup", "Select a supported panel backup with valid settings and configuration text.")
	case errors.Is(err, store.ErrConfigurationFileConflict):
		writeProblem(w, r, http.StatusPreconditionFailed, "configuration_file_conflict", "Configuration changed", "Review the current configuration before restoring the backup.")
	case err != nil:
		writePanelSettingsProblem(w, r, err)
	default:
		if result.ReauthenticationRequired {
			handler.clearSessionCookie(w, r)
		}
		w.Header().Set("Cache-Control", "no-store")
		writeJSON(w, http.StatusOK, result)
	}
}
