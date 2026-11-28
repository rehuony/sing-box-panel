// SPDX-License-Identifier: GPL-3.0-or-later

// Package service runs this node's sing-box: verified core installs,
// configuration checks and activation, process supervision and recovery,
// rollback bundles, core logs and read-only filesystem browsing. Its state
// lives in the node store; the panel drives it with commands.
package service

import (
	"context"
	"crypto/rand"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/rehuony/sing-box-panel/internal/sing-box-node/corelogs"
	"github.com/rehuony/sing-box-panel/internal/sing-box-node/host"
	"github.com/rehuony/sing-box-panel/internal/sing-box-node/store"
)

// DefaultCoreLogPolicy bounds core log files until the panel first sends its
// policy.
var DefaultCoreLogPolicy = corelogs.Policy{RetentionDays: 7, MaxFileBytes: 32 << 20}

// Options supplies a Node's collaborators.
type Options struct {
	// DataDir holds installed cores, runtime files and core logs.
	DataDir string
	// Events receives operation outcomes and runtime events; nil discards
	// them.
	Events func(Event)
}

// Node runs the node's sing-box runtime.
type Node struct {
	coreLogsMu     sync.Mutex
	coreLogs       *corelogs.Files
	artifacts      ArtifactInstaller
	runtimeControl RuntimeController
	publicIP       *host.PublicIPDetector
	database       *store.Store
	dataDir        string
	events         func(Event)
	now            func() time.Time
	random         func([]byte) (int, error)
	removeFile     func(string) error
	runtime        RuntimeResolver
	traffic        trafficCollector
}

type RuntimeResolver interface {
	Resolve(context.Context) (RuntimeIdentity, error)
}

// New returns the node runtime over database with its files in dataDir.
func New(database *store.Store, options Options) (*Node, error) {
	if !filepath.IsAbs(options.DataDir) {
		return nil, errors.New("node data directory must be an absolute path")
	}
	return &Node{
		database:   database,
		dataDir:    options.DataDir,
		events:     options.Events,
		publicIP:   host.NewPublicIPDetector(),
		now:        time.Now,
		random:     rand.Read,
		removeFile: os.Remove,
		runtime:    NewRuntimeIdentityResolver(database),
	}, nil
}

// PublicIP resolves the host's public IP hint, waiting for detection.
func (node *Node) PublicIP(ctx context.Context) string { return node.publicIP.Resolve(ctx) }

// CachedPublicIP returns the last detected public IP hint without waiting.
func (node *Node) CachedPublicIP(ctx context.Context) string { return node.publicIP.Cached(ctx) }

// RunPublicIPDetection refreshes the public IP hint until ctx ends.
func (node *Node) RunPublicIPDetection(ctx context.Context) { node.publicIP.Run(ctx) }
