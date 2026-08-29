// SPDX-License-Identifier: GPL-3.0-or-later

package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/rehuony/sing-box-panel/internal/configuration"
	"github.com/rehuony/sing-box-panel/internal/coreartifact"
	coreruntime "github.com/rehuony/sing-box-panel/internal/runtime"
	"github.com/rehuony/sing-box-panel/internal/store"
)

var ErrMonitoringTierUnavailable = errors.New("requested monitoring tier is unavailable")

type RuntimeStatus struct {
	DesiredRunning   bool             `json:"desired_running"`
	DesiredBundleID  string           `json:"desired_bundle_id,omitempty"`
	AppliedBundleID  string           `json:"applied_bundle_id,omitempty"`
	RollbackBundleID string           `json:"rollback_bundle_id,omitempty"`
	TargetGeneration int64            `json:"target_generation"`
	ObservationState string           `json:"observation_state"`
	Running          *RuntimeIdentity `json:"running,omitempty"`
}

type ActivationPreparation struct {
	StartupArtifact store.StartupArtifact  `json:"startup_artifact"`
	Bundle          store.ActivationBundle `json:"bundle"`
}

// ActivationSummary is safe for management transports. It deliberately omits
// startup bytes, rendered subscription content, source credentials, and
// discovered addresses while retaining every immutable identity and digest an
// operator needs to audit an apply.
type ActivationSummary struct {
	StartupArtifactID   string               `json:"startup_artifact_id"`
	CanonicalRevisionID string               `json:"canonical_revision_id"`
	ExactCoreVersion    string               `json:"exact_core_version"`
	CoreArtifactID      string               `json:"core_artifact_id"`
	ConfigSHA256        string               `json:"config_sha256"`
	ActivationBundleID  string               `json:"activation_bundle_id"`
	ActivationSHA256    string               `json:"activation_sha256"`
	MonitoringTier      store.MonitoringTier `json:"monitoring_tier"`
}

func (preparation ActivationPreparation) Summary() ActivationSummary {
	return ActivationSummary{
		StartupArtifactID:   preparation.StartupArtifact.ID,
		CanonicalRevisionID: preparation.StartupArtifact.CanonicalRevisionID,
		ExactCoreVersion:    preparation.StartupArtifact.ExactCoreVersion,
		CoreArtifactID:      preparation.StartupArtifact.CoreArtifactID,
		ConfigSHA256:        preparation.StartupArtifact.ConfigSHA256,
		ActivationBundleID:  preparation.Bundle.ID,
		ActivationSHA256:    preparation.Bundle.SHA256,
		MonitoringTier:      preparation.Bundle.MonitoringTier,
	}
}

type RuntimeMaterial struct {
	Bundle     coreruntime.AppliedBundle
	Activation store.ActivationBundle
	Startup    store.StartupArtifact
	Core       store.CoreArtifact
}

// PrepareActivationBundle binds already-checked startup bytes to one runtime
// monitoring policy. Subscription publication remains live and independent.
func (application *Application) PrepareActivationBundle(
	ctx context.Context,
	startupArtifactID string,
	monitoring store.MonitoringTier,
) (ActivationPreparation, error) {
	startup, err := application.database.GetStartupArtifact(ctx, startupArtifactID)
	if err != nil {
		return ActivationPreparation{}, err
	}
	if startup.State != store.StartupArtifactReady {
		return ActivationPreparation{}, store.ErrActivationBundleNotReady
	}
	if monitoring == "" {
		monitoring = store.MonitoringProcessOnly
	}
	if monitoring != store.MonitoringProcessOnly && monitoring != store.MonitoringLimited {
		return ActivationPreparation{}, fmt.Errorf("%w: invalid monitoring tier %q", ErrMonitoringTierUnavailable, monitoring)
	}
	if monitoring == store.MonitoringLimited {
		if _, err := coreruntime.ParseClashEndpoint(startup.ConfigBytes); err != nil {
			return ActivationPreparation{}, fmt.Errorf("%w: %v", ErrMonitoringTierUnavailable, err)
		}
	}
	if err := application.verifyActivationCandidate(ctx, startup); err != nil {
		return ActivationPreparation{}, err
	}

	bundleID := stableRuntimeID("bundle", startup.ID, string(monitoring))
	if existing, getErr := application.database.GetActivationBundle(ctx, bundleID); getErr == nil {
		return ActivationPreparation{StartupArtifact: startup, Bundle: existing}, nil
	} else if !errors.Is(getErr, store.ErrActivationBundleNotFound) {
		return ActivationPreparation{}, getErr
	}

	createdAt := application.now().UTC()
	bundle := store.ActivationBundle{
		ID: bundleID, StartupArtifactID: startup.ID, MonitoringTier: monitoring, CreatedAt: createdAt,
	}
	stored, err := application.database.SaveActivationBundle(ctx, bundle)
	if err != nil {
		return ActivationPreparation{}, err
	}
	return ActivationPreparation{StartupArtifact: startup, Bundle: stored}, nil
}

