// SPDX-License-Identifier: GPL-3.0-or-later

// Package httpapi implements the versioned management and public HTTP boundary.
package httpapi

import (
	"io/fs"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/rehuony/sing-box-panel/internal/buildinfo"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/app"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/settings"
)

const (
	// Bound JSON credentials before decoding or running password verification.
	maxLoginBody      = 4 << 10
	maxRequestIDBytes = 128
	sessionCookie     = "sbp_session"
)

// SystemStatus describes the panel's own build and platform.
type SystemStatus struct {
	Platform     SystemPlatform `json:"platform"`
	PanelVersion string         `json:"panel_version"`
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
}

type Handler struct {
	settings settings.Settings
	build    buildinfo.Info
	logins   *loginLimiter
	services app.Services
	assetFS  fs.FS
	assets   http.Handler
}

func NewHandler(options HandlerOptions) *Handler {
	var assets http.Handler
	if options.Assets != nil {
		assets = http.FileServer(http.FS(options.Assets))
	}
	return &Handler{
		settings: options.Settings,
		build:    options.Build,
		logins:   newLoginLimiter(),
		services: options.Services,
		assetFS:  options.Assets,
		assets:   assets,
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
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Frame-Options", "DENY")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self' data:; connect-src 'self'")
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
	if strings.HasPrefix(path, "/api/v1/") {
		_, methodAllowed, pathRegistered := registeredManagementOperation(request.Method, path)
		if !pathRegistered {
			writeProblem(w, request, http.StatusNotFound, "operation_not_found", "Operation not found", "The API operation does not exist.")
			return
		}
		if !methodAllowed {
			methodNotAllowed(w, request)
			return
		}
	}
	switch {
	case request.Method == http.MethodGet && path == "/api/v1/health":
		handler.health(w)
	case path == "/sub" || strings.HasPrefix(path, "/sub/"):
		handler.publicSubscription(w, request, path)
	case request.Method == http.MethodGet && path == "/api/v1/auth/session":
		handler.authenticated(handler.currentSession)(w, request)
	case request.Method == http.MethodPost && path == "/api/v1/auth/session":
		handler.login(w, request)
	case request.Method == http.MethodDelete && path == "/api/v1/auth/session":
		handler.authenticated(handler.logout)(w, request)
	case request.Method == http.MethodGet && path == "/api/v1/system/status":
		handler.authenticated(handler.systemStatus)(w, request)
	case handler.handleApplicationRoute(w, request, path):
		return
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

func (handler *Handler) health(w http.ResponseWriter) {
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "version": handler.build.Version})
}
