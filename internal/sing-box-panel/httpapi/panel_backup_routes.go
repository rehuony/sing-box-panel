// SPDX-License-Identifier: GPL-3.0-or-later
package httpapi

import (
	"errors"
	"io"
	"mime"
	"net/http"
	"time"

	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/backup"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/telemetry"
)

const (
	// backupUploadReadTimeout bounds reading one uploaded backup, enough for
	// a large panel database over a slow link.
	backupUploadReadTimeout = time.Hour
	// maximumBackupUploadBytes bounds an uploaded backup.
	maximumBackupUploadBytes = 1 << 36
	minimumPassphraseBytes   = 8
	maximumPassphraseBytes   = 1024
)

// PanelBackupList is the backups in the panel's data directory, newest first.
type PanelBackupList struct {
	Items []backup.Info `json:"items"`
}

// PanelBackupCreate asks for one backup.
type PanelBackupCreate struct {
	// Categories are what the backup holds.
	Categories []backup.Category `json:"categories"`
	// Passphrase encrypts the backup when it is not empty.
	Passphrase string `json:"passphrase,omitempty"`
}

// PanelRestoreStage asks to stage a stored backup.
type PanelRestoreStage struct {
	Passphrase string `json:"passphrase,omitempty"`
}

// PanelRestorePlanRequest names the categories of a staged restore to plan.
type PanelRestorePlanRequest struct {
	Categories []backup.Category `json:"categories"`
}

// PanelRestoreApply asks to apply categories of a staged restore.
type PanelRestoreApply struct {
	// TakeService also takes the backup's listener, base path and origin
	// with its settings.
	TakeService bool              `json:"take_service"`
	Categories  []backup.Category `json:"categories"`
}

// PanelRestoreRequested is a restore the panel applies when it next starts.
type PanelRestoreRequested struct {
	// StartedAt is when the panel that answers started; a restarted panel
	// reports another time.
	StartedAt time.Time `json:"started_at"`
	// Before is the backup of the state the restore replaces.
	Before backup.Info `json:"before"`
	// Restarting reports whether the panel restarts now to apply it; else
	// it applies when the panel is next started.
	Restarting bool `json:"restarting"`
}

