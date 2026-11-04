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

// OnCommit registers a hook that receives every committed settings value
// under the settings-file lock, in commit order. Hooks must not block or take
// the settings lock.
func (app *Service) OnCommit(hook func(Settings)) {
	app.hooksMu.Lock()
	defer app.hooksMu.Unlock()
	app.commitHooks = append(app.commitHooks, hook)
}

// publishSettings is called under the settings-file lock after a successful
// commit, preserving save order. Receivers do not run under this lock.
func (app *Service) publishSettings(value Settings) {
	app.hooksMu.Lock()
	hooks := append([]func(Settings){}, app.commitHooks...)
	app.hooksMu.Unlock()
	for _, hook := range hooks {
		hook(value)
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

type listeners struct {
	settingsListenersMu sync.Mutex
	settingsListeners   map[chan struct{}]struct{}
	hooksMu             sync.Mutex
	commitHooks         []func(Settings)
}
