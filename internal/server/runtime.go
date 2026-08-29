// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/rehuony/sing-box-panel/internal/application"
	coreruntime "github.com/rehuony/sing-box-panel/internal/runtime"
	"github.com/rehuony/sing-box-panel/internal/settings"
	"github.com/rehuony/sing-box-panel/internal/store"
)

type runtimeServices struct {
	database *store.Store
	commands *application.Application
	manager  runtimeManager
	identity runtimeIdentityResolver
}

type runtimeManager interface {
	Check(context.Context, coreruntime.AppliedBundle) error
	Start(context.Context, coreruntime.AppliedBundle) error
	Stop(context.Context) error
	Restart(context.Context, coreruntime.AppliedBundle) error
	Close(context.Context) error
	Wait() error
	MonitoringLevel() coreruntime.MonitoringLevel
	ObserveLiveIdentity() coreruntime.LiveIdentity
}

type runtimeIdentityResolver interface {
	Resolve(context.Context) (application.RuntimeIdentity, error)
	ProcessStartToken(context.Context, int) (string, error)
}

func newRuntimeServices(
	database *store.Store,
	commands *application.Application,
	configuration settings.Settings,
) (*runtimeServices, error) {
	manager, err := coreruntime.NewManager(coreruntime.Options{
		RuntimeDir: filepath.Join(configuration.DataDir, "runtime"),
	})
	if err != nil {
		return nil, err
	}
	return &runtimeServices{
		database: database, commands: commands, manager: manager,
		identity: application.NewRuntimeIdentityResolver(database),
	}, nil
}

// ReconcileStartup refuses to adopt a live unowned process and routes startup
// convergence through the same fenced, bounded recovery history used at
// runtime. An active durable intent remains the sole owner of convergence.
func (services *runtimeServices) ReconcileStartup(ctx context.Context) error {
	var expectedObservation *store.RuntimeObservation
	observation, err := services.database.RuntimeObservation(ctx)
	if err == nil {
		if proveErr := services.proveCapturedObservationExited(&observation); proveErr != nil {
			return fmt.Errorf("reconcile runtime observation: %w", proveErr)
		}
		expectedObservation = &observation
	} else if !errors.Is(err, store.ErrRuntimeObservationNotFound) {
		return err
	}

	bootstrap, err := services.database.Bootstrap(ctx)
	if err != nil {
		return err
	}
	if !bootstrap.Hub.DesiredRunning || bootstrap.Hub.AppliedBundleID == "" ||
		bootstrap.Hub.DesiredBundleID != bootstrap.Hub.AppliedBundleID {
		return services.clearCapturedRuntimeObservation(expectedObservation)
	}
	recovery, err := services.commands.RequestRuntimeRecovery(ctx, application.RuntimeRecoveryRequest{
		ExpectedBundleID:    bootstrap.Hub.AppliedBundleID,
		ExpectedGeneration:  bootstrap.Hub.TargetGeneration,
		ExpectedObservation: expectedObservation,
		StableRunProven:     runtimeObservationProvesStableRun(expectedObservation),
		// A successful recovery with no remaining observation is the durable
		// shape left by a clean panel shutdown. Failed/crashed children retain
		// their observation and therefore cannot consume this reset boundary.
		CleanBoundaryProven: expectedObservation == nil,
	})
	if err != nil {
		return err
	}
	if recovery.Task != nil {
		recordRuntimeRecoveryQueued(services.commands, recovery)
	}
	if recovery.Exhausted {
		recordRuntimeRecoveryExhausted(services.commands, recovery)
	}
	return nil
}

func (services *runtimeServices) Close() error {
	if services == nil || services.manager == nil {
		return nil
	}
	observation, observationErr := services.captureRuntimeObservation(context.Background())
	ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	closeErr := services.manager.Close(ctx)
	waitErr := services.manager.Wait()
	if observationErr != nil {
		closeErr = errors.Join(closeErr, observationErr)
	} else if closeErr == nil && waitErr == nil {
		closeErr = errors.Join(closeErr, services.clearCapturedRuntimeObservation(observation))
	} else {
		closeErr = errors.Join(closeErr, services.clearCapturedObservationAfterFailedStop(observation))
	}
	return errors.Join(closeErr, waitErr)
}