// verifyActivationCandidate rechecks every mutable eligibility decision at
// bundle-preparation time. Old bundles remain immutable, while a stale head,
// changed compiled adapter, or revoked binary cannot produce a new bundle.
func (application *Application) verifyActivationCandidate(
	ctx context.Context,
	startup store.StartupArtifact,
) error {
	head, err := application.database.Head(ctx)
	if err != nil {
		return err
	}
	if head == nil || head.ID != startup.CanonicalRevisionID {
		return fmt.Errorf(
			"%w: startup artifact canonical revision is not the current head",
			store.ErrActivationBundleNotReady,
		)
	}
	core, err := application.database.GetCoreArtifact(ctx, startup.CoreArtifactID)
	if err != nil {
		return err
	}
	if core.VerificationState != store.CoreArtifactVerified ||
		core.ExactVersion != startup.ExactCoreVersion ||
		core.ReportedVersion != startup.ExactCoreVersion {
		return fmt.Errorf(
			"%w: exact core artifact is no longer eligible",
			store.ErrActivationBundleNotReady,
		)
	}
	resolved, err := application.configurationAdapters.Resolve(coreArtifactProfile(core))
	if err != nil || resolved.ID() != startup.AdapterID || resolved.Revision() != startup.AdapterRevision {
		return fmt.Errorf(
			"%w: startup artifact does not match the compiled adapter for the exact core profile",
			store.ErrActivationBundleNotReady,
		)
	}
	return nil
}

func (application *Application) QueueRuntimeApply(ctx context.Context, bundleID string) (Task, error) {
	return application.queueRuntimeIntent(ctx, store.RuntimeIntentApply, bundleID)
}

func (application *Application) PrepareAndQueueRuntimeApply(
	ctx context.Context,
	startupArtifactID string,
	monitoring store.MonitoringTier,
) (ActivationPreparation, Task, error) {
	prepared, err := application.PrepareActivationBundle(ctx, startupArtifactID, monitoring)
	if err != nil {
		return ActivationPreparation{}, Task{}, err
	}
	task, err := application.QueueRuntimeApply(ctx, prepared.Bundle.ID)
	if err != nil {
		return ActivationPreparation{}, Task{}, err
	}
	return prepared, task, nil
}

func (application *Application) QueueRuntimeStart(ctx context.Context) (Task, error) {
	return application.queueRuntimeIntent(ctx, store.RuntimeIntentStart, "")
}

func (application *Application) QueueRuntimeStop(ctx context.Context) (Task, error) {
	return application.queueRuntimeIntent(ctx, store.RuntimeIntentStop, "")
}

func (application *Application) QueueRuntimeRestart(ctx context.Context) (Task, error) {
	return application.queueRuntimeIntent(ctx, store.RuntimeIntentRestart, "")
}

func (application *Application) QueueRuntimeRollback(ctx context.Context) (Task, error) {
	return application.queueRuntimeIntent(ctx, store.RuntimeIntentRollback, "")
}

func (application *Application) queueRuntimeIntent(
	ctx context.Context,
	kind store.RuntimeIntentKind,
	bundleID string,
) (Task, error) {
	taskID, err := application.newID("task")
	if err != nil {
		return Task{}, err
	}
	queued, err := application.database.RequestRuntimeIntent(ctx, store.RuntimeIntentInput{
		TaskID: taskID, Kind: kind, BundleID: strings.TrimSpace(bundleID), CreatedAt: application.now().UTC(),
	})
	if err != nil {
		return Task{}, err
	}
	return applicationTask(queued), nil
}

func (application *Application) RuntimeStatus(ctx context.Context) (RuntimeStatus, error) {
	bootstrap, err := application.database.Bootstrap(ctx)
	if err != nil {
		return RuntimeStatus{}, err
	}
	result := RuntimeStatus{
		DesiredRunning: bootstrap.Hub.DesiredRunning, DesiredBundleID: bootstrap.Hub.DesiredBundleID,
		AppliedBundleID: bootstrap.Hub.AppliedBundleID, RollbackBundleID: bootstrap.Hub.RollbackBundleID,
		TargetGeneration: bootstrap.Hub.TargetGeneration, ObservationState: "stopped",
	}
	identity, err := application.runtime.Resolve(ctx)
	switch {
	case err == nil:
		result.ObservationState = "running"
		result.Running = &identity
	case errors.Is(err, ErrNoRunningCore):
	case errors.Is(err, ErrStaleObservation):
		result.ObservationState = "stale"
	case errors.Is(err, ErrInspectionUnavailable):
		result.ObservationState = "inspection_unavailable"
	default:
		return RuntimeStatus{}, err
	}
	return result, nil
}

