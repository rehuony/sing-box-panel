// SPDX-License-Identifier: GPL-3.0-or-later

// Package server composes persistent state, HTTP, and process lifecycle.
package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"path/filepath"
	"time"

	"github.com/rehuony/sing-box-panel/internal/application"
	"github.com/rehuony/sing-box-panel/internal/artifactstore"
	"github.com/rehuony/sing-box-panel/internal/buildinfo"
	"github.com/rehuony/sing-box-panel/internal/httpapi"
	"github.com/rehuony/sing-box-panel/internal/panelprocess"
	"github.com/rehuony/sing-box-panel/internal/publicip"
	"github.com/rehuony/sing-box-panel/internal/settings"
	"github.com/rehuony/sing-box-panel/internal/store"
)

const (
	readHeaderTimeout = 5 * time.Second
	readTimeout       = 60 * time.Second
	idleTimeout       = 60 * time.Second
	shutdownTimeout   = 10 * time.Second
	maxHeaderBytes    = 1 << 20
)

// Run loads process settings and serves until ctx is canceled. It owns every
// resource it opens and does not return until the HTTP server has stopped.
func Run(ctx context.Context, settingsPath string, build buildinfo.Info, assets fs.FS) (runErr error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	configuration, err := settings.Load(settingsPath)
	if err != nil {
		return fmt.Errorf("load server settings: %w", err)
	}
	if err := prepareDataDirectory(configuration.DataDir); err != nil {
		return err
	}
	runtimeLease, err := panelprocess.AcquireLease(configuration.DataDir)
	if err != nil {
		return err
	}
	settingsPath, err = filepath.Abs(settingsPath)
	if err != nil {
		return errors.Join(err, runtimeLease.Close())
	}
	control, err := panelprocess.Listen(panelprocess.Status{
		Version: build.Version, SettingsPath: settingsPath, DataDir: configuration.DataDir,
	}, cancel)
	if err != nil {
		return errors.Join(err, runtimeLease.Close())
	}
	defer func() {
		control.StopAccepting()
		runErr = errors.Join(runErr, runtimeLease.Close())
		control.Finish(runErr)
	}()
	stopMarking := context.AfterFunc(ctx, control.Stopping)
	defer stopMarking()

	database, err := store.Open(ctx, filepath.Join(configuration.DataDir, "panel.db"))
	if err != nil {
		return startupError(ctx, "open panel database", err)
	}
	defer func() {
		runErr = errors.Join(runErr, database.Close())
	}()

	commands := application.FromStoreWithSettings(database, configuration)
	configuration, err = commands.EffectiveSettings(ctx)
	if err != nil {
		return startupError(ctx, "load persisted panel settings", err)
	}
	commands = application.FromStoreWithSettings(database, configuration)
	commands.SetPublicIPResolver(publicip.New().Resolve)
	defer func() {
		level, code, message := store.LogLevelInfo, "panel.stopped", "Panel server stopped"
		if runErr != nil {
			level, code, message = store.LogLevelError, "panel.failed", "Panel server stopped after an error"
		}
		recordOperationalLog(commands, application.LogRecordRequest{
			Source: store.LogSourcePanel, Level: level, Code: code, Message: message,
			Metadata: mustLogMetadata(map[string]any{"version": build.Version}),
		})
	}()
	retention, err := commands.EnforceLogRetention(ctx)
	if err != nil {
		return startupError(ctx, "enforce operational log retention", err)
	}
	if retention.Deleted > 0 {
		recordOperationalLog(commands, application.LogRecordRequest{
			Source: store.LogSourcePanel, Level: store.LogLevelInfo, Code: "logs.retention_enforced",
			Message:  "Expired operational log entries were deleted",
			Metadata: mustLogMetadata(map[string]any{"deleted": retention.Deleted}),
		})
	}
	trafficRetention, err := commands.EnforceTrafficSampleRetention(ctx)
	if err != nil {
		return startupError(ctx, "enforce traffic sample retention", err)
	}
	if trafficRetention.Deleted > 0 {
		recordOperationalLog(commands, application.LogRecordRequest{
			Source: store.LogSourcePanel, Level: store.LogLevelInfo, Code: "traffic.retention_enforced",
			Message: "Expired raw traffic samples were deleted",
			Metadata: mustLogMetadata(map[string]any{
				"deleted": trafficRetention.Deleted, "cutoff": trafficRetention.Cutoff,
			}),
		})
	}
	uploadGC, uploadGCErr := commands.GarbageCollectCoreUploads(ctx)
	if uploadGCErr != nil {
		recordOperationalLog(commands, application.LogRecordRequest{
			Source: store.LogSourcePanel, Level: store.LogLevelWarn, Code: "core_upload.gc_aborted",
			Message:  "Staged core upload cleanup was skipped because active task ownership was uncertain",
			Metadata: mustLogMetadata(map[string]any{"error": uploadGCErr.Error()}),
		})
	} else if uploadGC.Deleted > 0 {
		recordOperationalLog(commands, application.LogRecordRequest{
			Source: store.LogSourcePanel, Level: store.LogLevelInfo, Code: "core_upload.gc_completed",
			Message:  "Unreferenced staged core uploads were removed",
			Metadata: mustLogMetadata(map[string]any{"deleted": uploadGC.Deleted, "retained": uploadGC.Retained}),
		})
	}
	retentionContext, stopRetention := context.WithCancel(ctx)
	retentionDone := startLogRetention(retentionContext, commands)
	defer func() {
		stopRetention()
		<-retentionDone
	}()
	sampleRetentionContext, stopSampleRetention := context.WithCancel(ctx)
	sampleRetentionDone := startTrafficSampleRetention(sampleRetentionContext, commands)
	defer func() {
		stopSampleRetention()
		<-sampleRetentionDone
	}()
	artifacts, err := artifactstore.New(artifactstore.Options{Root: filepath.Join(configuration.DataDir, "artifacts")})
	if err != nil {
		return fmt.Errorf("open core artifact store: %w", err)
	}
	runtimeControl, err := newRuntimeServices(database, commands, configuration)
	if err != nil {
		return fmt.Errorf("construct sing-box runtime: %w", err)
	}
	defer func() {
		runErr = errors.Join(runErr, runtimeControl.Close())
	}()
	trafficContext, stopTraffic := context.WithCancel(ctx)
	trafficDone := startTrafficSampler(trafficContext, runtimeControl)
	defer func() {
		stopTraffic()
		<-trafficDone
	}()
	if err := runtimeControl.ReconcileStartup(ctx); err != nil {
		return startupError(ctx, "reconcile sing-box runtime", err)
	}
	recordOperationalLog(commands, application.LogRecordRequest{
		Source: store.LogSourcePanel, Level: store.LogLevelInfo, Code: "runtime.reconciled",
		Message: "Runtime state reconciled at panel startup", Metadata: json.RawMessage(`{}`),
	})
	handler := httpapi.NewHandler(httpapi.HandlerOptions{
		Settings: configuration,
		Build:    build,
		Assets:   assets,
		Status:   &statusProvider{database: database, build: build, commands: commands, runtime: runtimeControl},
		Commands: commands,
	})
	listener, err := net.Listen("tcp", net.JoinHostPort(configuration.Server.Host, fmt.Sprint(configuration.Server.Port)))
	if err != nil {
		return fmt.Errorf("listen for panel HTTP server: %w", err)
	}

	httpServer := &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: readHeaderTimeout,
		ReadTimeout:       readTimeout,
		IdleTimeout:       idleTimeout,
		MaxHeaderBytes:    maxHeaderBytes,
	}
	handlers := builtInTaskHandlers(commands, artifacts, runtimeControl)
	for kind, taskHandler := range handlers {
		handlers[kind] = withTaskLogging(commands, taskHandler)
	}
	runner, err := newTaskRunner(database, handlers, taskRunnerOptions{
		WorkerID: workerID(), FinalizeTask: commands.FinalizeTaskResources,
	})
	if err != nil {
		_ = listener.Close()
		return fmt.Errorf("construct durable task runner: %w", err)
	}
	if err := runner.Start(ctx); err != nil {
		_ = listener.Close()
		return startupError(ctx, "start durable task runner", err)
	}
	recoveryContext, stopRecovery := context.WithCancel(ctx)
	recoveryDone := startRuntimeReconciler(recoveryContext, runtimeControl)
	defer func() {
		stopRecovery()
		<-recoveryDone
	}()
	recordOperationalLog(commands, application.LogRecordRequest{
		Source: store.LogSourcePanel, Level: store.LogLevelInfo, Code: "panel.ready",
		Message:  "Panel HTTP server and durable task executor are ready",
		Metadata: mustLogMetadata(map[string]any{"version": build.Version}),
	})
	runnerResult := make(chan error, 1)
	go func() {
		runnerResult <- runner.Wait()
	}()

	serveResult := make(chan error, 1)
	go func() {
		serveResult <- httpServer.Serve(listener)
	}()
	control.Ready(listener.Addr().String())

	select {
	case err := <-serveResult:
		runner.Close()
		runnerErr := <-runnerResult
		if errors.Is(err, http.ErrServerClosed) {
			return runnerErr
		}
		return fmt.Errorf("serve panel HTTP server: %w", err)
	case runnerErr := <-runnerResult:
		shutdownErr := stopHTTPServer(httpServer, serveResult)
		if ctx.Err() != nil {
			return shutdownErr
		}
		if runnerErr != nil {
			return fmt.Errorf("run durable task executor: %w", runnerErr)
		}
		if shutdownErr != nil {
			return shutdownErr
		}
		return errors.New("durable task executor stopped unexpectedly")
	case <-ctx.Done():
		runner.Close()
		shutdownErr := stopHTTPServer(httpServer, serveResult)
		runnerErr := <-runnerResult
		if shutdownErr != nil {
			return shutdownErr
		}
		if runnerErr != nil {
			return fmt.Errorf("stop durable task executor: %w", runnerErr)
		}
		return nil
	}
}

