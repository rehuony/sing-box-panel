// SPDX-License-Identifier: GPL-3.0-or-later

package localnode

import (
	"crypto/rand"
	"os"
	"time"

	"github.com/rehuony/sing-box-panel/internal/sing-box-node/host"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/corecache"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/nodeconfig"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/settings"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/store"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/subscription"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/telemetry"
)

// newApplication composes a node over database with read-only empty
// settings. Tests that change node.settings or node.settingsPath call rebind.
func newApplication(database *store.Store) *Node {
	node := &Node{
		database: database, now: time.Now, random: rand.Read, removeFile: os.Remove,
		runtime: NewRuntimeIdentityResolver(database), publicIP: host.NewPublicIPDetector(),
	}
	node.metricsFeed = telemetry.NewFeed(LiveTrafficSampleInterval, node.metricsSnapshot)
	node.rebind()
	return node
}

// rebind rebuilds the panel services from node.settings and node.settingsPath,
// following the node's clock and random source.
func (node *Node) rebind() {
	node.panelSettings = settings.NewService(node.database, node.settingsPath, node.settings, settings.Options{})
	node.panelSettings.OnCommit(node.applyCoreLogPolicy)
	node.telemetry = telemetry.New(node.database, node.panelSettings, telemetry.Options{
		Classify: ClassifyOperationFailure,
		Now:      func() time.Time { return node.now() },
		Random:   func(value []byte) (int, error) { return node.random(value) },
	})
	node.configs = nodeconfig.New(node.database)
	node.cores = corecache.New(node.database, node.telemetry, node.panelSettings, corecache.Options{
		DataDir:    node.settings.DataDir,
		RemoveFile: func(path string) error { return node.removeFile(path) },
	})
}

// subscriptions returns a subscription service over the node's services.
func (node *Node) subscriptions() *subscription.Service {
	return subscription.New(node.database, node.panelSettings, node.telemetry, subscription.Options{
		Now:    func() time.Time { return node.now() },
		Random: func(value []byte) (int, error) { return node.random(value) },
	})
}

func FromStore(database *store.Store) *Node { return newApplication(database) }

func FromStoreWithRuntimeResolver(database *store.Store, resolver RuntimeResolver) *Node {
	node := newApplication(database)
	node.runtime = resolver
	return node
}

func FromStoreWithSettings(database *store.Store, value settings.Settings) *Node {
	node := newApplication(database)
	node.settings = value
	node.settingsPath = value.Path()
	node.rebind()
	return node
}
