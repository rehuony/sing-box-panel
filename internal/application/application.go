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

	"github.com/rehuony/sing-box-panel/internal/hostmetrics"
	"github.com/rehuony/sing-box-panel/internal/publicip"
	"github.com/rehuony/sing-box-panel/internal/settings"
	"github.com/rehuony/sing-box-panel/internal/store"
)

type Application struct {
	hostSampler  hostmetrics.Sampler
	database     *store.Store
	ownsDatabase bool
	now          func() time.Time
	random       func([]byte) (int, error)
	removeFile   func(string) error
	runtime      RuntimeResolver
	settings     settings.Settings
	settingsPath string
	publicIP     func(context.Context) string
}

type RuntimeResolver interface {
	Resolve(context.Context) (RuntimeIdentity, error)
}

// Open resolves only the data directory and opens SQLite for local commands.
// Runtime policy is validated by the server, which uses FromStoreWithSettings.
func Open(ctx context.Context, settingsPath string) (*Application, error) {
	dataDir, err := settings.LoadDataDir(settingsPath)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(dataDir)
	if err != nil {
		return nil, fmt.Errorf("inspect data directory: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("data path is not a directory: %s", dataDir)
	}
	database, err := store.Open(ctx, filepath.Join(dataDir, "panel.db"))
	if err != nil {
		return nil, err
	}
	application := newApplication(database)
	application.ownsDatabase = true
	application.settings.DataDir = dataDir
	application.settingsPath, err = filepath.Abs(settingsPath)
	if err != nil {
		database.Close()
		return nil, err
	}
	application.publicIP = publicip.New().Resolve
	return application, nil
}

// SetPublicIPResolver injects server-owned detection before requests are served.
func (application *Application) SetPublicIPResolver(resolve func(context.Context) string) {
	application.publicIP = resolve
}

func newApplication(database *store.Store) *Application {
	return &Application{
		database:   database,
		now:        time.Now,
		random:     rand.Read,
		removeFile: os.Remove,
		runtime:    NewRuntimeIdentityResolver(database),
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
	application.settingsPath = configuration.Path()
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