// Cancellation during startup is a requested stop. Normalize it before the
// resource defers run so failures in cleanup are still reported to the caller.
func startupError(ctx context.Context, operation string, err error) error {
	if ctx.Err() != nil && errors.Is(err, context.Canceled) {
		return nil
	}
	return fmt.Errorf("%s: %w", operation, err)
}

func builtInTaskHandlers(
	commands *application.Application,
	artifacts application.ArtifactInstaller,
	runtimeControl *runtimeServices,
) map[store.TaskKind]taskHandler {
	runtimeHandler := runtimeIntentHandler(runtimeControl)
	return map[store.TaskKind]taskHandler{
		store.TaskKindCanonicalSaved:            taskHandlerFunc(acknowledgeCanonicalSave),
		store.TaskKindCatalogRefresh:            taskHandlerFunc(catalogRefreshHandler(commands)),
		store.TaskKindCoreInstall:               taskHandlerFunc(coreArtifactHandler(commands, artifacts)),
		store.TaskKindCoreImport:                taskHandlerFunc(coreArtifactHandler(commands, artifacts)),
		store.TaskKindStartupCheck:              taskHandlerFunc(startupCheckHandler(commands, runtimeControl.manager)),
		store.TaskKindSubscriptionSourceRefresh: taskHandlerFunc(subscriptionSourceRefreshHandler(commands)),
		store.TaskKindRuntimeApply:              runtimeHandler,
		store.TaskKindRuntimeStart:              runtimeHandler,
		store.TaskKindRuntimeStop:               runtimeHandler,
		store.TaskKindRuntimeRestart:            runtimeHandler,
		store.TaskKindRuntimeRollback:           runtimeHandler,
	}
}
