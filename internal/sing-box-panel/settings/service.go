// SPDX-License-Identifier: GPL-3.0-or-later

package settings

import (
	"crypto/rand"
	"time"

	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/store"
)

// Service reads and saves the panel settings file, recovers interrupted
// saves, and exports and restores panel backups.
type Service struct {
	listeners
	database     *store.Store
	settings     Settings
	settingsPath string
	now          func() time.Time
	random       func([]byte) (int, error)
}

// NewService serves the settings file at path, started with value. An empty
// path is a read-only view of value that cannot be saved.
func NewService(database *store.Store, path string, value Settings) *Service {
	return &Service{database: database, settings: value, settingsPath: path, now: time.Now, random: rand.Read}
}

// Path returns the settings file path, or "" for a read-only view.
func (app *Service) Path() string { return app.settingsPath }

// Value returns the settings the service was started with.
func (app *Service) Value() Settings { return app.settings }