func startupCheckHandler(
	commands *application.Application,
	manager runtimeManager,
) taskHandlerFunc {
	return func(ctx context.Context, task store.Task, control taskExecutionControl) (json.RawMessage, error) {
		if err := control.SafePoint(ctx); err != nil {
			return nil, err
		}
		material, err := commands.LoadStartupCheckMaterial(ctx, task.StartupArtifactID)
		if err != nil {
			return nil, err
		}
		checkErr := manager.Check(ctx, material.Bundle)
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if err := control.SafePoint(ctx); err != nil {
			return nil, err
		}
		succeeded := checkErr == nil
		diagnostics := startupCheckDiagnostics(checkErr)
		completed, completeErr := commands.CompleteStartupCheck(
			ctx, task.StartupArtifactID, succeeded, diagnostics,
		)
		if completeErr != nil {
			return nil, errors.Join(checkErr, completeErr)
		}
		result, marshalErr := json.Marshal(map[string]any{
			"startup_artifact_id": completed.ID,
			"state":               completed.State,
			"config_sha256":       completed.ConfigSHA256,
		})
		if checkErr != nil {
			return result, checkErr
		}
		return result, marshalErr
	}
}

func startupCheckDiagnostics(err error) json.RawMessage {
	code := "sing_box_check_passed"
	if err != nil {
		code = "sing_box_check_failed"
		switch {
		case errors.Is(err, coreruntime.ErrArtifactDigest):
			code = "binary_digest_mismatch"
		case errors.Is(err, coreruntime.ErrStartupConfigDigest):
			code = "config_digest_mismatch"
		case errors.Is(err, coreruntime.ErrVersionMismatch):
			code = "exact_version_mismatch"
		case errors.Is(err, coreruntime.ErrCheckFailed):
			code = "sing_box_rejected_config"
		}
	}
	encoded, _ := json.Marshal([]map[string]string{{"code": code}})
	return encoded
}

func runtimeIntentHandler(services *runtimeServices) taskHandlerFunc {
	return func(ctx context.Context, task store.Task, control taskExecutionControl) (json.RawMessage, error) {
		if err := control.SafePoint(ctx); err != nil {
			return nil, err
		}
		if store.RuntimeIntentKind(task.Kind) == store.RuntimeIntentStop {
			return services.stopForTask(ctx, control)
		}
		material, err := services.commands.LoadRuntimeMaterial(ctx, task.ActivationBundleID)
		if err != nil {
			return nil, err
		}
		processMonitoringTier := store.MonitoringTier(services.manager.MonitoringLevel())
		if processMonitoringTier != store.MonitoringProcessOnly ||
			(material.Activation.MonitoringTier != store.MonitoringProcessOnly &&
				material.Activation.MonitoringTier != store.MonitoringLimited) {
			return nil, fmt.Errorf(
				"activation monitoring tier %q is unavailable; process probe supplies %q",
				material.Activation.MonitoringTier,
				processMonitoringTier,
			)
		}
		if err := services.revalidateRuntimeMaterial(ctx, material); err != nil {
			return nil, err
		}
		capturedObservation, err := services.captureRuntimeObservation(ctx)
		if err != nil {
			return nil, err
		}
		var recordedObservation *store.RuntimeObservation
		live := services.manager.ObserveLiveIdentity()
		alreadyExact := live.Running && live.BundleID == material.Bundle.ID &&
			live.ArtifactID == material.Bundle.ArtifactID && live.ExactVersion == material.Bundle.ExactVersion &&
			live.ArtifactDigest == material.Bundle.ArtifactDigest
		startedByTask := false
		if runtimeIntentNeedsTransition(store.RuntimeIntentKind(task.Kind), alreadyExact) {
			if live.Running {
				err = services.manager.Restart(ctx, material.Bundle)
			} else {
				err = services.manager.Start(ctx, material.Bundle)
			}
			if err != nil {
				return nil, errors.Join(err, services.clearCapturedObservationAfterFailedStop(capturedObservation))
			}
			startedByTask = true
			observation, recordErr := services.recordLiveObservation(ctx, material)
			if recordErr != nil {
				return nil, errors.Join(recordErr, services.stopAfterLostIntent(capturedObservation))
			}
			recordedObservation = &observation
			if err := services.revalidateRuntimeMaterial(ctx, material); err != nil {
				return nil, errors.Join(err, services.stopAfterLostIntent(recordedObservation))
			}
		}
		if material.Activation.MonitoringTier == store.MonitoringLimited {
			if err := services.awaitClashAPI(ctx, material); err != nil {
				if startedByTask {
					err = errors.Join(err, services.stopAfterLostIntent(recordedObservation))
				}
				return nil, err
			}
		}
		if err := control.SafePoint(ctx); err != nil {
			if startedByTask {
				err = errors.Join(err, services.stopAfterLostIntent(recordedObservation))
			}
			return nil, err
		}
		if recordedObservation == nil {
			observation, recordErr := services.recordLiveObservation(ctx, material)
			if recordErr != nil {
				if startedByTask {
					recordErr = errors.Join(recordErr, services.stopAfterLostIntent(recordedObservation))
				}
				return nil, recordErr
			}
			recordedObservation = &observation
		}
		if err := control.SafePoint(ctx); err != nil {
			if startedByTask {
				err = errors.Join(err, services.stopAfterLostIntent(recordedObservation))
			}
			return nil, err
		}
		return json.Marshal(map[string]any{
			"healthy": true, "monitoring_tier": material.Activation.MonitoringTier,
			"runtime": *recordedObservation,
		})
	}
}

