// SPDX-License-Identifier: GPL-3.0-or-later

// Package localnode is the panel's in-process stand-in for a connected node
// until node sessions carry commands. It runs this host's sing-box through the
// node packages and keeps the node's runtime evidence in the panel store.
package localnode

import (
	"context"
	"crypto/rand"
	"os"
	"sync"
	"time"

	"github.com/rehuony/sing-box-panel/internal/sing-box-node/corelogs"
	"github.com/rehuony/sing-box-panel/internal/sing-box-node/host"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/corecache"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/nodeconfig"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/settings"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/store"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/telemetry"
)

// Node runs the local sing-box runtime: core installs, configuration checks
// and activation, process supervision and recovery, host metrics, the public
// IP hint, core logs and traffic collection.
type Node struct {
	metricsFeed    *telemetry.Feed[MetricsStreamSnapshot]
	telemetry      *telemetry.Service
	cores          *corecache.Service
	configs        *nodeconfig.Service
	panelSettings  *settings.Service
	collector      trafficCollector
	coreLogsMu     sync.Mutex
	coreLogs       *corelogs.Files
	artifacts      ArtifactInstaller
	runtimeControl RuntimeController
	hostSampler    host.Sampler
	publicIP       *host.PublicIPDetector
	database       *store.Store
	now            func() time.Time
	random         func([]byte) (int, error)
	removeFile     func(string) error
	runtime        RuntimeResolver
	settings       settings.Settings
	settingsPath   string
}

type RuntimeResolver interface {
	Resolve(context.Context) (RuntimeIdentity, error)
}

// New returns the local node over the panel database. Its data directory and
// runtime settings come from panelSettings, whose commits update the core log
// policy.
func New(database *store.Store, records *telemetry.Service, panelSettings *settings.Service, configs *nodeconfig.Service, cores *corecache.Service) *Node {
	node := &Node{
		database:      database,
		telemetry:     records,
		cores:         cores,
		configs:       configs,
		panelSettings: panelSettings,
		settings:      panelSettings.Value(),
		settingsPath:  panelSettings.Path(),
		publicIP:      host.NewPublicIPDetector(),
		now:           time.Now,
		random:        rand.Read,
		removeFile:    os.Remove,
		runtime:       NewRuntimeIdentityResolver(database),
	}
	node.metricsFeed = telemetry.NewFeed(LiveTrafficSampleInterval, node.metricsSnapshot)
	panelSettings.OnCommit(node.applyCoreLogPolicy)
	return node
}

// PublicIP resolves the host's public IP hint, waiting for detection.
func (node *Node) PublicIP(ctx context.Context) string { return node.publicIP.Resolve(ctx) }

// CachedPublicIP returns the last detected public IP hint without waiting.
func (node *Node) CachedPublicIP(ctx context.Context) string { return node.publicIP.Cached(ctx) }

// RunPublicIPDetection refreshes the public IP hint until ctx ends.
func (node *Node) RunPublicIPDetection(ctx context.Context) { node.publicIP.Run(ctx) }

// UsePublicIPDetector replaces public IP detection, such as in tests.
func (node *Node) UsePublicIPDetector(detector *host.PublicIPDetector) { node.publicIP = detector }
