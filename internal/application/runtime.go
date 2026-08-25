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

	"github.com/rehuony/sing-box-panel/internal/capability"
	"github.com/rehuony/sing-box-panel/internal/coreartifact"
	coreruntime "github.com/rehuony/sing-box-panel/internal/runtime"
	"github.com/rehuony/sing-box-panel/internal/runtimeidentity"
	"github.com/rehuony/sing-box-panel/internal/store"
)

var ErrMonitoringTierUnavailable = errors.New("requested monitoring tier is unavailable")

type RuntimeStatus struct {
	DesiredRunning   bool                      `json:"desired_running"`
	DesiredBundleID  string                    `json:"desired_bundle_id,omitempty"`
	AppliedBundleID  string                    `json:"applied_bundle_id,omitempty"`
	RollbackBundleID string                    `json:"rollback_bundle_id,omitempty"`
	TargetGeneration int64                     `json:"target_generation"`
	ObservationState string                    `json:"observation_state"`
	Running          *runtimeidentity.Identity `json:"running,omitempty"`
}

type ActivationPreparation struct {
	StartupArtifact store.StartupArtifact      `json:"startup_artifact"`
	Snapshot        store.SubscriptionSnapshot `json:"subscription_snapshot"`
	Bundle          store.ActivationBundle     `json:"bundle"`
}

// ActivationSummary is safe for management transports. It deliberately omits
// startup bytes, rendered subscription content, source credentials, and
// discovered addresses while retaining every immutable identity and digest an
// operator needs to audit an apply.
type ActivationSummary struct {
	StartupArtifactID    string               `json:"startup_artifact_id"`
	CanonicalRevisionID  string               `json:"canonical_revision_id"`
	ExactCoreVersion     string               `json:"exact_core_version"`
	CoreArtifactID       string               `json:"core_artifact_id"`
	ConfigSHA256         string               `json:"config_sha256"`
	SubscriptionSnapshot string               `json:"subscription_snapshot_id"`
	SubscriptionSHA256   string               `json:"subscription_sha256"`
	ActivationBundleID   string               `json:"activation_bundle_id"`
	ActivationSHA256     string               `json:"activation_sha256"`
	MonitoringTier       store.MonitoringTier `json:"monitoring_tier"`
}

func (preparation ActivationPreparation) Summary() ActivationSummary {
	return ActivationSummary{
		StartupArtifactID:    preparation.StartupArtifact.ID,
		CanonicalRevisionID:  preparation.StartupArtifact.CanonicalRevisionID,
		ExactCoreVersion:     preparation.StartupArtifact.ExactCoreVersion,
		CoreArtifactID:       preparation.StartupArtifact.CoreArtifactID,
		ConfigSHA256:         preparation.StartupArtifact.ConfigSHA256,
		SubscriptionSnapshot: preparation.Snapshot.ID,
		SubscriptionSHA256:   preparation.Snapshot.SHA256,
		ActivationBundleID:   preparation.Bundle.ID,
		ActivationSHA256:     preparation.Bundle.SHA256,
		MonitoringTier:       preparation.Bundle.MonitoringTier,
	}
}

type RuntimeMaterial struct {
	Bundle     coreruntime.AppliedBundle
	Activation store.ActivationBundle
	Startup    store.StartupArtifact
	Core       store.CoreArtifact
}

// PrepareActivationBundle freezes the already-checked startup bytes and every
// enabled publication input before entering the short atomic save. Rendering
// and source snapshot collection never occur inside the database transaction.
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
	if monitoring != store.MonitoringProcessOnly {
		return ActivationPreparation{}, fmt.Errorf("%w: only process_only has an active health probe", ErrMonitoringTierUnavailable)
	}
	if err := application.verifyActivationCandidate(ctx, startup); err != nil {
		return ActivationPreparation{}, err
	}

	content, sourceSnapshots, err := application.prepareSubscriptionFreeze(ctx, startup)
	if err != nil {
		return ActivationPreparation{}, err
	}
	publicAddresses := json.RawMessage(`{}`)
	snapshotID := stableRuntimeID("snapshot", startup.ID, string(content))
	bundleID := stableRuntimeID(
		"bundle",
		startup.ID,
		snapshotID,
		string(publicAddresses),
		string(sourceSnapshots),
		string(monitoring),
	)
	if existing, getErr := application.database.GetActivationBundle(ctx, bundleID); getErr == nil {
		snapshot, snapshotErr := application.database.GetSubscriptionSnapshot(ctx, existing.SubscriptionSnapshotID)
		if snapshotErr != nil {
			return ActivationPreparation{}, snapshotErr
		}
		return ActivationPreparation{StartupArtifact: startup, Snapshot: snapshot, Bundle: existing}, nil
	} else if !errors.Is(getErr, store.ErrActivationBundleNotFound) {
		return ActivationPreparation{}, getErr
	}

	createdAt := application.now().UTC()
	snapshot := store.SubscriptionSnapshot{
		ID: snapshotID, CanonicalRevisionID: startup.CanonicalRevisionID,
		StartupArtifactID: startup.ID, Content: content, CreatedAt: createdAt,
	}
	bundle := store.ActivationBundle{
		ID: bundleID, StartupArtifactID: startup.ID, SubscriptionSnapshotID: snapshot.ID,
		PublicAddresses: publicAddresses, SourceSnapshots: sourceSnapshots,
		MonitoringTier: monitoring, CreatedAt: createdAt,
	}
	stored, err := application.database.SaveActivationBundle(ctx, snapshot, bundle)
	if err != nil {
		return ActivationPreparation{}, err
	}
	storedSnapshot, err := application.database.GetSubscriptionSnapshot(ctx, stored.SubscriptionSnapshotID)
	if err != nil {
		return ActivationPreparation{}, err
	}
	return ActivationPreparation{StartupArtifact: startup, Snapshot: storedSnapshot, Bundle: stored}, nil
}

// verifyActivationCandidate rechecks every mutable eligibility decision at
// bundle-preparation time. Old bundles remain immutable and usable for their
// dedicated start/rollback paths, but a stale head, moved capability pin, or
// revoked binary can never produce a new bundle.
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
	if startup.Kind != store.StartupArtifactStructured {
		return nil
	}
	manifest, pin, err := application.PinnedCapabilityManifest(ctx, startup.ExactCoreVersion)
	if err != nil {
		return fmt.Errorf("%w: pinned capability is unavailable: %v", store.ErrActivationBundleNotReady, err)
	}
	if (manifest.SupportLevel() != capability.SupportNativeStructured &&
		manifest.SupportLevel() != capability.SupportCompatibleStructured) ||
		pin.CommitSHA != startup.CapabilityCommit ||
		pin.ManifestSHA256 != startup.CapabilityDigest {
		return fmt.Errorf(
			"%w: structured startup artifact does not match the active immutable capability pin",
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
	case errors.Is(err, runtimeidentity.ErrNoRunningCore):
	case errors.Is(err, runtimeidentity.ErrStaleObservation):
		result.ObservationState = "stale"
	case errors.Is(err, runtimeidentity.ErrInspectionUnavailable):
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
	if startup.State != store.StartupArtifactReady && startup.State != store.StartupArtifactStale {
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
