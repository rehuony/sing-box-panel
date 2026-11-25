// SPDX-License-Identifier: GPL-3.0-or-later

// Package server composes persistent state, HTTP, and process lifecycle.
package server

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"path/filepath"
	"time"

	"github.com/rehuony/sing-box-panel/internal/buildinfo"
	"github.com/rehuony/sing-box-panel/internal/cliui"
	"github.com/rehuony/sing-box-panel/internal/procctl"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/app"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/httpapi"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/installation"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/settings"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/store"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/telemetry"
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
// When the panel is asked to restart, Run stops the server the way a stop
// does and then runs the panel's program again in this process; it returns
// only if that fails.
func Run(ctx context.Context, settingsPath string, build buildinfo.Info, assets fs.FS) error {
	return run(ctx, settingsPath, build, assets, execSelf)
}

func run(ctx context.Context, settingsPath string, build buildinfo.Info, assets fs.FS, exec execFunc) error {
	startedAt := time.Now().UTC()
	serveContext, stop := context.WithCancelCause(ctx)
	defer stop(nil)
	restart, err := newRestart(exec, func() { stop(errRestartRequested) })
	if err != nil {
		return err
	}
	if err := serve(serveContext, settingsPath, build, assets, startedAt, restart); err != nil {
		return err
	}
	if ctx.Err() == nil && errors.Is(context.Cause(serveContext), errRestartRequested) {
		return restart.run()
	}
	return nil
}

