// SPDX-License-Identifier: GPL-3.0-or-later

// Package app composes the panel's application services over one store and
// one settings file. The server, the command line and the tests of the
// transport packages share this composition.
package app

import (
	"context"
	"errors"

	"github.com/rehuony/sing-box-panel/internal/buildinfo"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/auth"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/corecache"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/fleet"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/nodeconfig"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/releases"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/settings"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/store"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/subscription"
	panelsystemd "github.com/rehuony/sing-box-panel/internal/sing-box-panel/systemd"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/telemetry"
	"github.com/rehuony/sing-box-panel/internal/systemd"
)

// Services are the panel use cases the management API and /sub delegate to.
type Services struct {
	Sessions      *auth.Sessions
	Settings      *settings.Service
	Configs       *nodeconfig.Service
	Cores         *corecache.Service
	Subscriptions *subscription.Service
	Telemetry     *telemetry.Service
	Fleet         *fleet.Service
	Releases      *releases.Service
}

// Options adjusts the composition.
type Options struct {
	// Build is the panel's build, reported to nodes.
	Build buildinfo.Info
	// Observe receives every recorded panel log entry, such as for console
	// output.
	Observe func(store.LogEntry)
	// Downloader replaces the downloader of official core archives.
	Downloader corecache.Downloader
	// Releases adjusts where the panel's releases come from.
	Releases releases.Options
}

// New composes the panel's services over database and the settings file
// selected by configuration.
func New(database *store.Store, configuration settings.Settings, options Options) Services {
	panelSettings := settings.NewService(database, configuration.Path(), configuration)
	records := telemetry.New(database, panelSettings, telemetry.Options{Classify: fleet.ClassifyOperationFailure, Observe: options.Observe})
	cores := corecache.New(database, records, panelSettings, corecache.Options{DataDir: configuration.DataDir, Downloader: options.Downloader})
	configs := nodeconfig.New(database, cores)
	releaseOptions := options.Releases
	if releaseOptions.Restart == nil {
		releaseOptions.Restart = restartPanel
	}
	panelReleases := releases.New(options.Build, configuration.DataDir, panelSettings, records, releaseOptions)
	subscriptions := subscription.New(database, panelSettings, records, subscription.Options{})
	return Services{
		Sessions:      auth.NewSessions(database, panelSettings),
		Settings:      panelSettings,
		Configs:       configs,
		Cores:         cores,
		Subscriptions: subscriptions,
		Telemetry:     records,
		Fleet: fleet.New(database, panelSettings, records, configs, cores, fleet.Options{
			Build: options.Build, Releases: panelReleases, PublicationChanged: subscriptions.PoolInputsChanged,
		}),
		Releases: panelReleases,
	}
}

// restartPanel queues a restart of the systemd unit that runs the panel.
func restartPanel(ctx context.Context) error {
	scope, supervised := systemd.RunningScope()
	if !supervised {
		return errors.New("the panel does not run under systemd")
	}
	manager, err := panelsystemd.New(systemd.Options{})
	if err != nil {
		return err
	}
	return manager.RequestRestart(ctx, scope)
}
