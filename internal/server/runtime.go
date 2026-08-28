// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"time"

	"github.com/rehuony/sing-box-panel/internal/application"
	"github.com/rehuony/sing-box-panel/internal/clashapi"
	coreruntime "github.com/rehuony/sing-box-panel/internal/runtime"
	"github.com/rehuony/sing-box-panel/internal/runtimeidentity"
	"github.com/rehuony/sing-box-panel/internal/settings"
	"github.com/rehuony/sing-box-panel/internal/store"
	"github.com/rehuony/sing-box-panel/internal/taskrunner"
)

type runtimeServices struct {
	database *store.Store
	commands *application.Application
	manager  *coreruntime.Manager
	identity *runtimeidentity.Resolver
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
		identity: runtimeidentity.New(database),
	}, nil
}

// ReconcileStartup clears only a proven-stale observation and restarts the
// last applied bundle when durable desired state says it should be running.
// A genuinely live, unowned process is never adopted implicitly.
func (services *runtimeServices) ReconcileStartup(ctx context.Context) error {
	observation, err := services.database.RuntimeObservation(ctx)
	if err == nil {
		if identity, resolveErr := services.identity.Resolve(ctx); resolveErr == nil {
			return fmt.Errorf(
				"refuse to adopt live sing-box process %d for bundle %s",
				identity.PID,
				identity.ActivationBundleID,
			)
		}
		if _, clearErr := services.database.ClearRuntimeObservation(
			ctx, observation.PID, observation.ProcessStartToken,
		); clearErr != nil {
			return clearErr
		}
	} else if !errors.Is(err, store.ErrRuntimeObservationNotFound) {
		return err
	}

	bootstrap, err := services.database.Bootstrap(ctx)
	if err != nil {
		return err
	}
	if !bootstrap.Hub.DesiredRunning || bootstrap.Hub.AppliedBundleID == "" ||
		bootstrap.Hub.DesiredBundleID != bootstrap.Hub.AppliedBundleID {
		return nil
	}
	for _, status := range []store.TaskStatus{store.TaskStatusQueued, store.TaskStatusRunning} {
		page, listErr := services.database.ListTasks(ctx, store.TaskListFilter{
			Lane: store.TaskLaneRuntime, Status: status, Limit: 1,
		})
		if listErr != nil {
			return listErr
		}
		if len(page.Items) != 0 {
			return nil
		}
	}
	_, err = services.commands.QueueRuntimeStart(ctx)
	return err
}

func (services *runtimeServices) Close() error {
	if services == nil || services.manager == nil {
		return nil
	}
	observation, observationErr := services.database.RuntimeObservation(context.Background())
	ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	closeErr := services.manager.Close(ctx)
	if observationErr == nil {
		_, clearErr := services.database.ClearRuntimeObservation(
			context.Background(), observation.PID, observation.ProcessStartToken,
		)
		closeErr = errors.Join(closeErr, clearErr)
	} else if !errors.Is(observationErr, store.ErrRuntimeObservationNotFound) {
		closeErr = errors.Join(closeErr, observationErr)
	}
	return errors.Join(closeErr, services.manager.Wait())
}

func startupCheckHandler(
	commands *application.Application,
	manager *coreruntime.Manager,
) taskrunner.HandlerFunc {
	return func(ctx context.Context, task store.Task, control taskrunner.Control) (json.RawMessage, error) {
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

func runtimeIntentHandler(services *runtimeServices) taskrunner.HandlerFunc {
	return func(ctx context.Context, task store.Task, control taskrunner.Control) (json.RawMessage, error) {
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
				services.clearRecordedObservation()
				return nil, err
			}
			startedByTask = true
			if err := services.revalidateRuntimeMaterial(ctx, material); err != nil {
				services.stopAfterLostIntent()
				return nil, err
			}
		}
		if material.Activation.MonitoringTier == store.MonitoringLimited {
			if err := services.awaitClashAPI(ctx, material); err != nil {
				if startedByTask {
					services.stopAfterLostIntent()
				}
				return nil, err
			}
		}
		if err := control.SafePoint(ctx); err != nil {
			if startedByTask {
				services.stopAfterLostIntent()
			}
			return nil, err
		}
		observation, err := services.recordLiveObservation(ctx, material)
		if err != nil {
			if startedByTask {
				services.stopAfterLostIntent()
			}
			return nil, err
		}
		if err := control.SafePoint(ctx); err != nil {
			if startedByTask {
				services.stopAfterLostIntent()
			}
			return nil, err
		}
		return json.Marshal(map[string]any{
			"healthy": true, "monitoring_tier": material.Activation.MonitoringTier,
			"runtime": observation,
		})
	}
}

func (services *runtimeServices) awaitClashAPI(ctx context.Context, material application.RuntimeMaterial) error {
	endpoint, err := clashapi.ParseEndpoint(material.Bundle.StartupConfig)
	if err != nil {
		return err
	}
	client, err := clashapi.New(endpoint)
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
	control taskrunner.Control,
) (json.RawMessage, error) {
	observation, observationErr := services.database.RuntimeObservation(ctx)
	if err := services.manager.Stop(ctx); err != nil {
		return nil, err
	}
	if observationErr == nil {
		if _, err := services.database.ClearRuntimeObservation(
			ctx, observation.PID, observation.ProcessStartToken,
		); err != nil {
			return nil, err
		}
	} else if !errors.Is(observationErr, store.ErrRuntimeObservationNotFound) {
		return nil, observationErr
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

func (services *runtimeServices) stopAfterLostIntent() {
	ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	_ = services.manager.Stop(ctx)
	services.clearRecordedObservation()
}

func (services *runtimeServices) clearRecordedObservation() {
	observation, err := services.database.RuntimeObservation(context.Background())
	if err != nil {
		return
	}
	_, _ = services.database.ClearRuntimeObservation(
		context.Background(), observation.PID, observation.ProcessStartToken,
	)
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
