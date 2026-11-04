// SPDX-License-Identifier: GPL-3.0-or-later

package corecache

import (
	"context"
	"errors"
	"os"
	"sync"
	"time"

	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/settings"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/store"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/telemetry"
)

// Options supplies a Service's collaborators.
type Options struct {
	// DataDir holds the private core upload staging directory.
	DataDir string
	// RemoveFile removes staged uploads; it defaults to os.Remove.
	RemoveFile func(string) error
}

// Service owns the core version catalog, staged core uploads and the
// recorded core artifacts.
type Service struct {
	database   *store.Store
	telemetry  *telemetry.Service
	settings   *settings.Service
	dataDir    string
	now        func() time.Time
	removeFile func(string) error
	catalogMu  sync.Mutex
}

// New serves the core catalog over database. Catalog discovery reads its
// GitHub token and refresh interval from panelSettings at each refresh.
func New(database *store.Store, records *telemetry.Service, panelSettings *settings.Service, options Options) *Service {
	service := &Service{
		database: database, telemetry: records, settings: panelSettings, dataDir: options.DataDir,
		now: time.Now, removeFile: os.Remove,
	}
	if options.RemoveFile != nil {
		service.removeFile = options.RemoveFile
	}
	return service
}

func (service *Service) catalogSettings(ctx context.Context) (settings.GitHub, error) {
	if service.settings == nil {
		return settings.GitHub{}, errors.New("catalog settings are unavailable")
	}
	value, err := service.settings.EffectiveSettings(ctx)
	return value.GitHub, err
}