func (services *runtimeServices) awaitClashAPI(ctx context.Context, material application.RuntimeMaterial) error {
	endpoint, err := coreruntime.ParseClashEndpoint(material.Bundle.StartupConfig)
	if err != nil {
		return err
	}
	client, err := coreruntime.NewClashClient(endpoint)
	if err != nil {
		return err
	}
	handshakeContext, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	version, err := client.Version(handshakeContext)
	if err != nil {
		return fmt.Errorf("limited monitoring handshake: %w", err)
	}
	if version != material.Core.ExactVersion {
		return fmt.Errorf("limited monitoring handshake: core version %q does not match %q", version, material.Core.ExactVersion)
	}
	return nil
}

func runtimeIntentNeedsTransition(kind store.RuntimeIntentKind, alreadyExact bool) bool {
	return kind == store.RuntimeIntentRestart || !alreadyExact
}

func (services *runtimeServices) revalidateRuntimeMaterial(
	ctx context.Context,
	material application.RuntimeMaterial,
) error {
	current, err := services.commands.LoadRuntimeMaterial(ctx, material.Activation.ID)
	if err != nil {
		return err
	}
	if current.Startup.ID != material.Startup.ID ||
		current.Core.ID != material.Core.ID ||
		current.Core.VerificationState != store.CoreArtifactVerified ||
		current.Bundle.ExactVersion != material.Bundle.ExactVersion ||
		current.Bundle.ArtifactDigest != material.Bundle.ArtifactDigest ||
		current.Bundle.StartupConfigDigest != material.Bundle.StartupConfigDigest {
		return store.ErrActivationBundleNotReady
	}
	return nil
}

func (services *runtimeServices) stopForTask(
	ctx context.Context,
	control taskExecutionControl,
) (json.RawMessage, error) {
	observation, err := services.captureRuntimeObservation(ctx)
	if err != nil {
		return nil, err
	}
	if err := services.manager.Stop(ctx); err != nil {
		return nil, errors.Join(err, services.clearCapturedObservationAfterFailedStop(observation))
	}
	if err := services.clearCapturedRuntimeObservation(observation); err != nil {
		return nil, err
	}
	if err := control.SafePoint(ctx); err != nil {
		return nil, err
	}
	return json.RawMessage(`{"running":false}`), nil
}

