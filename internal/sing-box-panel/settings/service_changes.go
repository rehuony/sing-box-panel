// SPDX-License-Identifier: GPL-3.0-or-later
package settings

import "sync"

// SettingsChanges coalesces notifications. Consumers load a fresh immutable
// snapshot at their operation boundary; cancellation never closes a sender's channel.
func (app *Service) SettingsChanges() (<-chan struct{}, func()) {
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

// publishSettings is called under the settings-file lock after a successful
// commit, preserving save order. Receivers do not run under this lock.
func (app *Service) publishSettings() {
	app.settingsListenersMu.Lock()
	defer app.settingsListenersMu.Unlock()
	for changes := range app.settingsListeners {
		select {
		case changes <- struct{}{}:
		default:
		}
	}
}

type listeners struct {
	settingsListenersMu sync.Mutex
	settingsListeners   map[chan struct{}]struct{}
}
