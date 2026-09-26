// SPDX-License-Identifier: GPL-3.0-or-later
package application

import (
	"github.com/rehuony/sing-box-panel/internal/corelogs"
	"github.com/rehuony/sing-box-panel/internal/settings"
)

// SettingsChanges coalesces notifications. Consumers load a fresh immutable
// snapshot at their operation boundary; cancellation never closes a sender's channel.
func (app *Application) SettingsChanges() (<-chan struct{}, func()) {
	changes := make(chan struct{}, 1)
	app.settingsListenersMu.Lock()
	if app.settingsListeners == nil {
		app.settingsListeners = make(map[chan struct{}]struct{})
	}
	app.settingsListeners[changes] = struct{}{}
	app.settingsListenersMu.Unlock()
	return changes, func() {
		app.settingsListenersMu.Lock()
		delete(app.settingsListeners, changes)
		app.settingsListenersMu.Unlock()
	}
}

// SetCoreLogs attaches the server-owned writer before requests are served.
func (app *Application) SetCoreLogs(files *corelogs.Files) {
	app.coreLogsMu.Lock()
	defer app.coreLogsMu.Unlock()
	app.coreLogs = files
}

func coreLogPolicy(value settings.Settings) corelogs.Policy {
	return corelogs.Policy{RetentionDays: value.Logs.CoreRetentionDays, MaxFiles: value.Logs.CoreMaxFiles, MaxFileBytes: int64(value.Logs.CoreMaxFileSizeMiB) << 20}
}

// publishSettings is called under the settings-file lock after a successful
// commit, preserving save order. Receivers do not run under this lock.
func (app *Application) publishSettings(value settings.Settings) {
	app.coreLogsMu.Lock()
	files := app.coreLogs
	app.coreLogsMu.Unlock()
	if files != nil {
		// Cleanup is retried by the retention worker; the validated policy is
		// installed even when removing an old file temporarily fails.
		_ = files.SetPolicy(coreLogPolicy(value))
	}
	app.settingsListenersMu.Lock()
	defer app.settingsListenersMu.Unlock()
	for changes := range app.settingsListeners {
		select {
		case changes <- struct{}{}:
		default:
		}
	}
}

func (app *Application) MaintainCoreLogs() error {
	app.coreLogsMu.Lock()
	defer app.coreLogsMu.Unlock()
	if app.coreLogs == nil {
		return nil
	}
	return app.coreLogs.EnsureCurrentFile()
}
