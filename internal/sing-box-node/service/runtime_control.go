// SPDX-License-Identifier: GPL-3.0-or-later
package service

import (
	"context"
	"errors"

	"github.com/rehuony/sing-box-panel/internal/sing-box-node/store"
)

// RuntimeController serializes runtime changes; Start installs it.
type RuntimeController interface {
	ExecuteRuntime(context.Context, RuntimeRequest) (RuntimeResponse, error)
}
type RuntimeRequest struct {
	Action            string               `json:"action"`
	CoreID            string               `json:"core_id,omitempty"`
	BundleID          string               `json:"bundle_id,omitempty"`
	StartupArtifactID string               `json:"startup_artifact_id,omitempty"`
	MonitoringTier    store.MonitoringTier `json:"monitoring_tier,omitempty"`
}
type RuntimeResponse struct {
	Status     RuntimeStatus           `json:"status"`
	Activation *ActivationSummary      `json:"activation,omitempty"`
	Startup    *StartupArtifactSummary `json:"startup_artifact,omitempty"`
}

func (a *Node) ExecuteRuntime(ctx context.Context, input RuntimeRequest) (RuntimeResponse, error) {
	if a.runtimeControl == nil {
		return RuntimeResponse{}, ErrRuntimeNotStarted
	}
	return a.runtimeControl.ExecuteRuntime(ctx, input)
}

// ErrRuntimeNotStarted rejects runtime commands before Start.
var ErrRuntimeNotStarted = errors.New("the node runtime has not started")

func (a *Node) EnableCore(ctx context.Context, id string) (RuntimeStatus, error) {
	if _, err := a.database.GetCoreArtifact(ctx, id); err != nil {
		return RuntimeStatus{}, err
	}
	r, err := a.ExecuteRuntime(ctx, RuntimeRequest{Action: "enable", CoreID: id})
	return r.Status, err
}
func (a *Node) DisableCore(ctx context.Context, id string) (RuntimeStatus, error) {
	r, err := a.ExecuteRuntime(ctx, RuntimeRequest{Action: "disable", CoreID: id})
	return r.Status, err
}
func (a *Node) StartRuntime(ctx context.Context) (RuntimeStatus, error) {
	r, err := a.ExecuteRuntime(ctx, RuntimeRequest{Action: "start"})
	return r.Status, err
}
func (a *Node) StopRuntime(ctx context.Context) (RuntimeStatus, error) {
	r, err := a.ExecuteRuntime(ctx, RuntimeRequest{Action: "stop"})
	return r.Status, err
}
func (a *Node) RestartRuntime(ctx context.Context) (RuntimeStatus, error) {
	r, err := a.ExecuteRuntime(ctx, RuntimeRequest{Action: "restart"})
	return r.Status, err
}
func (a *Node) RollbackRuntime(ctx context.Context, id string) (RuntimeStatus, error) {
	r, err := a.ExecuteRuntime(ctx, RuntimeRequest{Action: "rollback", BundleID: id})
	return r.Status, err
}
func (a *Node) ActivateRuntime(ctx context.Context, id string, tier store.MonitoringTier) (RuntimeResponse, error) {
	return a.ExecuteRuntime(ctx, RuntimeRequest{Action: "activate", StartupArtifactID: id, MonitoringTier: tier})
}
func (a *Node) CheckStartup(ctx context.Context, id string) (StartupArtifactSummary, error) {
	r, err := a.ExecuteRuntime(ctx, RuntimeRequest{Action: "check", StartupArtifactID: id})
	if err != nil {
		return StartupArtifactSummary{}, err
	}
	if r.Startup == nil {
		return StartupArtifactSummary{}, errors.New("runtime check returned no result")
	}
	return *r.Startup, nil
}
