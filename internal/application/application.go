// SPDX-License-Identifier: GPL-3.0-or-later

// Package application exposes use cases shared by CLI and HTTP transports.
package application

import (
	"context"
	"crypto/rand"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/rehuony/sing-box-panel/internal/configuration"
	"github.com/rehuony/sing-box-panel/internal/settings"
	"github.com/rehuony/sing-box-panel/internal/store"
)

type Application struct {
	database              *store.Store
	ownsDatabase          bool
	now                   func() time.Time
	random                func([]byte) (int, error)
	removeFile            func(string) error
	runtime               RuntimeResolver
	settings              settings.Settings
	configurationAdapters *configuration.AdapterRegistry
}

type RuntimeResolver interface {
	Resolve(context.Context) (RuntimeIdentity, error)
}

// Open resolves bootstrap settings and opens the SQLite source of truth.
func Open(ctx context.Context, settingsPath string) (*Application, error) {
	configuration, err := settings.Load(settingsPath)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(configuration.DataDir)
	if err != nil {
		return nil, fmt.Errorf("inspect data directory: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("data path is not a directory: %s", configuration.DataDir)
	}
	database, err := store.Open(ctx, filepath.Join(configuration.DataDir, "panel.db"))
	if err != nil {
		return nil, err
	}
	application := newApplication(database)
	application.ownsDatabase = true
	application.settings = configuration
	return application, nil
}

func newApplication(database *store.Store) *Application {
	return &Application{
		database:              database,
		now:                   time.Now,
		random:                rand.Read,
		removeFile:            os.Remove,
		runtime:               NewRuntimeIdentityResolver(database),
		configurationAdapters: compiledConfigurationRegistry,
	}
}

// FromStore exposes application use cases over a server-owned Store. Closing
// the returned value does not close the shared database.
func FromStore(database *store.Store) *Application {
	return newApplication(database)
}

// FromStoreWithRuntimeResolver exposes application use cases over a
// server-owned Store while replacing only live runtime identity resolution.
// Production composition should normally use FromStore or Open.
func FromStoreWithRuntimeResolver(database *store.Store, resolver RuntimeResolver) *Application {
	application := newApplication(database)
	application.runtime = resolver
	return application
}

// FromStoreWithSettings exposes server-owned services that also require the
// configured data root or authenticated upstream credentials.
func FromStoreWithSettings(database *store.Store, configuration settings.Settings) *Application {
	application := newApplication(database)
	application.settings = configuration
	return application
}

func (application *Application) Close() error {
	if application == nil {
		return nil
	}
	if !application.ownsDatabase {
		return nil
	}
	return application.database.Close()
}
