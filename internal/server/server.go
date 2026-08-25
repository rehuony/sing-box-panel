// SPDX-License-Identifier: GPL-3.0-or-later

// Package server composes persistent state, the HTTP boundary, and process
// lifecycle for the `server run` command.
package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/rehuony/sing-box-panel/internal/application"
	"github.com/rehuony/sing-box-panel/internal/artifactstore"
	"github.com/rehuony/sing-box-panel/internal/buildinfo"
	"github.com/rehuony/sing-box-panel/internal/httpapi"
	"github.com/rehuony/sing-box-panel/internal/settings"
	"github.com/rehuony/sing-box-panel/internal/store"
	"github.com/rehuony/sing-box-panel/internal/taskrunner"
)

const (
	readHeaderTimeout = 5 * time.Second
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
		IdleTimeout:       idleTimeout,
		MaxHeaderBytes:    maxHeaderBytes,
	}
	runtimeHandler := taskrunner.HandlerFunc(runtimeIntentHandler(runtimeControl))
	handlers := map[string]taskrunner.Handler{
		"canonical-saved":                   taskrunner.HandlerFunc(acknowledgeCanonicalSave),
		"catalog-refresh":                   taskrunner.HandlerFunc(catalogRefreshHandler(commands)),
		"core-install":                      taskrunner.HandlerFunc(coreArtifactHandler(commands, artifacts)),
		"core-import":                       taskrunner.HandlerFunc(coreArtifactHandler(commands, artifacts)),
		"startup-check":                     taskrunner.HandlerFunc(startupCheckHandler(commands, runtimeControl.manager)),
		string(store.RuntimeIntentApply):    runtimeHandler,
		string(store.RuntimeIntentStart):    runtimeHandler,
		string(store.RuntimeIntentStop):     runtimeHandler,
		string(store.RuntimeIntentRestart):  runtimeHandler,
		string(store.RuntimeIntentRollback): runtimeHandler,
	}
	for kind, taskHandler := range handlers {
		handlers[kind] = withTaskLogging(commands, taskHandler)
	}
	runner, err := taskrunner.New(database, handlers, taskrunner.Options{WorkerID: workerID()})
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

func startLogRetention(ctx context.Context, commands *application.Application) <-chan struct{} {
	done := make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(24 * time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				retention, err := commands.EnforceLogRetention(ctx)
				if err != nil {
					recordOperationalLog(commands, application.LogRecordRequest{
						Source: store.LogSourcePanel, Level: store.LogLevelError, Code: "logs.retention_failed",
						Message: "Operational log retention failed", Metadata: json.RawMessage(`{}`),
					})
					continue
				}
				if retention.Deleted > 0 {
					recordOperationalLog(commands, application.LogRecordRequest{
						Source: store.LogSourcePanel, Level: store.LogLevelInfo, Code: "logs.retention_enforced",
						Message:  "Expired operational log entries were deleted",
						Metadata: mustLogMetadata(map[string]any{"deleted": retention.Deleted}),
					})
				}
			}
		}
	}()
	return done
}

func withTaskLogging(commands *application.Application, next taskrunner.Handler) taskrunner.Handler {
	return taskrunner.HandlerFunc(func(
		ctx context.Context,
		task store.Task,
		control taskrunner.Control,
	) (json.RawMessage, error) {
		metadata := mustLogMetadata(map[string]any{
			"task_id": task.ID,
			"kind":    task.Kind,
			"lane":    task.Lane,
			"attempt": task.Attempt,
		})
		recordOperationalLog(commands, application.LogRecordRequest{
			Source: store.LogSourceTask, Level: store.LogLevelInfo, Code: "task.started",
			Message: "Durable task execution started", Metadata: metadata,
		})
		result, err := next.Handle(ctx, task, control)
		if err != nil {
			recordOperationalLog(commands, application.LogRecordRequest{
				Source: store.LogSourceTask, Level: store.LogLevelError, Code: "task.failed",
				Message: "Durable task execution failed", Metadata: metadata,
			})
			return result, err
		}
		recordOperationalLog(commands, application.LogRecordRequest{
			Source: store.LogSourceTask, Level: store.LogLevelInfo, Code: "task.succeeded",
			Message: "Durable task execution succeeded", Metadata: metadata,
		})
		return result, nil
	})
}

func recordOperationalLog(commands *application.Application, request application.LogRecordRequest) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, _ = commands.RecordLog(ctx, request)
}

