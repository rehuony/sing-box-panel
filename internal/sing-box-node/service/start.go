// SPDX-License-Identifier: GPL-3.0-or-later

package service

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"

	"github.com/rehuony/sing-box-panel/internal/sing-box-node/artifacts"
)

// Runtime is the node's started runtime services.
type Runtime struct {
	services *runtimeServices
	stops    []func() error
}

// Start restores the enabled core link, starts process supervision and core
// log retention, and reconciles the runtime with its recorded state. Close stops everything Start and StartRecovery started.
func (node *Node) Start(ctx context.Context) (*Runtime, error) {
	root := filepath.Join(node.dataDir, "artifacts")
	installed, err := artifacts.New(artifacts.Options{Root: root})
	if unsafe := (*artifacts.UnsafePathError)(nil); errors.As(err, &unsafe) {
		return nil, fmt.Errorf("open core artifact store: %w. Every directory above %s must belong to root or to the account "+
			"that runs the node and be writable only by its owner, so no other account can replace the cores the node runs", unsafe, root)
	}
	if err != nil {
		return nil, fmt.Errorf("open core artifact store: %w", err)
	}
	syncEnabledCore := func(ctx context.Context) error { return node.SyncEnabledCoreLink(ctx, installed) }
	if err := syncEnabledCore(ctx); err != nil {
		return nil, fmt.Errorf("restore enabled core link: %w", err)
	}
	node.artifacts = installed
	services, err := newRuntimeServices(node.database, node, node.dataDir)
	if err != nil {
		return nil, fmt.Errorf("construct sing-box runtime: %w", err)
	}
	runtime := &Runtime{services: services}
	retentionContext, stopRetention := context.WithCancel(ctx)
	retentionDone := startCoreLogRetention(retentionContext, node)
	runtime.stops = append(runtime.stops, func() error { stopRetention(); <-retentionDone; return nil })
	runtime.stops = append(runtime.stops, services.Close)
	services.lifetime = ctx
	services.syncEnabledCore = syncEnabledCore
	node.runtimeControl = services
	if err := services.ReconcileStartup(ctx); err != nil {
		return nil, errors.Join(fmt.Errorf("reconcile sing-box runtime: %w", err), runtime.Close())
	}
	node.record(Event{
		Source: SourceNode, Level: LevelInfo, Code: "runtime.reconciled",
		Message: "Runtime state reconciled at node startup",
	})
	return runtime, nil
}

// StartRecovery begins bounded recovery of a core that stopped unexpectedly.
func (runtime *Runtime) StartRecovery(ctx context.Context) {
	recoveryContext, stopRecovery := context.WithCancel(ctx)
	recoveryDone := startRuntimeReconciler(recoveryContext, runtime.services)
	runtime.stops = append(runtime.stops, func() error { stopRecovery(); <-recoveryDone; return nil })
}

// Handler serves the private runtime control socket.
func (runtime *Runtime) Handler() http.Handler { return runtime.services }

// LiveCore reports the core the runtime is running, if any.
func (runtime *Runtime) LiveCore() LiveCore {
	if runtime == nil {
		return LiveCore{}
	}
	live := runtime.services.manager.ObserveLiveIdentity()
	if !live.Running {
		return LiveCore{}
	}
	return LiveCore{Running: true, ExactVersion: live.ExactVersion.String(), ArtifactID: live.ArtifactID, ArtifactDigest: live.ArtifactDigest.String()}
}

// LiveCore identifies a verified running core.
type LiveCore struct {
	Running        bool
	ExactVersion   string
	ArtifactID     string
	ArtifactDigest string
}

// Close stops the runtime services in reverse start order.
func (runtime *Runtime) Close() error {
	if runtime == nil {
		return nil
	}
	var closeErr error
	for index := len(runtime.stops) - 1; index >= 0; index-- {
		closeErr = errors.Join(closeErr, runtime.stops[index]())
	}
	runtime.stops = nil
	return closeErr
}
