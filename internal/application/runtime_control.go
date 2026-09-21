// SPDX-License-Identifier: GPL-3.0-or-later
package application

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/rehuony/sing-box-panel/internal/panelprocess"
	"github.com/rehuony/sing-box-panel/internal/store"
)

// RuntimeController is implemented by the server owning the process lease.
// Local CLI invocations call that same controller through its private socket.
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

func (a *Application) SetRuntimeController(controller RuntimeController) {
	a.runtimeControl = controller
}
func (a *Application) ExecuteRuntime(ctx context.Context, input RuntimeRequest) (RuntimeResponse, error) {
	if a.runtimeControl != nil {
		return a.runtimeControl.ExecuteRuntime(ctx, input)
	}
	raw, err := panelprocess.CallRuntime(ctx, a.settings.DataDir, input)
	if err != nil {
		return RuntimeResponse{}, err
	}
	var response struct {
		Result RuntimeResponse      `json:"result"`
		Error  *RuntimeControlError `json:"error"`
	}
	if err := json.Unmarshal(raw, &response); err != nil {
		return RuntimeResponse{}, err
	}
	if response.Error != nil {
		return RuntimeResponse{}, response.Error
	}
	return response.Result, nil
}
func (a *Application) EnableCore(ctx context.Context, id string) (RuntimeStatus, error) {
	if _, err := a.CoreArtifact(ctx, id); err != nil {
		return RuntimeStatus{}, err
	}
	r, err := a.ExecuteRuntime(ctx, RuntimeRequest{Action: "enable", CoreID: id})
	return r.Status, err
}
func (a *Application) DisableCore(ctx context.Context, id string) (RuntimeStatus, error) {
	r, err := a.ExecuteRuntime(ctx, RuntimeRequest{Action: "disable", CoreID: id})
	return r.Status, err
}
func (a *Application) StartRuntime(ctx context.Context) (RuntimeStatus, error) {
	r, err := a.ExecuteRuntime(ctx, RuntimeRequest{Action: "start"})
	return r.Status, err
}
func (a *Application) StopRuntime(ctx context.Context) (RuntimeStatus, error) {
	r, err := a.ExecuteRuntime(ctx, RuntimeRequest{Action: "stop"})
	return r.Status, err
}
func (a *Application) RestartRuntime(ctx context.Context) (RuntimeStatus, error) {
	r, err := a.ExecuteRuntime(ctx, RuntimeRequest{Action: "restart"})
	return r.Status, err
}
func (a *Application) RollbackRuntime(ctx context.Context, id string) (RuntimeStatus, error) {
	r, err := a.ExecuteRuntime(ctx, RuntimeRequest{Action: "rollback", BundleID: id})
	return r.Status, err
}
func (a *Application) ActivateRuntime(ctx context.Context, id string, tier store.MonitoringTier) (RuntimeResponse, error) {
	return a.ExecuteRuntime(ctx, RuntimeRequest{Action: "activate", StartupArtifactID: id, MonitoringTier: tier})
}
func (a *Application) CheckStartup(ctx context.Context, id string) (StartupArtifactSummary, error) {
	r, err := a.ExecuteRuntime(ctx, RuntimeRequest{Action: "check", StartupArtifactID: id})
	if err != nil {
		return StartupArtifactSummary{}, err
	}
	if r.Startup == nil {
		return StartupArtifactSummary{}, errors.New("runtime check returned no result")
	}
	return *r.Startup, nil
}
