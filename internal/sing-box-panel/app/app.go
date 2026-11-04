// SPDX-License-Identifier: GPL-3.0-or-later

// Package app composes the panel's application services over one store and
// one settings file. The server, the command line and the tests of the
// transport packages share this composition.
package app

import (
	"context"

	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/auth"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/corecache"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/localnode"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/nodeconfig"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/settings"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/store"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/subscription"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/telemetry"
)

// Services are the panel use cases the management API and /sub delegate to.
type Services struct {
	Sessions      *auth.Sessions
	Settings      *settings.Service
	Configs       *nodeconfig.Service
	Cores         *corecache.Service
	Subscriptions *subscription.Service
	Telemetry     *telemetry.Service
	Node          *localnode.Node
}

// Options adjusts the composition.
type Options struct {
	// Observe receives every recorded panel log entry, such as for console
	// output.
	Observe func(store.LogEntry)
	// PublicIP replaces the local node's public IP detection.
	PublicIP func(context.Context) string
}

// New composes the panel's services over database and the settings file
// selected by configuration.
func New(database *store.Store, configuration settings.Settings, options Options) Services {
	var node *localnode.Node
	publicIP, publicIPCache := options.PublicIP, options.PublicIP
	if publicIP == nil {
		publicIP = func(ctx context.Context) string { return node.PublicIP(ctx) }
		publicIPCache = func(ctx context.Context) string { return node.CachedPublicIP(ctx) }
	}
	panelSettings := settings.NewService(database, configuration.Path(), configuration, settings.Options{PublicIP: publicIP})
	records := telemetry.New(database, panelSettings, telemetry.Options{Classify: localnode.ClassifyOperationFailure, Observe: options.Observe})
	configs := nodeconfig.New(database)
	cores := corecache.New(database, records, panelSettings, corecache.Options{DataDir: configuration.DataDir})
	node = localnode.New(database, records, panelSettings, configs, cores)
	return Services{
		Sessions:      auth.NewSessions(database, panelSettings),
		Settings:      panelSettings,
		Configs:       configs,
		Cores:         cores,
		Subscriptions: subscription.New(database, panelSettings, records, subscription.Options{PublicIP: publicIP, PublicIPCache: publicIPCache}),
		Telemetry:     records,
		Node:          node,
	}
}
