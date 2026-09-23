// SPDX-License-Identifier: GPL-3.0-or-later
package httpapi

import (
	"errors"
	"net/http"

	"github.com/rehuony/sing-box-panel/internal/application"
	"github.com/rehuony/sing-box-panel/internal/configuration"
	"github.com/rehuony/sing-box-panel/internal/settings"
	"github.com/rehuony/sing-box-panel/internal/store"
)

func (handler *Handler) exportPanelBackup(w http.ResponseWriter, r *http.Request) {
	if !handler.requireCommands(w, r) {
		return
	}
	if _, ok := strictCoreQuery(w, r); !ok {
		return
	}
	backup, err := handler.commands.ExportPanelBackup(r.Context())
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
	var input application.PanelRestoreRequest
	if !decodeStrictRequest(w, r, configuration.MaximumBytes*6+settings.MaximumBytes+4096, &input) {
		return
	}
	result, err := handler.commands.RestorePanelBackup(r.Context(), input)
	switch {
	case errors.Is(err, application.ErrPanelBackupInvalid), errors.Is(err, store.ErrConfigurationFileInvalid):
		writeProblem(w, r, http.StatusUnprocessableEntity, "panel_backup_invalid", "Invalid backup", "Select a supported panel backup with valid settings and configuration text.")
	case errors.Is(err, store.ErrConfigurationFileConflict):
		writeProblem(w, r, http.StatusPreconditionFailed, "configuration_file_conflict", "Configuration changed", "Review the current configuration before restoring the backup.")
	case err != nil:
		writePanelSettingsProblem(w, r, err)
	default:
		w.Header().Set("Cache-Control", "no-store")
		writeJSON(w, http.StatusOK, result)
	}
}