func mustLogMetadata(value map[string]any) json.RawMessage {
	encoded, err := json.Marshal(value)
	if err != nil {
		return json.RawMessage(`{}`)
	}
	return encoded
}

func stopHTTPServer(server *http.Server, serveResult <-chan error) error {
	shutdownContext, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	shutdownErr := server.Shutdown(shutdownContext)
	if shutdownErr != nil {
		_ = server.Close()
	}
	serveErr := <-serveResult
	if shutdownErr != nil {
		return fmt.Errorf("shut down panel HTTP server: %w", shutdownErr)
	}
	if serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
		return fmt.Errorf("serve panel HTTP server during shutdown: %w", serveErr)
	}
	return nil
}

func workerID() string {
	hostname, err := os.Hostname()
	if err != nil || hostname == "" {
		hostname = "unknown-host"
	}
	return fmt.Sprintf("panel/%s/%d", hostname, os.Getpid())
}

func acknowledgeCanonicalSave(
	ctx context.Context,
	task store.Task,
	control taskrunner.Control,
) (json.RawMessage, error) {
	if err := control.SafePoint(ctx); err != nil {
		return nil, err
	}
	return json.Marshal(map[string]string{"revision_id": task.CanonicalRevisionID})
}

func catalogRefreshHandler(commands *application.Application) taskrunner.HandlerFunc {
	return func(ctx context.Context, _ store.Task, control taskrunner.Control) (json.RawMessage, error) {
		if err := control.SafePoint(ctx); err != nil {
			return nil, err
		}
		result, err := commands.RefreshCatalog(ctx)
		if err != nil {
			return nil, err
		}
		return json.Marshal(result)
	}
}

func coreArtifactHandler(
	commands *application.Application,
	artifacts application.ArtifactInstaller,
) taskrunner.HandlerFunc {
	return func(ctx context.Context, task store.Task, control taskrunner.Control) (json.RawMessage, error) {
		if err := control.SafePoint(ctx); err != nil {
			return nil, err
		}
		result, err := commands.ExecuteCoreArtifactTask(
			ctx,
			task.Kind,
			task.Payload,
			artifacts,
			control.SafePoint,
		)
		if err != nil {
			return nil, err
		}
		return json.Marshal(result)
	}
}

func prepareDataDirectory(path string) error {
	if err := os.MkdirAll(path, 0o700); err != nil {
		return fmt.Errorf("create panel data directory: %w", err)
	}
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("inspect panel data directory: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return fmt.Errorf("panel data path is not a physical directory: %s", path)
	}
	if err := os.Chmod(path, 0o700); err != nil {
		return fmt.Errorf("secure panel data directory: %w", err)
	}
	return nil
}

type statusProvider struct {
	database *store.Store
	build    buildinfo.Info
	commands *application.Application
	runtime  *runtimeServices
}

func (provider *statusProvider) SystemStatus(ctx context.Context) (httpapi.SystemStatus, error) {
	bootstrap, err := provider.database.Bootstrap(ctx)
	if err != nil {
		return httpapi.SystemStatus{}, err
	}
	status := httpapi.SystemStatus{
		PanelVersion:    provider.build.Version,
		AppliedBundleID: stringPointer(bootstrap.Hub.AppliedBundleID),
		Running:         false,
		CapabilityState: "unresolved",
	}
	capabilityVersion := ""
	if provider.runtime != nil {
		live := provider.runtime.manager.ObserveLiveIdentity()
		status.Running = live.Running
		if live.Running {
			capabilityVersion = live.ExactVersion.String()
			status.RunningVersion = stringPointer(capabilityVersion)
			status.RunningArtifact = stringPointer(live.ArtifactID)
		}
	}
	if capabilityVersion == "" && bootstrap.Hub.AppliedBundleID != "" {
		bundle, bundleErr := provider.database.GetActivationBundle(ctx, bootstrap.Hub.AppliedBundleID)
		if bundleErr != nil {
			return httpapi.SystemStatus{}, bundleErr
		}
		startup, startupErr := provider.database.GetStartupArtifact(ctx, bundle.StartupArtifactID)
		if startupErr != nil {
			return httpapi.SystemStatus{}, startupErr
		}
		capabilityVersion = startup.ExactCoreVersion
	}
	if capabilityVersion != "" && provider.commands != nil {
		capabilityStatus, capabilityErr := provider.commands.CoreCapabilityStatus(ctx, capabilityVersion)
		if capabilityErr != nil {
			return httpapi.SystemStatus{}, capabilityErr
		}
		status.CapabilityState = string(capabilityStatus.SupportLevel)
		if capabilityStatus.Quarantined {
			status.CapabilityState = "quarantined_manual_json"
		}
	}
	if bootstrap.Head != nil {
		status.CanonicalRevision = bootstrap.Head.Sequence
	}
	return status, nil
}

