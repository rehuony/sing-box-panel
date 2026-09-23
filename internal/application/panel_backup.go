// SPDX-License-Identifier: GPL-3.0-or-later
package application

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/rehuony/sing-box-panel/internal/configuration"
	"github.com/rehuony/sing-box-panel/internal/settings"
	"github.com/rehuony/sing-box-panel/internal/store"
)

const PanelBackupFormat = "sing-box-panel-backup"
const PanelBackupVersion = 1

var ErrPanelBackupInvalid = errors.New("panel backup is invalid or unsupported")

// PanelBackup intentionally contains credentials. It is only returned by the
// authenticated export operation, never by normal settings or audit responses.
type PanelBackup struct {
	Format               string          `json:"format"`
	Version              int             `json:"version"`
	ExportedAt           time.Time       `json:"exported_at"`
	PanelSettings        json.RawMessage `json:"panel_settings"`
	SingBoxConfiguration string          `json:"sing_box_configuration"`
}

type PanelRestoreRequest struct {
	Backup                PanelBackup `json:"backup"`
	SettingsRevision      int64       `json:"settings_revision"`
	ConfigurationRevision int64       `json:"configuration_revision"`
}

type PanelRestoreResult struct {
	Settings                 PanelSettingsView `json:"settings"`
	ReauthenticationRequired bool              `json:"reauthentication_required"`
}

func (app *Application) ExportPanelBackup(ctx context.Context) (PanelBackup, error) {
	if app.settingsPath == "" {
		return PanelBackup{}, ErrPanelBackupInvalid
	}
	lock, err := settings.Lock(ctx, app.settingsPath)
	if err != nil {
		return PanelBackup{}, err
	}
	defer lock.Close()
	if err := app.recoverSettingsFile(ctx); err != nil {
		return PanelBackup{}, err
	}
	raw, err := settings.Read(app.settingsPath)
	if err != nil {
		return PanelBackup{}, err
	}
	if _, err := settings.Parse(app.settingsPath, raw); err != nil {
		return PanelBackup{}, err
	}
	file, err := app.database.ConfigurationFile(ctx)
	if err != nil {
		return PanelBackup{}, err
	}
	return PanelBackup{Format: PanelBackupFormat, Version: PanelBackupVersion, ExportedAt: app.now().UTC(), PanelSettings: raw, SingBoxConfiguration: file.Content}, nil
}

func (app *Application) RestorePanelBackup(ctx context.Context, input PanelRestoreRequest) (result PanelRestoreResult, operationErr error) {
	defer func() { app.RecordOperation(ctx, "panel.backup.restore", "Panel configuration restore", operationErr) }()
	backup := input.Backup
	if app.settingsPath == "" || backup.Format != PanelBackupFormat || backup.Version != PanelBackupVersion || backup.ExportedAt.IsZero() || input.SettingsRevision < 0 || input.ConfigurationRevision < 0 || len(backup.PanelSettings) > settings.MaximumBytes || len(backup.SingBoxConfiguration) > configuration.MaximumBytes || !utf8.ValidString(backup.SingBoxConfiguration) || strings.ContainsRune(backup.SingBoxConfiguration, '\x00') {
		return result, ErrPanelBackupInvalid
	}
	next, err := settings.Parse(app.settingsPath, backup.PanelSettings)
	if err != nil {
		return result, ErrPanelBackupInvalid
	}
	lock, err := settings.Lock(ctx, app.settingsPath)
	if err != nil {
		return result, err
	}
	defer lock.Close()
	if err := app.recoverSettingsFile(ctx); err != nil {
		return result, err
	}
	before, err := settings.Read(app.settingsPath)
	if err != nil {
		return result, err
	}
	if settings.Revision(before) != input.SettingsRevision {
		return result, store.ErrPanelSettingsConflict
	}
	previous, err := settings.Parse(app.settingsPath, before)
	if err != nil {
		return result, err
	}
	update, err := app.configurationFileUpdate(ConfigurationFileWrite{Revision: input.ConfigurationRevision, Content: backup.SingBoxConfiguration})
	if err != nil {
		return result, err
	}
	// Keep relative paths and exact sing-box text. Restoring both sources must
	// not run the identity editor or apply/restart the currently running core.
	after := append(append([]byte(nil), backup.PanelSettings...), '\n')
	if len(after) > settings.MaximumBytes {
		return result, ErrPanelBackupInvalid
	}
	if err := app.commitSettingsFile(ctx, before, after, update); err != nil {
		return result, err
	}
	app.publishSettings(next)
	return PanelRestoreResult{Settings: app.panelSettingsView(next, settings.Revision(after)), ReauthenticationRequired: previous.Auth.Token != next.Auth.Token}, nil
}