func serve(ctx context.Context, settingsPath string, build buildinfo.Info, assets fs.FS, startedAt time.Time, restart *restart) (runErr error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	configuration, err := installation.PrepareDataLocation(ctx, settingsPath)
	if err != nil {
		return startupError(ctx, "load server settings", err)
	}
	if err := prepareDataDirectory(configuration.DataDir); err != nil {
		return err
	}
	runtimeLease, err := procctl.AcquireLease(configuration.DataDir)
	if err != nil {
		return err
	}
	settingsPath, err = filepath.Abs(settingsPath)
	if err != nil {
		return errors.Join(err, runtimeLease.Close())
	}
	control, err := procctl.Listen(procctl.Status{
		Version: build.Version, SettingsPath: settingsPath, DataDir: configuration.DataDir,
		ManagedBy: procctl.ManagedBy("SING_BOX_PANEL_SUPERVISOR"),
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

	if err := settings.EstablishDataLocation(ctx, settingsPath, configuration.DataDir); err != nil {
		return startupError(ctx, "record initialized data directory", err)
	}
	openedDataDir := configuration.DataDir
	configuration, err = settings.LoadForStartup(settingsPath)
	if err != nil {
		return err
	}
	if configuration.DataDir != openedDataDir {
		return errors.New("settings changed data directory during startup; restart with a stable settings file")
	}
	services := app.New(database, configuration, app.Options{})
	if err := services.Settings.RecoverPanelSettingsFile(ctx); err != nil {
		return startupError(ctx, "recover interrupted panel settings save", err)
	}
	if err := settings.UpgradeFile(ctx, settingsPath); err != nil {
		return startupError(ctx, "rewrite panel settings in the current shape", err)
	}
	configuration, err = services.Settings.EffectiveSettings(ctx)
	if err != nil {
		return startupError(ctx, "load shared panel settings", err)
	}
	if configuration.DataDir != openedDataDir {
		return errors.New("settings changed data directory during startup; restart with a stable settings file")
	}
	output := cliui.ConsoleFrom(ctx)
	services = app.New(database, configuration, app.Options{Build: build, Observe: func(entry store.LogEntry) { output.Event(entry.Time, string(entry.Level), entry.Code, entry.Message) }})
	records := services.Telemetry
	restart.bind(services.Releases, records, build.Version)
	defer func() {
		level, code, message := store.LogLevelInfo, "panel.stopped", "Panel server stopped"
		if runErr != nil {
			level, code, message = store.LogLevelError, "panel.failed", "Panel server stopped after an error"
		}
		records.RecordEvent(telemetry.LogRecordRequest{
			Source: store.LogSourcePanel, Level: level, Code: code, Message: message,
			Metadata: telemetry.LogMetadata(map[string]any{"version": build.Version}),
		})
	}()
	if err := enforceRetention(ctx, services); err != nil {
		return startupError(ctx, "enforce record retention", err)
	}
	// Nodes and sources may have changed while the panel was stopped, and a
	// new release may read sources differently.
	services.Subscriptions.PoolInputsChanged(ctx)
	uploadGC, uploadGCErr := services.Cores.GarbageCollectCoreUploads(ctx)
	if uploadGCErr != nil {
		records.RecordEvent(telemetry.LogRecordRequest{
			Source: store.LogSourcePanel, Level: store.LogLevelWarn, Code: "core_upload.gc_aborted",
			Message:  "Staged core upload cleanup was skipped because the staging directory could not be inspected",
			Metadata: telemetry.LogMetadata(map[string]any{"error": uploadGCErr.Error()}),
		})
	} else if uploadGC.Deleted > 0 {
		records.RecordEvent(telemetry.LogRecordRequest{
			Source: store.LogSourcePanel, Level: store.LogLevelInfo, Code: "core_upload.gc_completed",
			Message:  "Unreferenced staged core uploads were removed",
			Metadata: telemetry.LogMetadata(map[string]any{"deleted": uploadGC.Deleted, "retained": uploadGC.Retained}),
		})
	}
	retentionContext, stopRetention := context.WithCancel(ctx)
	retentionDone := startRetention(retentionContext, services)
	defer func() {
		stopRetention()
		<-retentionDone
	}()
	handlerOptions := httpapi.HandlerOptions{
		Settings:  configuration,
		Build:     build,
		Assets:    assets,
		Services:  services,
		StartedAt: startedAt,
	}
	if restartSupported {
		handlerOptions.Restart = restart.request
	}
	handler := httpapi.NewHandler(handlerOptions)
	listener, err := net.Listen("tcp", net.JoinHostPort(configuration.Server.Host, fmt.Sprint(configuration.Server.Port)))
	if err != nil {
		return fmt.Errorf("listen for panel HTTP server: %w", err)
	}

	httpServer := &http.Server{
		BaseContext:       func(net.Listener) context.Context { return ctx },
		Handler:           handler,
		ReadHeaderTimeout: readHeaderTimeout,
		ReadTimeout:       readTimeout,
		IdleTimeout:       idleTimeout,
		MaxHeaderBytes:    maxHeaderBytes,
	}
	// The node endpoints sit beside the Web application under its base path.
	if err := services.Fleet.Open(ctx); err != nil {
		_ = listener.Close()
		return startupError(ctx, "open node endpoints", err)
	}
	defer func() {
		shutdownContext, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()
		services.Fleet.Shutdown(shutdownContext)
	}()
	sweepContext, stopSweep := context.WithCancel(ctx)
	sweepDone := startArtifactCacheSweep(sweepContext, services.Fleet, records)
	defer func() { stopSweep(); <-sweepDone }()
	subscriptionContext, stopSubscriptions := context.WithCancel(ctx)
	subscriptionDone := startSubscriptionRefresh(subscriptionContext, services.Subscriptions, records)
	defer func() { stopSubscriptions(); <-subscriptionDone }()
	catalogContext, stopCatalog := context.WithCancel(ctx)
	catalogDone := startCatalogRefresh(catalogContext, catalogRefreshServices{cores: services.Cores, settings: services.Settings})
	defer func() { stopCatalog(); <-catalogDone }()
	// Releases records an update its root helper applied, then checks for
	// new releases on the catalog refresh interval.
	releaseContext, stopReleases := context.WithCancel(ctx)
	releasesDone := make(chan struct{})
	go func() { defer close(releasesDone); services.Releases.Run(releaseContext) }()
	defer func() { stopReleases(); <-releasesDone }()
	records.RecordEvent(telemetry.LogRecordRequest{
		Source: store.LogSourcePanel, Level: store.LogLevelInfo, Code: "panel.ready",
		Message:  "Panel HTTP server is ready",
		Metadata: telemetry.LogMetadata(map[string]any{"version": build.Version}),
	})
	serveResult := make(chan error, 1)
	go func() {
		serveResult <- httpServer.Serve(listener)
	}()
	control.Ready(listener.Addr().String())
	panelURL := "http://" + listener.Addr().String() + configuration.Server.BasePath + "/"
	style := output.Style()
	output.Ready("sing-box-panel is running", []cliui.Field{
		{Label: "Panel URL", Value: style.Accent(panelURL)},
		{Label: "Settings", Value: style.Path(settingsPath)},
		{Label: "Data directory", Value: style.Path(configuration.DataDir)},
	}, "Press Ctrl+C to stop. View logs in the panel.", map[string]string{
		"event": "server_ready", "panel_url": panelURL, "settings_path": settingsPath,
		"data_dir": configuration.DataDir, "panel_logs": filepath.Join(configuration.DataDir, store.LogsFile),
	})

	select {
	case err := <-serveResult:
		cancel()
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return fmt.Errorf("serve panel HTTP server: %w", err)
	case <-ctx.Done():
		return stopHTTPServer(httpServer, serveResult)
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