func (provider *statusProvider) DashboardContext(ctx context.Context) (httpapi.DashboardContext, error) {
	bootstrap, err := provider.database.Bootstrap(ctx)
	if err != nil {
		return httpapi.DashboardContext{}, err
	}
	warning := "Install or import an exact sing-box artifact before preparing an activation bundle."
	result := httpapi.DashboardContext{
		View: httpapi.DashboardView{ExactVersion: "Not selected"},
		Canonical: httpapi.DashboardCanonical{
			Revision:            0,
			SavedAt:             bootstrap.Hub.UpdatedAt,
			HasUnappliedChanges: false,
		},
		Capability: httpapi.DashboardCapability{
			Level:   "unavailable",
			Label:   "No core selected",
			Warning: &warning,
		},
	}
	if provider.runtime != nil {
		live := provider.runtime.manager.ObserveLiveIdentity()
		if live.Running {
			result.View.ExactVersion = live.ExactVersion.String()
			result.Running = &httpapi.DashboardRuntime{
				ExactVersion: live.ExactVersion.String(),
				ArtifactName: live.ArtifactID,
				Digest:       live.ArtifactDigest.String(),
			}
		}
	}
	if bootstrap.Head != nil {
		result.Canonical.Revision = bootstrap.Head.Sequence
		result.Canonical.SavedAt = bootstrap.Head.CreatedAt
		result.Canonical.HasUnappliedChanges = bootstrap.Hub.AppliedBundleID == ""
	}
	if bootstrap.Hub.AppliedBundleID != "" {
		bundle, err := provider.database.GetActivationBundle(ctx, bootstrap.Hub.AppliedBundleID)
		if err != nil {
			return httpapi.DashboardContext{}, err
		}
		startup, err := provider.database.GetStartupArtifact(ctx, bundle.StartupArtifactID)
		if err != nil {
			return httpapi.DashboardContext{}, err
		}
		revision, err := provider.database.GetCanonicalRevision(ctx, startup.CanonicalRevisionID)
		if err != nil {
			return httpapi.DashboardContext{}, err
		}
		appliedAt := bundle.CreatedAt
		if bootstrap.Hub.AppliedAt != nil {
			appliedAt = *bootstrap.Hub.AppliedAt
		}
		result.Applied = &httpapi.DashboardApplied{
			Bundle: bundle.ID, Revision: revision.Sequence, AppliedAt: appliedAt,
		}
		if result.View.ExactVersion == "Not selected" {
			result.View.ExactVersion = startup.ExactCoreVersion
		}
		result.Canonical.HasUnappliedChanges = bootstrap.Head != nil && bootstrap.Head.ID != startup.CanonicalRevisionID
	}
	if result.View.ExactVersion != "Not selected" && provider.commands != nil {
		status, err := provider.commands.CoreCapabilityStatus(ctx, result.View.ExactVersion)
		if err != nil {
			return httpapi.DashboardContext{}, err
		}
		result.Capability.Level = string(status.SupportLevel)
		result.Capability.Label = dashboardCapabilityLabel(result.Capability.Level)
		result.Capability.Warning = dashboardCapabilityWarning(result.Capability.Level, status.Quarantined)
	}
	return result, nil
}

func dashboardCapabilityLabel(level string) string {
	switch level {
	case "native_structured":
		return "Native structured"
	case "compatible_structured":
		return "Compatible structured"
	case "manual_json":
		return "Manual JSON"
	default:
		return "Unavailable"
	}
}

func dashboardCapabilityWarning(level string, quarantined bool) *string {
	var warning string
	switch {
	case quarantined:
		warning = "The pinned capability is quarantined; new work is restricted to manual JSON."
	case level == "compatible_structured":
		warning = "Compatible structured projection requires explicit acceptance and remains visibly marked."
	case level == "unavailable":
		warning = "This exact version has no usable configuration path."
	default:
		return nil
	}
	return &warning
}

func stringPointer(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}
