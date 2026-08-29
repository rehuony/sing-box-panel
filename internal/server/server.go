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
	configuration, err := settings.Load(settingsPath)
	if err != nil {
		return fmt.Errorf("load server settings: %w", err)
	}
	if err := prepareDataDirectory(configuration.DataDir); err != nil {
		return err
	}
	runtimeLease, err := acquireRuntimeExecutorLease(configuration.DataDir)
	if err != nil {
		return err
	}
	defer func() {
		runErr = errors.Join(runErr, runtimeLease.Close())
	}()

	database, err := store.Open(ctx, filepath.Join(configuration.DataDir, "panel.db"))
	if err != nil {
		return fmt.Errorf("open panel database: %w", err)
	}
	defer database.Close()

	commands := application.FromStoreWithSettings(database, configuration)
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
		return fmt.Errorf("enforce operational log retention: %w", err)
	}
	if retention.Deleted > 0 {
		recordOperationalLog(commands, application.LogRecordRequest{
			Source: store.LogSourcePanel, Level: store.LogLevelInfo, Code: "logs.retention_enforced",
			Message:  "Expired operational log entries were deleted",
			Metadata: mustLogMetadata(map[string]any{"deleted": retention.Deleted}),
		})
	}
	retentionContext, stopRetention := context.WithCancel(ctx)
	retentionDone := startLogRetention(retentionContext, commands)
	defer func() {
		stopRetention()
		<-retentionDone
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
		return fmt.Errorf("reconcile sing-box runtime: %w", err)
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
	runner, err := newTaskRunner(database, handlers, taskRunnerOptions{WorkerID: workerID()})
	if err != nil {
		_ = listener.Close()
		return fmt.Errorf("construct durable task runner: %w", err)
	}
	if err := runner.Start(ctx); err != nil {
		_ = listener.Close()
		return fmt.Errorf("start durable task runner: %w", err)
	}
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

func builtInTaskHandlers(
	commands *application.Application,
	artifacts application.ArtifactInstaller,
	runtimeControl *runtimeServices,
) map[store.TaskKind]taskHandler {
	runtimeHandler := taskHandlerFunc(runtimeIntentHandler(runtimeControl))
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