// LoadRuntimeMaterial resolves every immutable input for a runtime task. It
// never chooses a version or artifact from desired/global state.
func (application *Application) LoadRuntimeMaterial(
	ctx context.Context,
	activationBundleID string,
) (RuntimeMaterial, error) {
	activation, err := application.database.GetActivationBundle(ctx, activationBundleID)
	if err != nil {
		return RuntimeMaterial{}, err
	}
	startup, err := application.database.GetStartupArtifact(ctx, activation.StartupArtifactID)
	if err != nil {
		return RuntimeMaterial{}, err
	}
	if startup.State != store.StartupArtifactReady {
		return RuntimeMaterial{}, store.ErrActivationBundleNotReady
	}
	return application.runtimeMaterial(ctx, activation.ID, activation, startup)
}

// LoadStartupCheckMaterial accepts only a pending immutable candidate. The
// durable checker owns its sole transition to ready or failed.
func (application *Application) LoadStartupCheckMaterial(
	ctx context.Context,
	startupArtifactID string,
) (RuntimeMaterial, error) {
	startup, err := application.database.GetStartupArtifact(ctx, startupArtifactID)
	if err != nil {
		return RuntimeMaterial{}, err
	}
	if startup.State != store.StartupArtifactPending {
		return RuntimeMaterial{}, store.ErrStartupArtifactState
	}
	return application.runtimeMaterial(ctx, startup.ID, store.ActivationBundle{}, startup)
}

func (application *Application) runtimeMaterial(
	ctx context.Context,
	bundleID string,
	activation store.ActivationBundle,
	startup store.StartupArtifact,
) (RuntimeMaterial, error) {
	core, err := application.database.GetCoreArtifact(ctx, startup.CoreArtifactID)
	if err != nil {
		return RuntimeMaterial{}, err
	}
	if core.VerificationState != store.CoreArtifactVerified || core.ExactVersion != startup.ExactCoreVersion ||
		core.ReportedVersion != startup.ExactCoreVersion {
		return RuntimeMaterial{}, store.ErrActivationBundleNotReady
	}
	resolved, err := application.configurationAdapters.Resolve(coreArtifactProfile(core))
	if err != nil || resolved.ID() != startup.AdapterID || resolved.Revision() != startup.AdapterRevision {
		return RuntimeMaterial{}, errors.Join(store.ErrActivationBundleNotReady, configuration.ErrUnsupportedCoreProfile)
	}
	version, err := coreartifact.ParseExactVersion(core.ExactVersion)
	if err != nil {
		return RuntimeMaterial{}, err
	}
	binaryDigest, err := coreartifact.ParseSHA256(core.BinarySHA256)
	if err != nil {
		return RuntimeMaterial{}, err
	}
	configDigest, err := coreartifact.ParseSHA256(startup.ConfigSHA256)
	if err != nil {
		return RuntimeMaterial{}, err
	}
	return RuntimeMaterial{
		Activation: activation, Startup: startup, Core: core,
		Bundle: coreruntime.AppliedBundle{
			ID: bundleID, ArtifactID: core.ID, ExactVersion: version,
			ArtifactDigest: binaryDigest, BinaryPath: core.BinaryPath,
			StartupConfig: append([]byte(nil), startup.ConfigBytes...), StartupConfigDigest: configDigest,
		},
	}, nil
}

func (application *Application) CompleteStartupCheck(
	ctx context.Context,
	startupArtifactID string,
	succeeded bool,
	diagnostics json.RawMessage,
) (store.StartupArtifact, error) {
	return application.database.CompleteStartupArtifactCheck(
		ctx, startupArtifactID, succeeded, diagnostics, application.now().UTC(),
	)
}

func stableRuntimeID(prefix string, parts ...string) string {
	hash := sha256.New()
	for _, part := range append([]string{prefix}, parts...) {
		_, _ = hash.Write([]byte(part))
		_, _ = hash.Write([]byte{0})
	}
	return prefix + "_" + hex.EncodeToString(hash.Sum(nil))
}

func IsActivationBundleNotReady(err error) bool {
	return errors.Is(err, store.ErrActivationBundleNotReady)
}

func IsMonitoringTierUnavailable(err error) bool {
	return errors.Is(err, ErrMonitoringTierUnavailable)
}

func IsNoAppliedBundle(err error) bool {
	return errors.Is(err, store.ErrNoAppliedBundle)
}

func IsNoRollbackBundle(err error) bool {
	return errors.Is(err, store.ErrNoRollbackBundle)
}
