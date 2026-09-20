// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"context"
	"errors"
	"time"

	"github.com/rehuony/sing-box-panel/internal/application"
	coreruntime "github.com/rehuony/sing-box-panel/internal/runtime"
	"github.com/rehuony/sing-box-panel/internal/store"
)

func (services *runtimeServices) checkConfigurationForTask(ctx context.Context, task store.Task, control taskExecutionControl) (store.Task, error) {
	if err := control.SafePoint(ctx); err != nil {
		return store.Task{}, err
	}
	material, err := services.commands.LoadConfigurationRuntimeCandidate(ctx, task.StartupArtifactID)
	if err != nil {
		return store.Task{}, err
	}
	checkErr := services.manager.Check(ctx, material.Bundle)
	if ctx.Err() != nil {
		return store.Task{}, ctx.Err()
	}
	if err := control.SafePoint(ctx); err != nil {
		return store.Task{}, err
	}
	if material.Startup.State == store.StartupArtifactPending {
		if _, err := services.commands.CompleteStartupCheck(ctx, task.StartupArtifactID, checkErr == nil); err != nil {
			return store.Task{}, errors.Join(checkErr, err)
		}
	}
	if checkErr != nil {
		return store.Task{}, checkErr
	}
	// Monitoring capability follows the actual saved configuration. Nothing is
	// injected into the user's file to make the monitoring endpoint exist.
	tier := store.MonitoringProcessOnly
	if _, err := coreruntime.ParseClashEndpoint(material.Startup.ConfigBytes); err == nil {
		tier = store.MonitoringLimited
	}
	prepared, err := services.commands.PrepareActivationBundle(ctx, task.StartupArtifactID, tier)
	if err != nil {
		return store.Task{}, err
	}
	if err := control.SafePoint(ctx); err != nil {
		return store.Task{}, err
	}
	return services.database.BindCheckedRuntimeTask(ctx, task, prepared.Bundle.ID, time.Now().UTC())
}

// Selecting a stopped core commits checked startup evidence without fabricating
// a running identity or launching a process. Completion still fences generation,
// task ownership and the absence of a live observation in the same transaction.
func (services *runtimeServices) selectStoppedCore(ctx context.Context, task store.Task, control taskExecutionControl) (taskHandlerResult, error) {
	if err := control.SafePoint(ctx); err != nil {
		return taskHandlerResult{}, err
	}
	if services.manager.ObserveLiveIdentity().Running {
		return taskHandlerResult{}, errors.New("core started during version selection")
	}
	if _, err := services.identity.Resolve(ctx); !errors.Is(err, application.ErrNoRunningCore) {
		return taskHandlerResult{}, errors.Join(errors.New("cannot prove the core is stopped"), err)
	}
	transition := runtimeTransitionWithoutObservation(
		runtimeTaskTransitionKey(task, "selected"), store.RuntimeTransitionStopped,
		"core_selected", task.ActivationBundleID, time.Now().UTC(), nil, task,
	)
	return taskHandlerResult{Runtime: &store.RuntimeTaskCommit{
		ClearObservation: true, Transitions: []store.RuntimeTransitionInput{transition},
	}}, nil
}
