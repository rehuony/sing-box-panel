// SPDX-License-Identifier: GPL-3.0-or-later

// Package httpapi implements the versioned management and public HTTP boundary.
package httpapi

import (
	"context"
	"io/fs"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/rehuony/sing-box-panel/internal/buildinfo"
	"github.com/rehuony/sing-box-panel/internal/nodeapi"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/app"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/settings"
	"github.com/rehuony/sing-box-panel/internal/webauth"
)

const (
	// Bound JSON credentials before decoding or running password verification.
	maxLoginBody      = 4 << 10
	maxRequestIDBytes = 128
	sessionCookie     = "sbp_session"
)

// SystemStatus describes the panel's own build and platform, and when this
// panel process started.
type SystemStatus struct {
	Platform     SystemPlatform `json:"platform"`
	PanelVersion string         `json:"panel_version"`
	StartedAt    time.Time      `json:"started_at"`
}

// Platform describes the deployed panel binary, never the browser's machine.
type SystemPlatform struct {
	OS   string `json:"os"`
	Arch string `json:"arch"`
}

type HandlerOptions struct {
	Settings settings.Settings
	Build    buildinfo.Info
	Assets   fs.FS
	Services app.Services
	// StartedAt is when the panel process started; zero is now.
	StartedAt time.Time
	// Restart asks the panel to restart once the request that asked has
	// been answered; nil where the panel cannot restart itself.
	Restart func(context.Context) error
}

type Handler struct {
	settings  settings.Settings
	build     buildinfo.Info
	logins    *webauth.LoginLimiter
	services  app.Services
	assetFS   fs.FS
	assets    http.Handler
	startedAt time.Time
	restart   func(context.Context) error
	// commands are the fleet commands a client can follow again.
	commands *fleetCommandRuns
}

func NewHandler(options HandlerOptions) *Handler {
	var assets http.Handler
	if options.Assets != nil {
		assets = http.FileServer(http.FS(options.Assets))
	}
	startedAt := options.StartedAt
	if startedAt.IsZero() {
		startedAt = time.Now()
	}
	return &Handler{
		settings:  options.Settings,
		build:     options.Build,
		logins:    webauth.NewLoginLimiter(time.Now),
		services:  options.Services,
		assetFS:   options.Assets,
		assets:    assets,
		startedAt: startedAt.UTC(),
		restart:   options.Restart,
		commands:  newFleetCommandRuns(time.Now),
	}
}

func (handler *Handler) ServeHTTP(w http.ResponseWriter, request *http.Request) {
	requestID := request.Header.Get("X-Request-ID")
	if requestID == "" || len(requestID) > maxRequestIDBytes || !utf8.ValidString(requestID) || len(request.Header.Values("X-Request-ID")) != 1 {
		requestID = newRequestID()
	}
	// Normalize before any route or error can echo the caller's ID. Replace
	// invalid or ambiguous IDs rather than truncating unrelated IDs to one.
	request.Header.Set("X-Request-ID", requestID)
	w.Header().Set("X-Request-ID", requestID)
	webauth.SetSecurityHeaders(w.Header())
	if handler.settings.Server.BasePath != "" && request.Method == http.MethodGet && request.URL.Path == handler.settings.Server.BasePath {
		target := handler.settings.Server.BasePath + "/"
		if request.URL.RawQuery != "" {
			target += "?" + request.URL.RawQuery
		}
		http.Redirect(w, request, target, http.StatusPermanentRedirect)
		return
	}

	path, ok := handler.stripBasePath(request.URL.Path)
	if !ok {
		http.NotFound(w, request)
		return
	}
	switch {
	case strings.HasPrefix(path, nodeapi.PathPrefix+"/"):
		handler.services.Fleet.ServeNode(w, request, path)
	case strings.HasPrefix(path, "/api/v1/"):
		handler.serveOperation(w, request, strings.TrimPrefix(path, "/api/v1"))
	case path == "/sub" || strings.HasPrefix(path, "/sub/"):
		handler.publicSubscription(w, request, path)
	case strings.HasPrefix(path, "/api/"):
		writeProblem(w, request, http.StatusNotFound, "operation_not_found", "Operation not found", "The API operation does not exist.")
	case handler.assets != nil && request.Method == http.MethodGet:
		handler.serveAsset(w, request, path)
	default:
		http.NotFound(w, request)
	}
}

func (handler *Handler) stripBasePath(path string) (string, bool) {
	base := handler.settings.Server.BasePath
	if base == "" {
		return path, true
	}
	if path == base {
		return "/", true
	}
	if !strings.HasPrefix(path, base+"/") {
		return "", false
	}
	return strings.TrimPrefix(path, base), true
}

func (handler *Handler) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "version": handler.build.Version})
}