func (handler *Handler) listPanelBackups(w http.ResponseWriter, r *http.Request) {
	if _, ok := strictCoreQuery(w, r); !ok {
		return
	}
	items, err := handler.services.Backups.List(r.Context())
	if err != nil {
		writeBackupProblem(w, r, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, PanelBackupList{Items: items})
}

func (handler *Handler) createPanelBackup(w http.ResponseWriter, r *http.Request) {
	if _, ok := strictCoreQuery(w, r); !ok {
		return
	}
	var input PanelBackupCreate
	if !decodeStrictRequest(w, r, 16<<10, &input) || !validPassphrase(w, r, input.Passphrase, true) {
		return
	}
	startedAt := time.Now()
	info, err := handler.services.Backups.Create(r.Context(), backup.CreateOptions{
		Kind: backup.KindManual, Categories: input.Categories, Passphrase: input.Passphrase,
	})
	handler.services.Telemetry.RecordOperation(r.Context(), "panel.backup.create", "Panel backup", err,
		telemetry.OperationLogContext{StartedAt: startedAt, File: info.Name})
	if err != nil {
		writeBackupProblem(w, r, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusCreated, info)
}

func (handler *Handler) downloadPanelBackup(w http.ResponseWriter, r *http.Request, name string) {
	if _, ok := strictCoreQuery(w, r); !ok {
		return
	}
	file, info, err := handler.services.Backups.Open(name)
	if err != nil {
		writeBackupProblem(w, r, err)
		return
	}
	defer file.Close()
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": info.Name}))
	http.ServeContent(w, r, "", info.CreatedAt, file)
}

func (handler *Handler) deletePanelBackup(w http.ResponseWriter, r *http.Request, name string) {
	if _, ok := strictCoreQuery(w, r); !ok || !requireEmptyCoreBody(w, r) {
		return
	}
	err := handler.services.Backups.Delete(name)
	handler.services.Telemetry.RecordOperation(r.Context(), "panel.backup.delete", "Panel backup removal", err,
		telemetry.OperationLogContext{File: name})
	if err != nil {
		writeBackupProblem(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (handler *Handler) stagePanelBackup(w http.ResponseWriter, r *http.Request, name string) {
	if _, ok := strictCoreQuery(w, r); !ok {
		return
	}
	var input PanelRestoreStage
	if !decodeStrictRequest(w, r, 16<<10, &input) || !validPassphrase(w, r, input.Passphrase, false) {
		return
	}
	staged, err := handler.services.Backups.StageStored(r.Context(), name, input.Passphrase)
	handler.services.Telemetry.RecordOperation(r.Context(), "panel.restore.stage", "Restore check", err,
		telemetry.OperationLogContext{File: name})
	if err != nil {
		writeBackupProblem(w, r, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, staged)
}

// uploadPanelRestore stages a restore from an uploaded archive: a
// multipart form of the archive file and, for an encrypted one, its
// passphrase before it.
func (handler *Handler) uploadPanelRestore(w http.ResponseWriter, r *http.Request) {
	if _, ok := strictCoreQuery(w, r); !ok {
		return
	}
	if err := http.NewResponseController(w).SetReadDeadline(time.Now().Add(backupUploadReadTimeout)); err != nil && !errors.Is(err, http.ErrNotSupported) {
		writeProblem(w, r, http.StatusInternalServerError, "panel_restore_unavailable", "Restore unavailable", "The upload could not be prepared.")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maximumBackupUploadBytes+(64<<10))
	reader, err := r.MultipartReader()
	if err != nil {
		writeProblem(w, r, http.StatusUnsupportedMediaType, "panel_restore_media_type", "Restore media type invalid", "Use multipart/form-data with the backup file.")
		return
	}
	passphrase := ""
	for {
		part, err := reader.NextPart()
		if err != nil {
			writeProblem(w, r, http.StatusUnprocessableEntity, "panel_restore_invalid", "Restore request invalid", "Send the passphrase, if any, and then one backup file.")
			return
		}
		switch part.FormName() {
		case "passphrase":
			value, readErr := io.ReadAll(io.LimitReader(part, maximumPassphraseBytes+1))
			_ = part.Close()
			if readErr != nil {
				writeProblem(w, r, http.StatusUnprocessableEntity, "panel_restore_invalid", "Restore request invalid", "The passphrase could not be read.")
				return
			}
			if passphrase = string(value); !validPassphrase(w, r, passphrase, false) {
				return
			}
			continue
		case "archive":
		default:
			_ = part.Close()
			writeProblem(w, r, http.StatusUnprocessableEntity, "panel_restore_invalid", "Restore request invalid", "The upload contains an unknown field.")
			return
		}
		staged, err := handler.services.Backups.Stage(r.Context(), part, passphrase)
		_ = part.Close()
		handler.services.Telemetry.RecordOperation(r.Context(), "panel.restore.stage", "Restore check", err, telemetry.OperationLogContext{})
		if err != nil {
			writeBackupProblem(w, r, err)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		writeJSON(w, http.StatusOK, staged)
		return
	}
}

// planPanelRestore counts what restoring categories of a staged restore
// changes in the categories it keeps, and whether it signs everyone out.
func (handler *Handler) planPanelRestore(w http.ResponseWriter, r *http.Request, id string) {
	if _, ok := strictCoreQuery(w, r); !ok {
		return
	}
	var input PanelRestorePlanRequest
	if !decodeStrictRequest(w, r, 16<<10, &input) {
		return
	}
	plan, err := handler.services.Backups.Plan(r.Context(), id, input.Categories)
	if err != nil {
		writeBackupProblem(w, r, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, plan)
}

func (handler *Handler) discardPanelRestore(w http.ResponseWriter, r *http.Request, id string) {
	if _, ok := strictCoreQuery(w, r); !ok || !requireEmptyCoreBody(w, r) {
		return
	}
	if err := handler.services.Backups.Discard(id); err != nil {
		writeBackupProblem(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// applyPanelRestore backs the panel up as it is, marks categories of the
// staged restore to be applied before the panel next opens its databases, and
// restarts it where it can restart itself. Restoring the settings ends every
// session.
func (handler *Handler) applyPanelRestore(w http.ResponseWriter, r *http.Request, id string) {
	if _, ok := strictCoreQuery(w, r); !ok {
		return
	}
	var input PanelRestoreApply
	if !decodeStrictRequest(w, r, 16<<10, &input) {
		return
	}
	startedAt := time.Now()
	before, err := handler.services.Backups.RequestRestore(r.Context(), id, input.TakeService, input.Categories)
	handler.services.Telemetry.RecordOperation(r.Context(), "panel.restore.request", "Restore request", err,
		telemetry.OperationLogContext{StartedAt: startedAt, File: before.Name})
	if err != nil {
		writeBackupProblem(w, r, err)
		return
	}
	restarting := handler.restart != nil && handler.restart(r.Context()) == nil
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusAccepted, PanelRestoreRequested{StartedAt: handler.startedAt, Before: before, Restarting: restarting})
}

// validPassphrase accepts no passphrase or one of a usable length; a new
// backup's passphrase must be long enough to protect it.
func validPassphrase(w http.ResponseWriter, r *http.Request, passphrase string, creating bool) bool {
	if passphrase == "" || (len(passphrase) <= maximumPassphraseBytes && (!creating || len(passphrase) >= minimumPassphraseBytes)) {
		return true
	}
	writeProblem(w, r, http.StatusUnprocessableEntity, "backup_passphrase_invalid", "Passphrase invalid",
		"Use a passphrase of 8 to 1024 bytes.")
	return false
}

func writeBackupProblem(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, backup.ErrNotFound):
		writeProblem(w, r, http.StatusNotFound, "backup_not_found", "Backup not found", "The backup or staged restore no longer exists.")
	case errors.Is(err, backup.ErrBusy):
		writeProblem(w, r, http.StatusConflict, "backup_busy", "Backup busy", "Another backup or restore is running; try again when it has finished.")
	case errors.Is(err, backup.ErrPassphraseRequired):
		writeProblem(w, r, http.StatusUnprocessableEntity, "backup_passphrase_required", "Passphrase required", "The backup is encrypted; enter its passphrase.")
	case errors.Is(err, backup.ErrPassphraseIncorrect):
		writeProblem(w, r, http.StatusUnprocessableEntity, "backup_passphrase_incorrect", "Passphrase incorrect", "The passphrase does not open this backup.")
	case errors.Is(err, backup.ErrCategories):
		writeProblem(w, r, http.StatusUnprocessableEntity, "backup_categories_invalid", "Categories invalid", "Choose at least one category the backup holds.")
	case errors.Is(err, backup.ErrTooNew):
		writeProblem(w, r, http.StatusUnprocessableEntity, "backup_too_new", "Backup too new", "The backup was made by a newer panel version; update this panel first.")
	case errors.Is(err, backup.ErrInvalid):
		writeProblem(w, r, http.StatusUnprocessableEntity, "backup_invalid", "Backup invalid", "The file is not a complete panel backup of a supported version.")
	default:
		writeProblem(w, r, http.StatusServiceUnavailable, "backup_unavailable", "Backup unavailable", "The panel could not read or write its backups.")
	}
}
