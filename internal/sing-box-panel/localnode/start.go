// SPDX-License-Identifier: GPL-3.0-or-later

package localnode

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"

	"github.com/rehuony/sing-box-panel/internal/sing-box-node/artifacts"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/store"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/telemetry"
)

// Runtime is the local node's started runtime services.
type Runtime struct {
	services *runtimeServices
	stops    []func() error
}

// Start restores the enabled core link, starts process supervision, core log
// retention and traffic sampling, and reconciles the runtime with its
// recorded state. Close stops everything Start and StartRecovery started.
func (node *Node) Start(ctx context.Context) (*Runtime, error) {
	installed, err := artifacts.New(artifacts.Options{Root: filepath.Join(node.settings.DataDir, "artifacts")})
	if err != nil {
		return nil, fmt.Errorf("open core artifact store: %w", err)
	}
	syncEnabledCore := func(ctx context.Context) error { return node.SyncEnabledCoreLink(ctx, installed) }
	if err := syncEnabledCore(ctx); err != nil {
		return nil, fmt.Errorf("restore enabled core link: %w", err)
	}
	node.SetArtifactInstaller(installed)
	services, err := newRuntimeServices(node.database, node, node.settings)
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
	node.SetRuntimeController(services)
	trafficContext, stopTraffic := context.WithCancel(ctx)
	trafficDone := startTrafficSampler(trafficContext, services)
	runtime.stops = append(runtime.stops, func() error { stopTraffic(); <-trafficDone; return nil })
	if err := services.ReconcileStartup(ctx); err != nil {
		return nil, errors.Join(fmt.Errorf("reconcile sing-box runtime: %w", err), runtime.Close())
	}
	node.telemetry.RecordEvent(telemetry.LogRecordRequest{
		Source: store.LogSourcePanel, Level: store.LogLevelInfo, Code: "runtime.reconciled",
		Message: "Runtime state reconciled at panel startup", Metadata: []byte(`{}`),
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
