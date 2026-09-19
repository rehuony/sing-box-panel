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

// EnableCore queues a checked replacement, preserving the running core until
// the candidate's saved configuration passes binary validation.
func (application *Application) EnableCore(ctx context.Context, coreID string) (Task, error) {
	core, err := application.database.GetCoreArtifact(ctx, coreID)
	if err != nil {
		return Task{}, err
	}
	if core.VerificationState != store.CoreArtifactVerified {
		return Task{}, ErrCoreArtifactVerificationBlocked
	}
	if core.OperatingSystem != runtime.GOOS || core.Architecture != runtime.GOARCH {
		return Task{}, ErrCorePlatformMismatch
	}
	return application.QueueConfigurationRuntime(ctx, coreID, store.RuntimeIntentRestart)
}

// AppliedCoreArtifactID returns the core artifact bound to the applied
// activation bundle, or "" before any core has been applied. Callers that
// omit an explicit core use it so version selection stays explicit and never
// falls back to the newest catalog release.
func (application *Application) AppliedCoreArtifactID(ctx context.Context) (string, error) {
	bootstrap, err := application.database.Bootstrap(ctx)
	if err != nil {
		return "", err
	}
	if bootstrap.Hub.AppliedBundleID == "" {
		return "", nil
	}
	bundle, err := application.database.GetActivationBundle(ctx, bootstrap.Hub.AppliedBundleID)
	if err != nil {
		return "", err
	}
	startup, err := application.database.GetStartupArtifact(ctx, bundle.StartupArtifactID)
	if err != nil {
		return "", err
	}
	return startup.CoreArtifactID, nil
}

// QueueConfigurationRuntime snapshots the current file for a fresh binary
// preflight in the serialized runtime lane. Empty coreID retains the applied
// binary identity; version selection may supply an explicit verified artifact.
func (application *Application) QueueConfigurationRuntime(ctx context.Context, coreID string, kind store.RuntimeIntentKind) (Task, error) {
	coreID = strings.TrimSpace(coreID)
	var appliedCanonical string
	if coreID == "" {
		bootstrap, err := application.database.Bootstrap(ctx)
		if err != nil {
			return Task{}, err
		}
		if bootstrap.Hub.AppliedBundleID == "" {
			return Task{}, store.ErrNoAppliedBundle
		}
		material, err := application.LoadRuntimeMaterial(ctx, bootstrap.Hub.AppliedBundleID)
		if err != nil {
			return Task{}, err
		}
		coreID = material.Core.ID
		appliedCanonical = material.Startup.CanonicalRevisionID
	}
	preview, err := application.PreviewConfiguration(ctx, ConfigurationPreviewRequest{CoreArtifactID: coreID})
	if err != nil {
		return Task{}, err
	}
	if kind == store.RuntimeIntentStart && appliedCanonical == preview.CanonicalRevision.ID {
		return application.queueRuntimeIntent(ctx, store.RuntimeIntentStart, "")
	}
	if preview.Support.Structured {
		if err := singbox.ValidateConfiguration(preview.CoreArtifact.ExactVersion, preview.Config); err != nil {
			return Task{}, fmt.Errorf("%w: %v", ErrConfigurationSchemaValidation, err)
		}
	}
	startupID, err := application.newID("startup")
	if err != nil {
		return Task{}, err
	}
	taskID, err := application.newID("task")
	if err != nil {
		return Task{}, err
	}
	now := application.now().UTC()
	task, err := application.database.RequestConfigurationRuntimeIntent(ctx, store.RuntimeIntentInput{TaskID: taskID, Kind: kind, CreatedAt: now}, store.StartupArtifact{
		ID: startupID, CanonicalRevisionID: preview.CanonicalRevision.ID, ExactCoreVersion: preview.CoreArtifact.ExactVersion,
		CoreArtifactID: coreID, ConfigBytes: preview.Config, CreatedAt: now,
	})
	if err != nil {
		return Task{}, err
	}
	return applicationTask(task), nil
}

// LoadConfigurationRuntimeCandidate also accepts ready candidates after a
// worker crash between preflight and binding. Rechecking is safe and required.
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