func (services *runtimeServices) recordLiveObservation(
	ctx context.Context,
	material application.RuntimeMaterial,
) (store.RuntimeObservation, error) {
	live := services.manager.ObserveLiveIdentity()
	if !live.Running || live.PID <= 0 {
		return store.RuntimeObservation{}, coreruntime.ErrNotRunning
	}
	startToken, err := services.identity.ProcessStartToken(ctx, live.PID)
	if err != nil {
		return store.RuntimeObservation{}, err
	}
	return services.database.RecordRuntimeObservation(ctx, store.RuntimeObservation{
		PID: live.PID, ProcessStartToken: startToken, CoreArtifactID: material.Core.ID,
		ActivationBundleID: material.Activation.ID, ExactCoreVersion: material.Core.ExactVersion,
		ArchiveSHA256: material.Core.ArchiveSHA256, BinarySHA256: material.Core.BinarySHA256,
		StartedAt: live.StartedAt, ObservedAt: time.Now().UTC(),
	})
}

func (services *runtimeServices) stopAfterLostIntent(observation *store.RuntimeObservation) error {
	ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	if err := services.manager.Stop(ctx); err != nil {
		return errors.Join(err, services.clearCapturedObservationAfterFailedStop(observation))
	}
	return services.clearCapturedRuntimeObservation(observation)
}

func (services *runtimeServices) captureRuntimeObservation(ctx context.Context) (*store.RuntimeObservation, error) {
	observation, err := services.database.RuntimeObservation(ctx)
	if errors.Is(err, store.ErrRuntimeObservationNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &observation, nil
}

func (services *runtimeServices) clearCapturedRuntimeObservation(observation *store.RuntimeObservation) error {
	if observation == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, err := services.database.ClearRuntimeObservation(
		ctx, observation.PID, observation.ProcessStartToken,
	)
	return err
}

func (services *runtimeServices) clearCapturedObservationAfterFailedStop(
	observation *store.RuntimeObservation,
) error {
	if observation == nil {
		return nil
	}
	if err := services.proveCapturedObservationExited(observation); err != nil {
		return err
	}
	return services.clearCapturedRuntimeObservation(observation)
}

func (services *runtimeServices) proveCapturedObservationExited(observation *store.RuntimeObservation) error {
	if observation == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	startToken, err := services.identity.ProcessStartToken(ctx, observation.PID)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("prove stopped runtime process incarnation: %w", err)
	}
	if startToken == observation.ProcessStartToken {
		return fmt.Errorf("runtime process %d still has the captured process start token", observation.PID)
	}
	return nil
}

func startTrafficSampler(ctx context.Context, services *runtimeServices) <-chan struct{} {
	done := make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				services.collectTrafficSample(ctx)
			}
		}
	}()
	return done
}

func (services *runtimeServices) collectTrafficSample(ctx context.Context) {
	observation, err := services.database.RuntimeObservation(ctx)
	if err != nil {
		return
	}
	bundle, err := services.database.GetActivationBundle(ctx, observation.ActivationBundleID)
	if err != nil || bundle.MonitoringTier != store.MonitoringLimited {
		return
	}
	sampleContext, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	result, err := services.commands.CollectLimitedTrafficSample(sampleContext, observation)
	if err != nil {
		recordOperationalLog(services.commands, application.LogRecordRequest{
			Source: store.LogSourceCore, Level: store.LogLevelWarn, Code: "traffic.sample_failed",
			Message: "Clash API traffic sample failed", Metadata: json.RawMessage(`{}`),
		})
		return
	}
	if !result.Sample.Accepted {
		recordOperationalLog(services.commands, application.LogRecordRequest{
			Source: store.LogSourceCore, Level: store.LogLevelWarn, Code: "traffic.counter_decreased",
			Message:  "Clash API counters decreased within one process; sample rejected",
			Metadata: json.RawMessage(`{}`),
		})
	}
}
