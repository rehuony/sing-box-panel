// SPDX-License-Identifier: GPL-3.0-or-later

package application

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"strings"

	"github.com/rehuony/sing-box-panel/internal/singbox"
	"github.com/rehuony/sing-box-panel/internal/store"
)

var ErrCorePlatformMismatch = errors.New("core platform does not match the deployed panel")

// PrepareCoreEnable selects a checked binary while preserving whether the core is running.
// A running core is replaced only after the candidate passes binary validation.
func (application *Application) PrepareCoreEnable(ctx context.Context, coreID string) (store.RuntimeIntent, error) {
	core, err := application.database.GetCoreArtifact(ctx, coreID)
	if err != nil {
		return store.RuntimeIntent{}, err
	}
	if core.OperatingSystem != runtime.GOOS || core.Architecture != runtime.GOARCH {
		return store.RuntimeIntent{}, ErrCorePlatformMismatch
	}
	_, err = application.runtime.Resolve(ctx)
	selectOnly := errors.Is(err, ErrNoRunningCore)
	if err != nil && !selectOnly {
		return store.RuntimeIntent{}, err
	}
	return application.prepareConfigurationRuntime(ctx, coreID, store.RuntimeIntentRestart, selectOnly)
}

// PrepareCoreDisable stops the process and clears the selected version. A normal
// runtime stop retains the selection so that it can be started again.
func (application *Application) PrepareCoreDisable(ctx context.Context, coreID string) (store.RuntimeIntent, error) {
	if _, err := application.database.GetCoreArtifact(ctx, coreID); err != nil {
		return store.RuntimeIntent{}, err
	}
	intent, err := application.database.RequestRuntimeIntent(ctx, store.RuntimeIntentInput{
		Kind: store.RuntimeIntentStop, DisableCoreID: coreID, CreatedAt: application.now().UTC(),
	})
	if err != nil {
		return store.RuntimeIntent{}, err
	}
	return intent, nil
}

// PrepareConfigurationRuntime snapshots the current file for a fresh binary
// preflight in the serialized runtime controller. Empty coreID retains the applied
// binary identity; version selection may supply an explicit verified artifact.
func (application *Application) PrepareConfigurationRuntime(ctx context.Context, coreID string, kind store.RuntimeIntentKind) (store.RuntimeIntent, error) {
	return application.prepareConfigurationRuntime(ctx, coreID, kind, false)
}

func (application *Application) prepareConfigurationRuntime(ctx context.Context, coreID string, kind store.RuntimeIntentKind, selectOnly bool) (store.RuntimeIntent, error) {
	coreID = strings.TrimSpace(coreID)
	var appliedCanonical string
	if coreID == "" {
		bootstrap, err := application.database.Bootstrap(ctx)
		if err != nil {
			return store.RuntimeIntent{}, err
		}
		if bootstrap.Hub.AppliedBundleID == "" {
			return store.RuntimeIntent{}, store.ErrNoAppliedBundle
		}
		material, err := application.LoadRuntimeMaterial(ctx, bootstrap.Hub.AppliedBundleID)
		if err != nil {
			return store.RuntimeIntent{}, err
		}
		coreID = material.Core.ID
		appliedCanonical = material.Startup.CanonicalRevisionID
	}
	preview, err := application.PreviewConfiguration(ctx, ConfigurationPreviewRequest{CoreArtifactID: coreID})
	if err != nil {
		return store.RuntimeIntent{}, err
	}
	if kind == store.RuntimeIntentStart && appliedCanonical == preview.CanonicalRevision.ID {
		return application.PrepareRuntimeIntent(ctx, store.RuntimeIntentStart, "")
	}
	if preview.Support.Structured {
		if err := singbox.ValidateConfiguration(preview.CoreArtifact.ExactVersion, preview.Config); err != nil {
			return store.RuntimeIntent{}, fmt.Errorf("%w: %v", ErrConfigurationSchemaValidation, err)
		}
	}
	startupID, err := application.newID("startup")
	if err != nil {
		return store.RuntimeIntent{}, err
	}
	now := application.now().UTC()
	intent, err := application.database.RequestConfigurationRuntimeIntent(ctx, store.RuntimeIntentInput{Kind: kind, CreatedAt: now, SelectOnly: selectOnly}, store.StartupArtifact{
		ID: startupID, CanonicalRevisionID: preview.CanonicalRevision.ID, ExactCoreVersion: preview.CoreArtifact.ExactVersion,
		CoreArtifactID: coreID, ConfigBytes: preview.Config, CreatedAt: now,
	})
	if err != nil {
		return store.RuntimeIntent{}, err
	}
	return intent, nil
}

// LoadConfigurationRuntimeCandidate also accepts ready candidates after a
// interrupted request between preflight and binding. Rechecking is required.
func (application *Application) LoadConfigurationRuntimeCandidate(ctx context.Context, id string) (RuntimeMaterial, error) {
	startup, err := application.database.GetStartupArtifact(ctx, id)
	if err != nil {
		return RuntimeMaterial{}, err
	}
	if startup.State != store.StartupArtifactPending && startup.State != store.StartupArtifactReady {
		return RuntimeMaterial{}, store.ErrStartupArtifactState
	}
	return application.runtimeMaterial(ctx, startup.ID, store.ActivationBundle{}, startup)
}
