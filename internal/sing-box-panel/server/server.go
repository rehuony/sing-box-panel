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
	"github.com/rehuony/sing-box-panel/internal/console"
	"github.com/rehuony/sing-box-panel/internal/procctl"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/app"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/fleet"
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
func Run(ctx context.Context, settingsPath string, build buildinfo.Info, assets fs.FS) (runErr error) {
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
	configuration, err = services.Settings.EffectiveSettings(ctx)
	if err != nil {
		return startupError(ctx, "load shared panel settings", err)
	}
	if configuration.DataDir != openedDataDir {
		return errors.New("settings changed data directory during startup; restart with a stable settings file")
	}
	output := console.FromContext(ctx)
	services = app.New(database, configuration, app.Options{Build: build, Observe: func(entry store.LogEntry) { output.Event(entry.Time, string(entry.Level), entry.Code, entry.Message) }})
	records := services.Telemetry
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
	trafficRetention, err := records.EnforceTrafficSampleRetention(ctx)
	if err != nil {
		return startupError(ctx, "enforce traffic sample retention", err)
	}
	if trafficRetention.Deleted > 0 {
		records.RecordEvent(telemetry.LogRecordRequest{
			Source: store.LogSourcePanel, Level: store.LogLevelInfo, Code: "traffic.retention_enforced",
			Message: "Expired raw traffic samples were deleted",
			Metadata: telemetry.LogMetadata(map[string]any{
				"deleted": trafficRetention.Deleted, "cutoff": trafficRetention.Cutoff,
			}),
		})
	}
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
	sampleRetentionContext, stopSampleRetention := context.WithCancel(ctx)
	sampleRetentionDone := startTrafficSampleRetention(sampleRetentionContext, services.Settings, records)
	defer func() {
		stopSampleRetention()
		<-sampleRetentionDone
	}()
	handler := httpapi.NewHandler(httpapi.HandlerOptions{
		Settings: configuration,
		Build:    build,
		Assets:   assets,
		Services: services,
	})
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
	// Node sessions share the Web listener; see fleet.SplitListener.
	webListener, nodeListeners := fleet.SplitListener(listener)
	fleetContext, stopFleet := context.WithCancel(ctx)
	fleetResult := make(chan error, 1)
	go func() { fleetResult <- services.Fleet.Serve(fleetContext, nodeListeners) }()
	defer func() {
		stopFleet()
		runErr = errors.Join(runErr, <-fleetResult)
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
		serveResult <- httpServer.Serve(webListener)
	}()
	control.Ready(listener.Addr().String())
	output.Ready("http://"+listener.Addr().String()+configuration.Server.BasePath+"/", settingsPath, configuration.DataDir)

	select {
	case err := <-serveResult:
		cancel()
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return fmt.Errorf("serve panel HTTP server: %w", err)
	case err := <-fleetResult:
		// Report the node service failure once; the deferred wait must not
		// block on a result that was already received.
		fleetResult <- nil
		cancel()
		stopErr := stopHTTPServer(httpServer, serveResult)
		if err == nil {
			return stopErr
		}
		return errors.Join(fmt.Errorf("serve node connections: %w", err), stopErr)
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
