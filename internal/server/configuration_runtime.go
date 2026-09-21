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

func (services *runtimeServices) checkConfigurationForIntent(ctx context.Context, intent store.RuntimeIntent, control runtimeExecutionGuard) (store.RuntimeIntent, error) {
	if err := control.SafePoint(ctx); err != nil {
		return store.RuntimeIntent{}, err
	}
	material, err := services.commands.LoadConfigurationRuntimeCandidate(ctx, intent.StartupArtifactID)
	if err != nil {
		return store.RuntimeIntent{}, err
	}
	checkErr := services.manager.Check(ctx, material.Bundle)
	if ctx.Err() != nil {
		return store.RuntimeIntent{}, ctx.Err()
	}
	if err := control.SafePoint(ctx); err != nil {
		return store.RuntimeIntent{}, err
	}
	if material.Startup.State == store.StartupArtifactPending {
		if _, err := services.commands.CompleteStartupCheck(ctx, intent.StartupArtifactID, checkErr == nil); err != nil {
			return store.RuntimeIntent{}, errors.Join(checkErr, err)
		}
	}
	if checkErr != nil {
		return store.RuntimeIntent{}, checkErr
	}
	// Monitoring capability follows the actual saved configuration. Nothing is
	// injected into the user's file to make the monitoring endpoint exist.
	tier := store.MonitoringProcessOnly
	if _, err := coreruntime.ParseClashEndpoint(material.Startup.ConfigBytes); err == nil {
		tier = store.MonitoringLimited
	}
	prepared, err := services.commands.PrepareActivationBundle(ctx, intent.StartupArtifactID, tier)
	if err != nil {
		return store.RuntimeIntent{}, err
	}
	if err := control.SafePoint(ctx); err != nil {
		return store.RuntimeIntent{}, err
	}
	return services.database.BindCheckedRuntimeIntent(ctx, intent, prepared.Bundle.ID, time.Now().UTC())
}

// Selecting a stopped core commits checked startup evidence without fabricating
// a running identity or launching a process. Completion still fences generation,
// intent ownership and the absence of a live observation in the same transaction.
func (services *runtimeServices) selectStoppedCore(ctx context.Context, intent store.RuntimeIntent, control runtimeExecutionGuard) (runtimeResult, error) {
	if err := control.SafePoint(ctx); err != nil {
		return runtimeResult{}, err
	}
	if services.manager.ObserveLiveIdentity().Running {
		return runtimeResult{}, errors.New("core started during version selection")
	}
	if _, err := services.identity.Resolve(ctx); !errors.Is(err, application.ErrNoRunningCore) {
		return runtimeResult{}, errors.Join(errors.New("cannot prove the core is stopped"), err)
	}
	transition := runtimeTransitionWithoutObservation(
		runtimeIntentTransitionKey(intent, "selected"), store.RuntimeTransitionStopped,
		"core_selected", intent.ActivationBundleID, time.Now().UTC(), nil, intent,
	)
	return runtimeResult{Runtime: &store.RuntimeCommit{
		ClearObservation: true, Transitions: []store.RuntimeTransitionInput{transition},
	}}, nil
}
